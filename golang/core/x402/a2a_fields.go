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

package x402

import (
	x402types "github.com/coinbase/x402/go/types"
)

const (
	ExtraKeyResource     = "resource"
	ExtraKeyDescription  = "description"
	ExtraKeyMimeType     = "mimeType"
	ExtraKeyOutputSchema = "outputSchema"
)

func AddA2AFieldsToExtra(req *x402types.PaymentRequirements, resource, description, mimeType string, outputSchema interface{}) {
	if req.Extra == nil {
		req.Extra = make(map[string]interface{})
	}
	if resource != "" {
		req.Extra[ExtraKeyResource] = resource
	}
	if description != "" {
		req.Extra[ExtraKeyDescription] = description
	}
	if mimeType != "" {
		req.Extra[ExtraKeyMimeType] = mimeType
	}
	if outputSchema != nil {
		req.Extra[ExtraKeyOutputSchema] = outputSchema
	}
}

func A2AFieldsFromExtra(req *x402types.PaymentRequirements) (resource, description, mimeType string, outputSchema interface{}) {
	if req.Extra == nil {
		return "", "", "", nil
	}
	if r, ok := req.Extra[ExtraKeyResource].(string); ok {
		resource = r
	}
	if d, ok := req.Extra[ExtraKeyDescription].(string); ok {
		description = d
	}
	if m, ok := req.Extra[ExtraKeyMimeType].(string); ok {
		mimeType = m
	}
	outputSchema = req.Extra[ExtraKeyOutputSchema]
	return
}
