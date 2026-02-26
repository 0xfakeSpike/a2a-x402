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
	"fmt"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
)

// WaitForCompletion starts a task by sending a message and waits for it to reach a terminal state.
func (c *Client) WaitForCompletion(ctx context.Context, messageText string) (*a2a.Task, error) {

	message := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: messageText})
	task, _, err := SendMessage(ctx, c.client, message)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	for {
		task, err = c.client.GetTask(ctx, &a2a.TaskQueryParams{
			ID: task.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get task: %w", err)
		}

		if err := c.processPaymentState(ctx, task); err != nil {
			return nil, fmt.Errorf("failed to process payment state: %w", err)
		}

		if task.Status.State.Terminal() {
			return task, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
