package sessionapi

import (
	"context"
	"testing"

	"code-code.internal/platform-k8s/internal/platform/runevents"
)

func TestRecordAssistantTurnMessageStoresToolResult(t *testing.T) {
	t.Parallel()

	messages := &fakeTurnMessages{}
	server := &SessionServer{turnMessages: messages, turnOutputMessages: newAGUITurnMessageProjector()}

	err := server.recordAssistantTurnMessage(context.Background(), testOutputEvent(
		"session-1",
		"turn-1-attempt-2",
		9,
		map[string]any{
			"type":       "TOOL_CALL_RESULT",
			"messageId":  "tool-message-tool-1",
			"toolCallId": "tool-1",
			"content":    `{"summary":"ls -la"}`,
		},
	))
	if err != nil {
		t.Fatalf("recordAssistantTurnMessage(tool result) error = %v", err)
	}
	if got, want := messages.message.SessionID, "session-1"; got != want {
		t.Fatalf("session_id = %q, want %q", got, want)
	}
	if got, want := messages.message.TurnID, "turn-1"; got != want {
		t.Fatalf("turn_id = %q, want %q", got, want)
	}
	if got, want := messages.message.MessageID, "tool-turn-1-attempt-2-tool-message-tool-1"; got != want {
		t.Fatalf("message_id = %q, want %q", got, want)
	}
	if got, want := testMessageRole(t, messages.message.Message), "tool"; got != want {
		t.Fatalf("role = %q, want %q", got, want)
	}
	if got, want := testMessageToolCallID(t, messages.message.Message), "tool-1"; got != want {
		t.Fatalf("toolCallId = %q, want %q", got, want)
	}
	if got, want := testMessageContent(t, messages.message.Message), `{"summary":"ls -la"}`; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestRecordAssistantTurnMessageStoresAssistantToolCall(t *testing.T) {
	t.Parallel()

	messages := &fakeTurnMessages{}
	server := &SessionServer{turnMessages: messages, turnOutputMessages: newAGUITurnMessageProjector()}

	events := []runevents.OutputEvent{
		testOutputEvent("session-1", "turn-1-attempt-2", 6, map[string]any{
			"type":      "TEXT_MESSAGE_START",
			"messageId": "assistant-message",
			"role":      "assistant",
		}),
		testOutputEvent("session-1", "turn-1-attempt-2", 7, map[string]any{
			"type":      "TEXT_MESSAGE_CONTENT",
			"messageId": "assistant-message",
			"delta":     "checking",
		}),
		testOutputEvent("session-1", "turn-1-attempt-2", 8, map[string]any{
			"type":      "TEXT_MESSAGE_END",
			"messageId": "assistant-message",
		}),
		testOutputEvent("session-1", "turn-1-attempt-2", 9, map[string]any{
			"type":            "TOOL_CALL_START",
			"toolCallId":      "tool-1",
			"toolCallName":    "shell",
			"parentMessageId": "assistant-message",
		}),
		testOutputEvent("session-1", "turn-1-attempt-2", 10, map[string]any{
			"type":       "TOOL_CALL_ARGS",
			"toolCallId": "tool-1",
			"delta":      `{"cmd":"ls"}`,
		}),
		testOutputEvent("session-1", "turn-1-attempt-2", 11, map[string]any{
			"type":       "TOOL_CALL_END",
			"toolCallId": "tool-1",
		}),
	}
	for _, event := range events {
		if err := server.recordAssistantTurnMessage(context.Background(), event); err != nil {
			t.Fatalf("recordAssistantTurnMessage(%s) error = %v", event.Output.GetEvent().GetFields()["type"].GetStringValue(), err)
		}
	}
	if got, want := messages.message.MessageID, "assistant-turn-1-attempt-2-assistant-message"; got != want {
		t.Fatalf("message_id = %q, want %q", got, want)
	}
	if got, want := testMessageContent(t, messages.message.Message), "checking"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	toolCalls := testMessageToolCalls(t, messages.message.Message)
	if len(toolCalls) != 1 {
		t.Fatalf("tool_calls length = %d, want 1", len(toolCalls))
	}
	if got, want := toolCalls[0].ID, "tool-1"; got != want {
		t.Fatalf("tool_call id = %q, want %q", got, want)
	}
	if got, want := toolCalls[0].Type, "function"; got != want {
		t.Fatalf("tool_call type = %q, want %q", got, want)
	}
	if got, want := toolCalls[0].Function.Name, "shell"; got != want {
		t.Fatalf("tool_call function.name = %q, want %q", got, want)
	}
	if got, want := toolCalls[0].Function.Arguments, `{"cmd":"ls"}`; got != want {
		t.Fatalf("tool_call function.arguments = %q, want %q", got, want)
	}
}

func TestRecordAssistantTurnMessageStoresToolCallChunk(t *testing.T) {
	t.Parallel()

	messages := &fakeTurnMessages{}
	server := &SessionServer{turnMessages: messages, turnOutputMessages: newAGUITurnMessageProjector()}

	events := []runevents.OutputEvent{
		testOutputEvent("session-1", "turn-1-attempt-2", 6, map[string]any{
			"type":      "TEXT_MESSAGE_START",
			"messageId": "assistant-message",
			"role":      "assistant",
		}),
		testOutputEvent("session-1", "turn-1-attempt-2", 7, map[string]any{
			"type":            "TOOL_CALL_CHUNK",
			"toolCallId":      "tool-1",
			"toolCallName":    "shell",
			"parentMessageId": "assistant-message",
			"delta":           `{"cmd":`,
		}),
		testOutputEvent("session-1", "turn-1-attempt-2", 8, map[string]any{
			"type":       "TOOL_CALL_CHUNK",
			"toolCallId": "tool-1",
			"delta":      `"ls"}`,
		}),
	}
	for _, event := range events {
		if err := server.recordAssistantTurnMessage(context.Background(), event); err != nil {
			t.Fatalf("recordAssistantTurnMessage(%s) error = %v", event.Output.GetEvent().GetFields()["type"].GetStringValue(), err)
		}
	}
	if got, want := messages.message.MessageID, "assistant-turn-1-attempt-2-assistant-message"; got != want {
		t.Fatalf("message_id = %q, want %q", got, want)
	}
	toolCalls := testMessageToolCalls(t, messages.message.Message)
	if len(toolCalls) != 1 {
		t.Fatalf("tool_calls length = %d, want 1", len(toolCalls))
	}
	if got, want := toolCalls[0].Function.Name, "shell"; got != want {
		t.Fatalf("tool_call function.name = %q, want %q", got, want)
	}
	if got, want := toolCalls[0].Function.Arguments, `{"cmd":"ls"}`; got != want {
		t.Fatalf("tool_call function.arguments = %q, want %q", got, want)
	}
}
