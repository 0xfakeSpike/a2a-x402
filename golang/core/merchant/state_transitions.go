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
	"github.com/google-agentic-commerce/a2a-x402/core/x402/state"
)

func (o *BusinessOrchestrator) createTask(
	ctx context.Context,
	requestContext *a2asrv.RequestContext,
	eventQueue eventqueue.Queue,
) (*a2a.Task, error) {
	requestContext.StoredTask = a2a.NewSubmittedTask(requestContext, requestContext.Message)

	event := a2a.NewStatusUpdateEvent(requestContext, a2a.TaskStateSubmitted, nil)
	if err := eventQueue.Write(ctx, event); err != nil {
		return nil, fmt.Errorf("failed to write task creation event: %w", err)
	}

	return requestContext.StoredTask, nil
}

func (o *BusinessOrchestrator) transitionToPaymentRequired(
	ctx context.Context,
	requestContext *a2asrv.RequestContext,
	task *a2a.Task,
	queue eventqueue.Queue,
	paymentState *state.PaymentState,
) error {
	task.Status.State = a2a.TaskStateInputRequired

	if err := state.RecordPaymentRequired(task, paymentState.Requirements, "Payment required"); err != nil {
		return fmt.Errorf("failed to record payment required: %w", err)
	}

	originalPrompt := state.ExtractMessageText(requestContext.Message)
	if originalPrompt != "" {
		state.SetOriginalPrompt(task.Status.Message, originalPrompt)
	}

	event := a2a.NewStatusUpdateEvent(requestContext, a2a.TaskStateInputRequired, task.Status.Message)
	event.Final = true

	return queue.Write(ctx, event)
}

func (o *BusinessOrchestrator) transitionToCompleted(
	ctx context.Context,
	requestContext *a2asrv.RequestContext,
	task *a2a.Task,
	queue eventqueue.Queue,
	result *state.PaymentState,
) error {
	responseText := result.Message
	if responseText == "" {
		responseText = "Task completed"
	}

	if err := state.RecordPaymentCompleted(task, result.Receipts, responseText); err != nil {
		return fmt.Errorf("failed to record payment completed: %w", err)
	}

	task.Status.State = a2a.TaskStateCompleted

	event := a2a.NewStatusUpdateEvent(requestContext, a2a.TaskStateCompleted, task.Status.Message)
	event.Final = true

	return queue.Write(ctx, event)
}

func (o *BusinessOrchestrator) transitionToFailed(
	ctx context.Context,
	requestContext *a2asrv.RequestContext,
	task *a2a.Task,
	queue eventqueue.Queue,
	err error,
	errorCode string,
) error {
	task.Status.State = a2a.TaskStateFailed

	state.RecordPaymentFailed(task, errorCode, err.Error())

	event := a2a.NewStatusUpdateEvent(requestContext, a2a.TaskStateFailed, task.Status.Message)
	event.Final = true

	return queue.Write(ctx, event)
}

func (o *BusinessOrchestrator) transitionToPaymentVerified(
	ctx context.Context,
	requestContext *a2asrv.RequestContext,
	task *a2a.Task,
	queue eventqueue.Queue,
	paymentState *state.PaymentState,
) error {
	if err := state.RecordPaymentVerified(task, paymentState, "Payment verified"); err != nil {
		return fmt.Errorf("failed to record payment verified: %w", err)
	}

	event := a2a.NewStatusUpdateEvent(requestContext, task.Status.State, task.Status.Message)
	event.Final = false

	return queue.Write(ctx, event)
}
