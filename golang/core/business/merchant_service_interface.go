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

// Package business defines the interfaces and types that merchants must implement
// to integrate their business services with the x402 payment framework.
// These interfaces allow developers to implement custom business logic while
// the framework handles payment verification and settlement.
package business

import (
	"context"
)

type BusinessService interface {
	Execute(ctx context.Context, prompt string) (string, error)

	ServiceRequirements(prompt string) ServiceRequirements
}

type ServiceRequirements struct {
	// Price is the payment amount required for the service (as a string, e.g., "1", "0.5")
	Price string

	// Resource is the resource identifier or URL associated with this service
	Resource string

	// Description is a human-readable description of the service
	Description string

	// MimeType is the MIME type of the resource (e.g., "application/json", "image/png")
	MimeType string

	// Scheme is the payment scheme (e.g., "exact", "at-least")
	Scheme string

	// MaxTimeoutSeconds is the maximum time in seconds before payment expires
	MaxTimeoutSeconds int
}
