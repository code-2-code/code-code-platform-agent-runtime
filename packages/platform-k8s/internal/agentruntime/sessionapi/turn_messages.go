package sessionapi

import (
	"context"
	"strings"

	corev1 "code-code.internal/go-contract/agent/core/v1"
	"code-code.internal/go-contract/agui"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	"code-code.internal/platform-k8s/internal/platform/runevents"
	sessiondomain "code-code.internal/session"
	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

type textTurnMessageInput struct {
	sessionID string
	turnID    string
	runID     string
	messageID string
	role      string
	text      string
	sequence  int64
}

type assistantTurnMessageInput struct {
	sessionID string
	turnID    string
	runID     string
	messageID string
	text      string
	toolCalls []aguitypes.ToolCall
	sequence  int64
}

type toolTurnMessageInput struct {
	sessionID  string
	turnID     string
	runID      string
	messageID  string
	toolCallID string
	content    string
	sequence   int64
}

type aguiTurnMessageInput struct {
	sessionID string
	turnID    string
	runID     string
	messageID string
	message   aguitypes.Message
	sequence  int64
}

func (s *SessionServer) recordUserTurnMessage(ctx context.Context, sessionID string, action *agentsessionactionv1.AgentSessionActionState, request *corev1.RunRequest) error {
	if s == nil || s.turnMessages == nil || request == nil {
		return nil
	}
	text := strings.TrimSpace(request.GetInput().GetText())
	if text == "" {
		return nil
	}
	turnID := actionTurnID(action)
	if turnID == "" {
		return nil
	}
	runID := actionID(action)
	if runID == "" {
		runID = turnID
	}
	message, err := newTextTurnMessage(textTurnMessageInput{
		sessionID: sessionID,
		turnID:    turnID,
		runID:     runID,
		messageID: "user-" + turnID,
		role:      string(aguitypes.RoleUser),
		text:      text,
	})
	if err != nil {
		return err
	}
	return s.turnMessages.UpsertTurnMessage(ctx, message)
}

func (s *SessionServer) recordAssistantTurnMessage(ctx context.Context, event runevents.OutputEvent) error {
	if s == nil || s.turnMessages == nil || event.Output == nil {
		return nil
	}
	projector := s.turnOutputMessages
	if projector == nil {
		return nil
	}
	message, ok, err := projector.Apply(event)
	if err != nil || !ok {
		return err
	}
	return s.turnMessages.UpsertTurnMessage(ctx, message)
}

func actionTurnID(action *agentsessionactionv1.AgentSessionActionState) string {
	if action == nil || action.GetSpec() == nil {
		return ""
	}
	if turnID := strings.TrimSpace(action.GetSpec().GetTurnId()); turnID != "" {
		return turnID
	}
	return strings.TrimSpace(action.GetSpec().GetActionId())
}

func actionID(action *agentsessionactionv1.AgentSessionActionState) string {
	if action == nil || action.GetSpec() == nil {
		return ""
	}
	return strings.TrimSpace(action.GetSpec().GetActionId())
}

func newTextTurnMessage(input textTurnMessageInput) (sessiondomain.TurnMessage, error) {
	payload, err := agui.TextMessage(input.messageID, input.role, input.text)
	if err != nil {
		return sessiondomain.TurnMessage{}, err
	}
	return newAGUITurnMessage(aguiTurnMessageInput{
		sessionID: input.sessionID,
		turnID:    input.turnID,
		runID:     input.runID,
		messageID: input.messageID,
		message:   payload,
		sequence:  input.sequence,
	})
}

func newAssistantTurnMessage(input assistantTurnMessageInput) (sessiondomain.TurnMessage, error) {
	payload, err := agui.AssistantMessage(input.messageID, input.text, input.toolCalls)
	if err != nil {
		return sessiondomain.TurnMessage{}, err
	}
	return newAGUITurnMessage(aguiTurnMessageInput{
		sessionID: input.sessionID,
		turnID:    input.turnID,
		runID:     input.runID,
		messageID: input.messageID,
		message:   payload,
		sequence:  input.sequence,
	})
}

func newToolTurnMessage(input toolTurnMessageInput) (sessiondomain.TurnMessage, error) {
	payload, err := agui.ToolMessage(input.messageID, input.toolCallID, input.content)
	if err != nil {
		return sessiondomain.TurnMessage{}, err
	}
	return newAGUITurnMessage(aguiTurnMessageInput{
		sessionID: input.sessionID,
		turnID:    input.turnID,
		runID:     input.runID,
		messageID: input.messageID,
		message:   payload,
		sequence:  input.sequence,
	})
}

func newAGUITurnMessage(input aguiTurnMessageInput) (sessiondomain.TurnMessage, error) {
	message, err := agui.MessageRaw(input.message)
	if err != nil {
		return sessiondomain.TurnMessage{}, err
	}
	return sessiondomain.NormalizeTurnMessage(sessiondomain.TurnMessage{
		SessionID: input.sessionID,
		TurnID:    input.turnID,
		RunID:     input.runID,
		MessageID: input.messageID,
		Message:   message,
		Sequence:  input.sequence,
	})
}
