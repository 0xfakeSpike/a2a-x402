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

	"github.com/a2aproject/a2a-go/a2a"
	x402 "github.com/coinbase/x402/go"
	evm "github.com/coinbase/x402/go/mechanisms/evm/exact/client"
	svm "github.com/coinbase/x402/go/mechanisms/svm/exact/client"
	evmsigners "github.com/coinbase/x402/go/signers/evm"
	svmsigners "github.com/coinbase/x402/go/signers/svm"
	x402types "github.com/coinbase/x402/go/types"
	"github.com/google-agentic-commerce/a2a-x402/core/types"
	x402pkg "github.com/google-agentic-commerce/a2a-x402/core/x402"
	"github.com/google-agentic-commerce/a2a-x402/core/x402/state"
)

type X402Client struct {
	client *x402.X402Client
}

func NewX402Client(networkKeyPairs []types.NetworkKeyPair) (*X402Client, error) {
	if len(networkKeyPairs) == 0 {
		return nil, fmt.Errorf("at least one network-key pair is required")
	}

	client := x402.Newx402Client()

	for _, pair := range networkKeyPairs {
		switch {
		case pair.NetworkName == x402pkg.NetworkBase || pair.NetworkName == x402pkg.NetworkBaseSepolia:
			evmSigner, err := evmsigners.NewClientSignerFromPrivateKey(pair.PrivateKey)
			if err != nil {
				return nil, fmt.Errorf("failed to create EVM signer for network %s: %w", pair.NetworkName, err)
			}
			client.Register(x402.Network(pair.NetworkName), evm.NewExactEvmScheme(evmSigner))
		case pair.NetworkName == x402pkg.NetworkSolanaMainnet || pair.NetworkName == x402pkg.NetworkSolanaDevnet || pair.NetworkName == x402pkg.NetworkSolanaTestnet:
			svmSigner, err := svmsigners.NewClientSignerFromPrivateKey(pair.PrivateKey)
			if err != nil {
				return nil, fmt.Errorf("failed to create SVM signer for network %s: %w", pair.NetworkName, err)
			}
			client.Register(x402.Network(pair.NetworkName), svm.NewExactSvmScheme(svmSigner))
		default:
			return nil, fmt.Errorf("unsupported network: %s", pair.NetworkName)
		}
	}
	return &X402Client{
		client: client,
	}, nil
}

func (c *X402Client) ProcessPaymentRequired(
	ctx context.Context,
	taskID a2a.TaskID,
	paymentRequired *x402types.PaymentRequired,
) (*a2a.Message, error) {
	if len(paymentRequired.Accepts) == 0 {
		return nil, fmt.Errorf("no payment options available")
	}

	paymentRequirements, err := c.client.SelectPaymentRequirements(paymentRequired.Accepts)
	if err != nil {
		return nil, fmt.Errorf("no matching payment option found: %w", err)
	}

	resource, description, mimeType, _ := x402pkg.A2AFieldsFromExtra(&paymentRequirements)
	var resourceInfo *x402types.ResourceInfo
	if resource != "" || description != "" || mimeType != "" {
		resourceInfo = &x402types.ResourceInfo{
			URL:         resource,
			Description: description,
			MimeType:    mimeType,
		}
	}

	payload, err := c.client.CreatePaymentPayload(
		ctx,
		paymentRequirements,
		resourceInfo,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment payload: %w", err)
	}

	paymentMessage, err := state.EncodePaymentSubmission(taskID, &payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode payment submission: %w", err)
	}

	return paymentMessage, nil
}
