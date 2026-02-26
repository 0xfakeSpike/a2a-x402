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

package client

import (
	"context"
	"fmt"

	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/google-agentic-commerce/a2a-x402/core/types"
)

type Client struct {
	x402Client *X402Client
	client     *a2aclient.Client
}

func NewClient(merchantURL string, networkKeyPairs []types.NetworkKeyPair) (*Client, error) {
	a2aClient, err := NewA2AClient(context.Background(), merchantURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create A2A client: %w", err)
	}
	x402Client, err := NewX402Client(networkKeyPairs)
	if err != nil {
		return nil, fmt.Errorf("failed to create x402 client wrapper: %w", err)
	}

	return &Client{
		x402Client: x402Client,
		client:     a2aClient,
	}, nil
}
