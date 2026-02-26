// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package merchant

import (
	"context"
	"errors"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	x402core "github.com/coinbase/x402/go"
	x402pkg "github.com/coinbase/x402/go"
	x402types "github.com/coinbase/x402/go/types"
	"github.com/google-agentic-commerce/a2a-x402/core/business"
	"github.com/google-agentic-commerce/a2a-x402/core/types"
	"github.com/google-agentic-commerce/a2a-x402/core/x402"
	x402state "github.com/google-agentic-commerce/a2a-x402/core/x402/state"
)

type mockBusinessService struct {
	executeFunc func(ctx context.Context, prompt string) (string, error)
}

func (m *mockBusinessService) Execute(ctx context.Context, prompt string) (string, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, prompt)
	}
	return "Mock response", nil
}

func (m *mockBusinessService) ServiceRequirements(prompt string) business.ServiceRequirements {
	return business.ServiceRequirements{
		Price:             "1.00",
		Resource:          "/test",
		Description:       "Test service",
		MimeType:          "application/json",
		Scheme:            "exact",
		MaxTimeoutSeconds: 60,
	}
}

type MockResourceServer struct {
	BuildPaymentRequirementsFromConfigFunc func(ctx context.Context, config x402pkg.ResourceConfig) ([]x402types.PaymentRequirements, error)
	FindMatchingRequirementsFunc           func(accepts []x402types.PaymentRequirements, payload x402types.PaymentPayload) *x402types.PaymentRequirements
	VerifyPaymentFunc                      func(ctx context.Context, payload x402types.PaymentPayload, requirements x402types.PaymentRequirements) (*x402core.VerifyResponse, error)
	SettlePaymentFunc                      func(ctx context.Context, payload x402types.PaymentPayload, requirements x402types.PaymentRequirements) (*x402core.SettleResponse, error)
}

func (m *MockResourceServer) BuildPaymentRequirementsFromConfig(ctx context.Context, config x402pkg.ResourceConfig) ([]x402types.PaymentRequirements, error) {
	if m.BuildPaymentRequirementsFromConfigFunc != nil {
		return m.BuildPaymentRequirementsFromConfigFunc(ctx, config)
	}
	return []x402types.PaymentRequirements{
		{
			Scheme:  "exact",
			Network: string(config.Network),
			PayTo:   config.PayTo,
			Asset:   "0x456",
		},
	}, nil
}

func (m *MockResourceServer) FindMatchingRequirements(accepts []x402types.PaymentRequirements, payload x402types.PaymentPayload) *x402types.PaymentRequirements {
	if m.FindMatchingRequirementsFunc != nil {
		return m.FindMatchingRequirementsFunc(accepts, payload)
	}
	if len(accepts) > 0 {
		return &accepts[0]
	}
	return nil
}

func (m *MockResourceServer) VerifyPayment(ctx context.Context, payload x402types.PaymentPayload, requirements x402types.PaymentRequirements) (*x402core.VerifyResponse, error) {
	if m.VerifyPaymentFunc != nil {
		return m.VerifyPaymentFunc(ctx, payload, requirements)
	}
	return &x402core.VerifyResponse{IsValid: true, Payer: "0x789"}, nil
}

func (m *MockResourceServer) SettlePayment(ctx context.Context, payload x402types.PaymentPayload, requirements x402types.PaymentRequirements) (*x402core.SettleResponse, error) {
	if m.SettlePaymentFunc != nil {
		return m.SettlePaymentFunc(ctx, payload, requirements)
	}
	return &x402core.SettleResponse{Success: true, Network: "base-sepolia"}, nil
}

type MockExtensionChecker struct {
	ExtensionsFromFunc func(ctx context.Context) (*a2asrv.Extensions, bool)
}

func (m *MockExtensionChecker) ExtensionsFrom(ctx context.Context) (*a2asrv.Extensions, bool) {
	if m.ExtensionsFromFunc != nil {
		return m.ExtensionsFromFunc(ctx)
	}
	return a2asrv.ExtensionsFrom(ctx)
}

func newMockExtensionCheckerWithX402() *MockExtensionChecker {
	return &MockExtensionChecker{
		ExtensionsFromFunc: func(ctx context.Context) (*a2asrv.Extensions, bool) {
			headers := map[string][]string{
				"X-A2A-Extensions": {x402.X402ExtensionURI},
			}
			requestMeta := a2asrv.NewRequestMeta(headers)
			ctxWithMeta, _ := a2asrv.WithCallContext(context.Background(), requestMeta)
			exts, ok := a2asrv.ExtensionsFrom(ctxWithMeta)
			if !ok {
				return nil, false
			}
			return exts, true
		},
	}
}

type mockEventQueue struct {
	events []interface{}
}

func (m *mockEventQueue) Write(ctx context.Context, event a2a.Event) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventQueue) WriteVersioned(ctx context.Context, event a2a.Event, version a2a.TaskVersion) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventQueue) Read(ctx context.Context) (a2a.Event, a2a.TaskVersion, error) {
	if len(m.events) == 0 {
		return nil, 0, nil
	}
	event := m.events[0]
	m.events = m.events[1:]
	if e, ok := event.(a2a.Event); ok {
		return e, 0, nil
	}
	return nil, 0, nil
}

func (m *mockEventQueue) Close() error {
	return nil
}

func TestBusinessOrchestrator_ensureExtension(t *testing.T) {
	ctx := context.Background()
	mockService := &mockBusinessService{}
	mockQueue := &mockEventQueue{}

	tests := []struct {
		name           string
		checker        ExtensionChecker
		wantErr        bool
		expectedEvents int
	}{
		{
			name: "extension enabled",
			checker: &MockExtensionChecker{
				ExtensionsFromFunc: func(ctx context.Context) (*a2asrv.Extensions, bool) {
					// Create a context with x402 extension header
					headers := map[string][]string{
						"X-A2A-Extensions": {x402.X402ExtensionURI},
					}
					requestMeta := a2asrv.NewRequestMeta(headers)
					ctxWithMeta, _ := a2asrv.WithCallContext(context.Background(), requestMeta)
					exts, ok := a2asrv.ExtensionsFrom(ctxWithMeta)
					if !ok {
						return nil, false
					}
					return exts, true
				},
			},
			wantErr:        false,
			expectedEvents: 0,
		},
		{
			name: "extension missing",
			checker: &MockExtensionChecker{
				ExtensionsFromFunc: func(ctx context.Context) (*a2asrv.Extensions, bool) {
					return nil, false
				},
			},
			wantErr:        true,
			expectedEvents: 1,
		},
		{
			name: "extension not requested",
			checker: &MockExtensionChecker{
				ExtensionsFromFunc: func(ctx context.Context) (*a2asrv.Extensions, bool) {
					exts, ok := a2asrv.ExtensionsFrom(ctx)
					if ok {
						x402Ext := &a2a.AgentExtension{URI: x402.X402ExtensionURI}
						if !exts.Requested(x402Ext) {
							return exts, true
						}
					}
					headers := map[string][]string{
						"X-A2A-Extensions": {"https://other-extension.org/"},
					}
					requestMeta := a2asrv.NewRequestMeta(headers)
					ctxWithMeta, _ := a2asrv.WithCallContext(context.Background(), requestMeta)
					exts, ok = a2asrv.ExtensionsFrom(ctxWithMeta)
					if !ok {
						return nil, false
					}
					return exts, true
				},
			},
			wantErr:        true,
			expectedEvents: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQueue.events = nil
			mockMerchant := &MockResourceServer{}

			orchestrator := NewBusinessOrchestratorWithDeps(
				mockMerchant,
				mockService,
				[]types.NetworkConfig{
					{NetworkName: "eip155:84532", PayToAddress: "0x123"},
				},
				tt.checker,
			)

			task := &a2a.Task{
				ID:        "task-123",
				ContextID: "context-456",
				Status:    a2a.TaskStatus{State: a2a.TaskStateWorking, Message: a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: ""})},
			}
			requestContext := &a2asrv.RequestContext{
				Message:    a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: "test"}),
				StoredTask: task,
				TaskID:     "task-123",
				ContextID:  "context-456",
			}

			err := orchestrator.ensureExtension(ctx, requestContext, task, mockQueue)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				if len(mockQueue.events) != tt.expectedEvents {
					t.Errorf("expected %d events, got %d", tt.expectedEvents, len(mockQueue.events))
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestBusinessOrchestrator_Execute_PaymentFlow(t *testing.T) {
	ctx := context.Background()

	paymentRequirements := x402types.PaymentRequirements{
		Scheme:  "exact",
		Network: "base-sepolia",
		PayTo:   "0x123",
		Asset:   "0x456",
	}

	paymentPayload := x402types.PaymentPayload{
		X402Version: 1,
		Accepted: x402types.PaymentRequirements{
			Scheme:  "exact",
			Network: "base-sepolia",
			Amount:  "100",
			Asset:   "0x456",
			PayTo:   "0x123",
		},
		Payload: map[string]interface{}{
			"signature": "0xabc",
			"authorization": map[string]interface{}{
				"from":         "0x789",
				"to":           "0x123",
				"value":        "100",
				"valid_after":  "0",
				"valid_before": "9999999999",
				"nonce":        "0xdef",
			},
		},
	}

	var verifyCalled, settleCalled, businessExecuteCalled bool

	mockMerchant := &MockResourceServer{
		FindMatchingRequirementsFunc: func(accepts []x402types.PaymentRequirements, payload x402types.PaymentPayload) *x402types.PaymentRequirements {
			return &paymentRequirements
		},
		VerifyPaymentFunc: func(ctx context.Context, payload x402types.PaymentPayload, requirements x402types.PaymentRequirements) (*x402core.VerifyResponse, error) {
			verifyCalled = true
			return &x402core.VerifyResponse{IsValid: true, Payer: "0x789"}, nil
		},
		SettlePaymentFunc: func(ctx context.Context, payload x402types.PaymentPayload, requirements x402types.PaymentRequirements) (*x402core.SettleResponse, error) {
			settleCalled = true
			return &x402core.SettleResponse{Success: true, Network: "base-sepolia"}, nil
		},
	}

	mockService := &mockBusinessService{
		executeFunc: func(ctx context.Context, prompt string) (string, error) {
			businessExecuteCalled = true
			return "Business logic executed successfully", nil
		},
	}

	mockQueue := &mockEventQueue{}
	mockExtensionChecker := newMockExtensionCheckerWithX402()

	orchestrator := NewBusinessOrchestratorWithDeps(
		mockMerchant,
		mockService,
		[]types.NetworkConfig{
			{NetworkName: "eip155:84532", PayToAddress: "0x123"},
		},
		mockExtensionChecker,
	)

	task := &a2a.Task{
		ID:        "task-123",
		ContextID: "context-456",
		Status:    a2a.TaskStatus{State: a2a.TaskStateWorking, Message: a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: ""})},
	}
	x402state.SetPaymentStatus(task.Status.Message, x402state.PaymentSubmitted)
	x402state.SetPaymentPayload(task.Status.Message, &paymentPayload)
	x402state.SetPaymentRequirements(task.Status.Message, &x402types.PaymentRequired{
		X402Version: 2,
		Accepts:     []x402types.PaymentRequirements{paymentRequirements},
	})
	x402state.SetOriginalPrompt(task.Status.Message, "test prompt")

	message := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: "Payment authorization provided"})
	x402state.SetPaymentStatus(message, x402state.PaymentSubmitted)
	x402state.SetPaymentPayload(message, &paymentPayload)

	requestContext := &a2asrv.RequestContext{
		Message:    message,
		StoredTask: task,
		TaskID:     "task-123",
		ContextID:  "context-456",
	}

	err := orchestrator.Execute(ctx, requestContext, mockQueue)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !verifyCalled {
		t.Error("verifyPayment was not called")
	}
	if !settleCalled {
		t.Error("settlePayment was not called")
	}
	if !businessExecuteCalled {
		t.Error("business service Execute was not called")
	}

	if task.Status.State != a2a.TaskStateCompleted {
		t.Errorf("expected task state to be Completed, got %v", task.Status.State)
	}

	if len(mockQueue.events) == 0 {
		t.Error("expected events to be written to queue")
	}
}

func TestBusinessOrchestrator_handlePaymentSubmitted(t *testing.T) {
	ctx := context.Background()

	paymentRequirements := x402types.PaymentRequirements{
		Scheme:  "exact",
		Network: "base-sepolia",
		PayTo:   "0x123",
		Asset:   "0x456",
	}

	paymentPayload := x402types.PaymentPayload{
		X402Version: 1,
		Accepted: x402types.PaymentRequirements{
			Scheme:  "exact",
			Network: "base-sepolia",
			Amount:  "100",
			Asset:   "0x456",
			PayTo:   "0x123",
		},
	}

	tests := []struct {
		name           string
		verifyResponse *x402core.VerifyResponse
		verifyError    error
		wantErr        bool
		wantState      x402state.PaymentStatus
	}{
		{
			name:           "valid payment",
			verifyResponse: &x402core.VerifyResponse{IsValid: true, Payer: "0x789"},
			verifyError:    nil,
			wantErr:        false,
			wantState:      x402state.PaymentVerified,
		},
		{
			name:           "invalid payment",
			verifyResponse: &x402core.VerifyResponse{IsValid: false, InvalidReason: "insufficient_funds"},
			verifyError:    nil,
			wantErr:        true,
			wantState:      x402state.PaymentFailed,
		},
		{
			name:           "verification error",
			verifyResponse: nil,
			verifyError:    errors.New("verification failed"),
			wantErr:        true,
			wantState:      x402state.PaymentFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockMerchant := &MockResourceServer{
				FindMatchingRequirementsFunc: func(accepts []x402types.PaymentRequirements, payload x402types.PaymentPayload) *x402types.PaymentRequirements {
					return &paymentRequirements
				},
				VerifyPaymentFunc: func(ctx context.Context, payload x402types.PaymentPayload, requirements x402types.PaymentRequirements) (*x402core.VerifyResponse, error) {
					return tt.verifyResponse, tt.verifyError
				},
			}

			mockService := &mockBusinessService{}
			mockQueue := &mockEventQueue{}
			mockExtensionChecker := newMockExtensionCheckerWithX402()

			orchestrator := NewBusinessOrchestratorWithDeps(
				mockMerchant,
				mockService,
				[]types.NetworkConfig{
					{NetworkName: "eip155:84532", PayToAddress: "0x123"},
				},
				mockExtensionChecker,
			)

			task := &a2a.Task{
				ID:        "task-123",
				ContextID: "context-456",
				Status:    a2a.TaskStatus{State: a2a.TaskStateWorking, Message: a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: ""})},
			}
			paymentState := &x402state.PaymentState{
				Status:  x402state.PaymentSubmitted,
				Payload: &paymentPayload,
				Requirements: &x402types.PaymentRequired{
					X402Version: 2,
					Accepts:     []x402types.PaymentRequirements{paymentRequirements},
				},
			}

			requestContext := &a2asrv.RequestContext{
				Message:    a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: "test"}),
				StoredTask: task,
				TaskID:     "task-123",
				ContextID:  "context-456",
			}

			resultState, err := orchestrator.handlePaymentSubmitted(ctx, requestContext, task, mockQueue, paymentState)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				if task.Status.State != a2a.TaskStateFailed {
					t.Errorf("expected task state to be Failed, got %v", task.Status.State)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resultState.Status != tt.wantState {
					t.Errorf("expected payment state %v, got %v", tt.wantState, resultState.Status)
				}
			}
		})
	}
}

func TestBusinessOrchestrator_handlePaymentVerified(t *testing.T) {
	ctx := context.Background()

	paymentRequirements := x402types.PaymentRequirements{
		Scheme:  "exact",
		Network: "base-sepolia",
		PayTo:   "0x123",
		Asset:   "0x456",
	}

	paymentPayload := x402types.PaymentPayload{
		X402Version: 1,
		Accepted: x402types.PaymentRequirements{
			Scheme:  "exact",
			Network: "base-sepolia",
			Amount:  "100",
			Asset:   "0x456",
			PayTo:   "0x123",
		},
	}

	tests := []struct {
		name           string
		businessError  error
		settleResponse *x402core.SettleResponse
		settleError    error
		wantErr        bool
		wantState      x402state.PaymentStatus
		businessCalled bool
		settleCalled   bool
	}{
		{
			name:           "successful flow",
			businessError:  nil,
			settleResponse: &x402core.SettleResponse{Success: true, Network: "base-sepolia"},
			settleError:    nil,
			wantErr:        false,
			wantState:      x402state.PaymentCompleted,
			businessCalled: true,
			settleCalled:   true,
		},
		{
			name:           "business execution fails",
			businessError:  errors.New("business logic error"),
			settleResponse: nil,
			settleError:    nil,
			wantErr:        true,
			wantState:      "",
			businessCalled: true,
			settleCalled:   false,
		},
		{
			name:           "settlement fails",
			businessError:  nil,
			settleResponse: &x402core.SettleResponse{Success: false, ErrorReason: "settlement failed"},
			settleError:    nil,
			wantErr:        true,
			wantState:      "",
			businessCalled: true,
			settleCalled:   true,
		},
		{
			name:           "settlement error",
			businessError:  nil,
			settleResponse: nil,
			settleError:    errors.New("settlement error"),
			wantErr:        true,
			wantState:      "",
			businessCalled: true,
			settleCalled:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var businessCalled, settleCalled bool

			mockMerchant := &MockResourceServer{
				FindMatchingRequirementsFunc: func(accepts []x402types.PaymentRequirements, payload x402types.PaymentPayload) *x402types.PaymentRequirements {
					return &paymentRequirements
				},
				SettlePaymentFunc: func(ctx context.Context, payload x402types.PaymentPayload, requirements x402types.PaymentRequirements) (*x402core.SettleResponse, error) {
					settleCalled = true
					return tt.settleResponse, tt.settleError
				},
			}

			mockService := &mockBusinessService{
				executeFunc: func(ctx context.Context, prompt string) (string, error) {
					businessCalled = true
					return "result", tt.businessError
				},
			}

			mockExtensionChecker := newMockExtensionCheckerWithX402()

			orchestrator := NewBusinessOrchestratorWithDeps(
				mockMerchant,
				mockService,
				[]types.NetworkConfig{
					{NetworkName: "eip155:84532", PayToAddress: "0x123"},
				},
				mockExtensionChecker,
			)

			task := &a2a.Task{
				ID:        "task-123",
				ContextID: "context-456",
				Status:    a2a.TaskStatus{State: a2a.TaskStateWorking, Message: a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: ""})},
			}
			x402state.SetOriginalPrompt(task.Status.Message, "test prompt")

			paymentState := &x402state.PaymentState{
				Status:  x402state.PaymentVerified,
				Payload: &paymentPayload,
				Requirements: &x402types.PaymentRequired{
					X402Version: 2,
					Accepts:     []x402types.PaymentRequirements{paymentRequirements},
				},
			}

			resultState, err := orchestrator.handlePaymentVerified(ctx, task, paymentState)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if resultState.Status != tt.wantState {
					t.Errorf("expected payment state %v, got %v", tt.wantState, resultState.Status)
				}
			}

			if businessCalled != tt.businessCalled {
				t.Errorf("business service called = %v, want %v", businessCalled, tt.businessCalled)
			}
			if settleCalled != tt.settleCalled {
				t.Errorf("settle payment called = %v, want %v", settleCalled, tt.settleCalled)
			}
		})
	}
}

func TestBusinessOrchestrator_Execute_InitialRequest(t *testing.T) {
	ctx := context.Background()

	mockMerchant := &MockResourceServer{
		BuildPaymentRequirementsFromConfigFunc: func(ctx context.Context, config x402pkg.ResourceConfig) ([]x402types.PaymentRequirements, error) {
			return []x402types.PaymentRequirements{
				{
					Scheme:  "exact",
					Network: string(config.Network),
					PayTo:   config.PayTo,
					Asset:   "0x456",
				},
			}, nil
		},
	}

	mockService := &mockBusinessService{}
	mockQueue := &mockEventQueue{}
	mockExtensionChecker := newMockExtensionCheckerWithX402()

	orchestrator := NewBusinessOrchestratorWithDeps(
		mockMerchant,
		mockService,
		[]types.NetworkConfig{
			{NetworkName: "eip155:84532", PayToAddress: "0x123"},
		},
		mockExtensionChecker,
	)

	// Create initial request without payment state
	message := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: "I want to use the service"})
	requestContext := &a2asrv.RequestContext{
		Message:   message,
		TaskID:    "task-123",
		ContextID: "context-456",
	}

	err := orchestrator.Execute(ctx, requestContext, mockQueue)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Verify task was created
	if requestContext.StoredTask == nil {
		t.Error("expected task to be created")
	}

	// Verify payment required state
	paymentState, err := x402state.ExtractPaymentState(requestContext.StoredTask, message)
	if err != nil {
		t.Fatalf("failed to extract payment state: %v", err)
	}

	if paymentState.Status != x402state.PaymentRequired {
		t.Errorf("expected payment state to be PaymentRequired, got %v", paymentState.Status)
	}

	if paymentState.Requirements == nil || len(paymentState.Requirements.Accepts) == 0 {
		t.Error("expected payment requirements to be set")
	}

	// Verify event was written
	if len(mockQueue.events) == 0 {
		t.Error("expected events to be written to queue")
	}
}
