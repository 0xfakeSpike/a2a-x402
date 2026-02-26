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

package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/google-agentic-commerce/a2a-x402/core/x402/state"
)

func (c *Client) processPaymentState(
	ctx context.Context,
	task *a2a.Task,
) error {
	paymentState, err := state.ExtractPaymentState(task, nil)
	if err != nil {
		return fmt.Errorf("failed to extract payment state: %w", err)
	}

	switch paymentState.Status {
	case state.PaymentRequired:

		if paymentState.Requirements == nil || len(paymentState.Requirements.Accepts) == 0 {
			return fmt.Errorf("no payment options available")
		}

		paymentMessage, err := c.x402Client.ProcessPaymentRequired(ctx, task.ID, paymentState.Requirements)
		if err != nil {
			return fmt.Errorf("failed to process payment requirements: %w", err)
		}

		_, _, err = SendMessage(ctx, c.client, paymentMessage)
		if err != nil {
			return fmt.Errorf("failed to send payment message: %w", err)
		}

		return nil

	case state.PaymentCompleted:
		return nil

	case state.PaymentFailed:
		if task.Status.Message != nil {
			msgJSON, err := json.Marshal(task.Status.Message)
			if err == nil {
				return fmt.Errorf("payment failed: %s", string(msgJSON))
			}
			if task.Status.Message.Parts != nil {
				for _, part := range task.Status.Message.Parts {
					if textPart, ok := part.(a2a.TextPart); ok && textPart.Text != "" {
						return fmt.Errorf("payment failed: %s", textPart.Text)
					}
				}
			}
		}
		// If no message is available, return a generic error
		return fmt.Errorf("payment failed")

	default:
		if task.Status.State == a2a.TaskStateWorking {
			if task.Status.Message != nil {
				msgJSON, err := json.Marshal(task.Status.Message)
				if err == nil {
					return fmt.Errorf("unknown payment state: %s", string(msgJSON))
				}
			}
		}
		return nil
	}
}
