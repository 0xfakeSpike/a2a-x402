// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package merchant

import (
	"context"
	"fmt"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	"github.com/google-agentic-commerce/a2a-x402/core/business"
	"github.com/google-agentic-commerce/a2a-x402/core/types"
	"github.com/google-agentic-commerce/a2a-x402/core/x402"
	"github.com/google-agentic-commerce/a2a-x402/core/x402/state"
)

type BusinessOrchestrator struct {
	merchant         ResourceServer
	businessService  business.BusinessService
	networkConfigs   []types.NetworkConfig
	extensionChecker ExtensionChecker
}

// NewBusinessOrchestrator creates a new orchestrator with real dependencies (production use)
func NewBusinessOrchestrator(
	ctx context.Context,
	facilitatorURL string,
	businessService business.BusinessService,
	networkConfigs []types.NetworkConfig,
) (*BusinessOrchestrator, error) {
	resourceServer, err := NewResourceServer(ctx, facilitatorURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create x402 resource server: %w", err)
	}

	merchant := &resourceServerWrapper{server: resourceServer}

	return &BusinessOrchestrator{
		merchant:         merchant,
		businessService:  businessService,
		networkConfigs:   networkConfigs,
		extensionChecker: DefaultExtensionChecker(),
	}, nil
}

// NewBusinessOrchestratorWithDeps creates a new orchestrator with dependency injection support (for testing)
func NewBusinessOrchestratorWithDeps(
	merchant ResourceServer,
	businessService business.BusinessService,
	networkConfigs []types.NetworkConfig,
	extensionChecker ExtensionChecker,
) *BusinessOrchestrator {
	if extensionChecker == nil {
		extensionChecker = DefaultExtensionChecker()
	}
	return &BusinessOrchestrator{
		merchant:         merchant,
		businessService:  businessService,
		networkConfigs:   networkConfigs,
		extensionChecker: extensionChecker,
	}
}

func (o *BusinessOrchestrator) Execute(
	ctx context.Context,
	requestContext *a2asrv.RequestContext,
	eventQueue eventqueue.Queue,
) error {
	message := requestContext.Message

	task := requestContext.StoredTask
	if requestContext.Message.TaskID == "" && task == nil {
		var err error
		task, err = o.createTask(ctx, requestContext, eventQueue)
		if err != nil {
			return err
		}
	}

	if err := o.ensureExtension(ctx, requestContext, task, eventQueue); err != nil {
		return err
	}

	paymentState, err := state.ExtractPaymentState(task, message)
	if err != nil {
		return o.transitionToFailed(ctx, requestContext, task, eventQueue,
			fmt.Errorf("failed to extract payment state: %w", err), "state_extraction_failed")
	}

	for {
		if task.Status.State == a2a.TaskStateFailed {
			return nil
		}
		if task.Status.State == a2a.TaskStateCompleted {
			return nil
		}

		switch paymentState.Status {
		case state.PaymentRequired:
			if paymentState.Payload != nil {
				paymentState.Status = state.PaymentSubmitted
				var err error
				paymentState, err = o.handlePaymentSubmitted(ctx, requestContext, task, eventQueue, paymentState)
				if err != nil {
					return err
				}
				continue
			}
			return nil

		case state.PaymentSubmitted:
			var err error
			paymentState, err = o.handlePaymentSubmitted(ctx, requestContext, task, eventQueue, paymentState)
			if err != nil {
				return err
			}

		case state.PaymentVerified:
			var err error
			paymentState, err = o.handlePaymentVerified(ctx, task, paymentState)
			if err != nil {
				return o.transitionToFailed(ctx, requestContext, task, eventQueue,
					fmt.Errorf("business execution failed: %w", err), "business_execution_failed")
			}

		case state.PaymentCompleted:
			return o.transitionToCompleted(ctx, requestContext, task, eventQueue, paymentState)

		default:
			prompt := state.ExtractMessageText(message)
			paymentState, err := o.buildPaymentRequirements(ctx, prompt)
			if err != nil {
				return o.transitionToFailed(ctx, requestContext, task, eventQueue,
					fmt.Errorf("failed to create payment requirements: %w", err), "payment_requirements_creation_failed")
			}
			return o.transitionToPaymentRequired(ctx, requestContext, task, eventQueue, paymentState)
		}
	}
}

func (o *BusinessOrchestrator) Cancel(
	ctx context.Context,
	requestContext *a2asrv.RequestContext,
	queue eventqueue.Queue,
) error {
	message := a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: "Task cancelled"})
	event := a2a.NewStatusUpdateEvent(requestContext, a2a.TaskStateFailed, message)
	event.Final = true
	return queue.Write(ctx, event)
}

func (o *BusinessOrchestrator) ensureExtension(
	ctx context.Context,
	requestContext *a2asrv.RequestContext,
	task *a2a.Task,
	eventQueue eventqueue.Queue,
) error {
	extensions, ok := o.extensionChecker.ExtensionsFrom(ctx)
	if !ok {
		errorMsg := "x402 extension is required but not active. Client must send X-A2A-Extensions header with value: " + x402.X402ExtensionURI
		err := fmt.Errorf("%s", errorMsg)
		if transitionErr := o.transitionToFailed(ctx, requestContext, task, eventQueue, err, "extension_missing"); transitionErr != nil {
			return fmt.Errorf("failed to transition to failed state: %w", transitionErr)
		}
		return err
	}

	x402Extension := &a2a.AgentExtension{
		URI: x402.X402ExtensionURI,
	}
	if !extensions.Requested(x402Extension) {
		errorMsg := "x402 extension is required but not active. Client must send X-A2A-Extensions header with value: " + x402.X402ExtensionURI
		err := fmt.Errorf("%s", errorMsg)
		if transitionErr := o.transitionToFailed(ctx, requestContext, task, eventQueue, err, "extension_not_requested"); transitionErr != nil {
			return fmt.Errorf("failed to transition to failed state: %w", transitionErr)
		}
		return err
	}

	return nil
}
