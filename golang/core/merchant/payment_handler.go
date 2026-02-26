// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed on the License is distributed on an "AS IS" BASIS,
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
	x402core "github.com/coinbase/x402/go"
	x402types "github.com/coinbase/x402/go/types"
	"github.com/google-agentic-commerce/a2a-x402/core/x402/state"
)

func (o *BusinessOrchestrator) buildPaymentRequirements(
	ctx context.Context,
	prompt string,
) (*state.PaymentState, error) {

	serviceReq := o.businessService.ServiceRequirements(prompt)
	allRequirements := make([]x402types.PaymentRequirements, 0)

	for _, networkConfig := range o.networkConfigs {
		reqs, err := BuildPaymentRequirements(ctx, o.merchant, networkConfig, serviceReq)
		if err != nil {
			return nil, fmt.Errorf("failed to create payment requirement for network %s: %w", networkConfig.NetworkName, err)
		}

		for _, req := range reqs {
			allRequirements = append(allRequirements, *req)
		}
	}

	return &state.PaymentState{
		Status: state.PaymentRequired,
		Requirements: &x402types.PaymentRequired{
			X402Version: 2,
			Accepts:     allRequirements,
		},
	}, nil
}

func (o *BusinessOrchestrator) findMatchingRequirement(paymentState *state.PaymentState) (*x402types.PaymentRequirements, error) {
	if paymentState.Payload == nil {
		return nil, fmt.Errorf("payment payload is required")
	}
	if paymentState.Requirements == nil || len(paymentState.Requirements.Accepts) == 0 {
		return nil, fmt.Errorf("payment requirements are required")
	}

	matchedRequirement := o.merchant.FindMatchingRequirements(
		paymentState.Requirements.Accepts,
		*paymentState.Payload,
	)
	if matchedRequirement == nil {
		return nil, fmt.Errorf("no matching payment requirement found for payload (accepted: scheme=%s, network=%s, amount=%s, asset=%s, payTo=%s)",
			paymentState.Payload.Accepted.Scheme,
			paymentState.Payload.Accepted.Network,
			paymentState.Payload.Accepted.Amount,
			paymentState.Payload.Accepted.Asset,
			paymentState.Payload.Accepted.PayTo)
	}

	return matchedRequirement, nil
}

func (o *BusinessOrchestrator) verifyPayment(
	ctx context.Context,
	paymentState *state.PaymentState,
) error {
	matchedRequirement, err := o.findMatchingRequirement(paymentState)
	if err != nil {
		return fmt.Errorf("failed to find matching requirement: %w", err)
	}

	verifyResponse, err := o.merchant.VerifyPayment(
		ctx,
		*paymentState.Payload,
		*matchedRequirement,
	)
	if err != nil {
		return fmt.Errorf("payment verification failed: %w", err)
	}

	if !verifyResponse.IsValid {
		return fmt.Errorf("payment verification failed: %s, %s", verifyResponse.InvalidReason, verifyResponse.InvalidMessage)
	}

	return nil
}

func (o *BusinessOrchestrator) handlePaymentSubmitted(
	ctx context.Context,
	requestContext *a2asrv.RequestContext,
	task *a2a.Task,
	eventQueue eventqueue.Queue,
	paymentState *state.PaymentState,
) (*state.PaymentState, error) {
	if task.Status.State == a2a.TaskStateFailed || task.Status.State == a2a.TaskStateCompleted {
		updatedState, err := state.ExtractPaymentState(task, requestContext.Message)
		if err != nil {
			return nil, fmt.Errorf("failed to re-extract payment state: %w", err)
		}
		return updatedState, nil
	}

	if err := o.verifyPayment(ctx, paymentState); err != nil {
		return nil, o.transitionToFailed(ctx, requestContext, task, eventQueue,
			fmt.Errorf("payment verification failed: %w", err), "payment_verification_failed")
	}

	paymentState.Status = state.PaymentVerified
	if err := o.transitionToPaymentVerified(ctx, requestContext, task, eventQueue, paymentState); err != nil {
		return nil, fmt.Errorf("failed to record payment verified state: %w", err)
	}

	return &state.PaymentState{
		Status:       state.PaymentVerified,
		Requirements: paymentState.Requirements,
		Payload:      paymentState.Payload,
		Receipts:     paymentState.Receipts,
	}, nil
}

func (o *BusinessOrchestrator) handlePaymentVerified(
	ctx context.Context,
	task *a2a.Task,
	paymentState *state.PaymentState,
) (*state.PaymentState, error) {
	matchedRequirement, err := o.findMatchingRequirement(paymentState)
	if err != nil {
		return nil, err
	}

	prompt := state.ExtractOriginalPrompt(task)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required: original prompt not found in task metadata")
	}

	businessMessage, err := o.businessService.Execute(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("business logic execution failed: %w", err)
	}

	settleResponse, err := o.settlePayment(ctx, paymentState, matchedRequirement)
	if err != nil {
		return nil, err
	}

	return &state.PaymentState{
		Status:   state.PaymentCompleted,
		Message:  businessMessage,
		Receipts: []*x402core.SettleResponse{settleResponse},
	}, nil
}

func (o *BusinessOrchestrator) settlePayment(
	ctx context.Context,
	paymentState *state.PaymentState,
	matchedRequirement *x402types.PaymentRequirements,
) (*x402core.SettleResponse, error) {
	settleResponse, err := o.merchant.SettlePayment(
		ctx,
		*paymentState.Payload,
		*matchedRequirement,
	)
	if err != nil {
		return nil, fmt.Errorf("payment settlement failed: %w", err)
	}

	if !settleResponse.Success {
		return nil, fmt.Errorf("payment settlement failed: %s", settleResponse.ErrorReason)
	}

	return settleResponse, nil
}
