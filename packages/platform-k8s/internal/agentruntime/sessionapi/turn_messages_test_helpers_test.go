package sessionapi

import (
	"context"
	"encoding/json"
	"testing"

	outputv1 "code-code.internal/go-contract/agent/output/v1"
	"code-code.internal/platform-k8s/internal/platform/runevents"
	sessiondomain "code-code.internal/session"
	"google.golang.org/protobuf/types/known/structpb"
)

type fakeTurnMessages struct {
	message sessiondomain.TurnMessage
}

func (f *fakeTurnMessages) UpsertTurnMessage(_ context.Context, message sessiondomain.TurnMessage) error {
	f.message = message
	return nil
}

func (f *fakeTurnMessages) ListTurnMessages(context.Context, string, int32, string) ([]sessiondomain.TurnMessage, string, error) {
	return nil, "", nil
}

func testOutputEvent(sessionID, runID string, sequence uint64, event map[string]any) runevents.OutputEvent {
	payload, err := structpb.NewStruct(event)
	if err != nil {
		panic(err)
	}
	return runevents.OutputEvent{
		SessionID: sessionID,
		RunID:     runID,
		Output:    &outputv1.RunOutput{Sequence: sequence, Event: payload},
	}
}

func testMessageRole(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var payload struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal(role) error = %v", err)
	}
	return payload.Role
}

func testMessageContent(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var payload struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal(content) error = %v", err)
	}
	return payload.Content
}

func testMessageToolCallID(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var payload struct {
		ToolCallID string `json:"toolCallId"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal(toolCallId) error = %v", err)
	}
	return payload.ToolCallID
}

func testMessageToolCalls(t *testing.T, raw json.RawMessage) []struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
} {
	t.Helper()
	var payload struct {
		ToolCalls []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"toolCalls"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal(toolCalls) error = %v", err)
	}
	return payload.ToolCalls
}
