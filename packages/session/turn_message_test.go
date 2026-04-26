package session

import (
	"encoding/json"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNormalizeTurnMessageStoresAGUIMessage(t *testing.T) {
	raw, err := json.Marshal(map[string]string{
		"id":      "message-1",
		"role":    "user",
		"content": "hello",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	message, err := NormalizeTurnMessage(TurnMessage{
		SessionID: " session-1 ",
		TurnID:    " turn-1 ",
		RunID:     " run-1 ",
		Message:   raw,
	})
	if err != nil {
		t.Fatalf("NormalizeTurnMessage() error = %v", err)
	}
	if message.SessionID != "session-1" || message.TurnID != "turn-1" || message.RunID != "run-1" || message.MessageID != "message-1" {
		t.Fatalf("message identifiers were not normalized: %+v", message)
	}
	var stored struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(message.Message, &stored); err != nil {
		t.Fatalf("message is not valid json: %v", err)
	}
	if stored.Content != "hello" {
		t.Fatalf("content = %q, want hello", stored.Content)
	}
}

func TestNormalizeTurnMessageRejectsInvalidMessage(t *testing.T) {
	_, err := NormalizeTurnMessage(TurnMessage{
		SessionID: "session-1",
		Message:   []byte("{"),
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("NormalizeTurnMessage() code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestNormalizeTurnMessageRequiresAGUIMessageID(t *testing.T) {
	_, err := NormalizeTurnMessage(TurnMessage{
		SessionID: "session-1",
		Message:   []byte(`{"role":"assistant","content":"hello"}`),
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("NormalizeTurnMessage() code = %v, want InvalidArgument", status.Code(err))
	}
}
