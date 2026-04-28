package sessionapi

import (
	"context"
	"testing"

	corev1 "code-code.internal/go-contract/agent/core/v1"
	inputv1 "code-code.internal/go-contract/agent/input/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
)

func TestRecordAssistantTurnMessageStoresFinalAssistantOutput(t *testing.T) {
	t.Parallel()

	messages := &fakeTurnMessages{}
	server := &SessionServer{turnMessages: messages, turnOutputMessages: newAGUITurnMessageProjector()}

	err := server.recordAssistantTurnMessage(context.Background(), testOutputEvent(
		"session-1",
		"turn-1-attempt-2",
		6,
		map[string]any{"type": "TEXT_MESSAGE_START", "messageId": "assistant-message", "role": "assistant"},
	))
	if err != nil {
		t.Fatalf("recordAssistantTurnMessage(start) error = %v", err)
	}
	err = server.recordAssistantTurnMessage(context.Background(), testOutputEvent(
		"session-1",
		"turn-1-attempt-2",
		7,
		map[string]any{"type": "TEXT_MESSAGE_CONTENT", "messageId": "assistant-message", "delta": "pong"},
	))
	if err != nil {
		t.Fatalf("recordAssistantTurnMessage(content) error = %v", err)
	}
	err = server.recordAssistantTurnMessage(context.Background(), testOutputEvent(
		"session-1",
		"turn-1-attempt-2",
		8,
		map[string]any{"type": "TEXT_MESSAGE_END", "messageId": "assistant-message"},
	))
	if err != nil {
		t.Fatalf("recordAssistantTurnMessage(end) error = %v", err)
	}
	if got, want := messages.message.SessionID, "session-1"; got != want {
		t.Fatalf("session_id = %q, want %q", got, want)
	}
	if got, want := messages.message.TurnID, "turn-1"; got != want {
		t.Fatalf("turn_id = %q, want %q", got, want)
	}
	if got, want := messages.message.MessageID, "assistant-turn-1-attempt-2-assistant-message"; got != want {
		t.Fatalf("message_id = %q, want %q", got, want)
	}
	if got, want := testMessageContent(t, messages.message.Message), "pong"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestRecordUserTurnMessageStoresPrompt(t *testing.T) {
	t.Parallel()

	messages := &fakeTurnMessages{}
	server := &SessionServer{turnMessages: messages}

	err := server.recordUserTurnMessage(context.Background(), "session-1", &agentsessionactionv1.AgentSessionActionState{
		Spec: &agentsessionactionv1.AgentSessionActionSpec{ActionId: "action-1", TurnId: "turn-1"},
	}, &corev1.RunRequest{Input: &inputv1.RunInput{Text: "ping"}})
	if err != nil {
		t.Fatalf("recordUserTurnMessage() error = %v", err)
	}
	if got, want := messages.message.MessageID, "user-turn-1"; got != want {
		t.Fatalf("message_id = %q, want %q", got, want)
	}
	if got, want := testMessageRole(t, messages.message.Message), "user"; got != want {
		t.Fatalf("role = %q, want %q", got, want)
	}
}
