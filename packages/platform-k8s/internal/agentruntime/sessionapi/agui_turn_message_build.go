package sessionapi

import (
	"regexp"
	"strings"

	"code-code.internal/platform-k8s/internal/platform/runevents"
	sessiondomain "code-code.internal/session"
	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

var runAttemptSuffix = regexp.MustCompile(`-attempt-[0-9]+$`)

func buildAGUITurnMessage(input aguiTextMessage) (sessiondomain.TurnMessage, bool, error) {
	if strings.TrimSpace(input.text) == "" && len(input.toolCalls) == 0 {
		return sessiondomain.TurnMessage{}, false, nil
	}
	runID := strings.TrimSpace(input.runID)
	if runID == "" {
		return sessiondomain.TurnMessage{}, false, nil
	}
	messageID := persistentAGUIMessageID(input.role, runID, input.messageID)
	if input.role == string(aguitypes.RoleAssistant) {
		message, err := newAssistantTurnMessage(assistantTurnMessageInput{
			sessionID: input.sessionID,
			turnID:    turnIDFromRunID(runID),
			runID:     runID,
			messageID: messageID,
			text:      input.text,
			toolCalls: input.toolCalls,
			sequence:  input.sequence,
		})
		return message, true, err
	}
	message, err := newTextTurnMessage(textTurnMessageInput{
		sessionID: input.sessionID,
		turnID:    turnIDFromRunID(runID),
		runID:     runID,
		messageID: messageID,
		role:      input.role,
		text:      input.text,
		sequence:  input.sequence,
	})
	return message, true, err
}

func aguiTextMessageKey(event runevents.OutputEvent, messageID string) string {
	return strings.TrimSpace(event.SessionID) + "/" + strings.TrimSpace(event.RunID) + "/" + strings.TrimSpace(messageID)
}

func persistentAGUIMessageID(role, runID, messageID string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		role = string(aguitypes.RoleAssistant)
	}
	return role + "-" + strings.TrimSpace(runID) + "-" + strings.TrimSpace(messageID)
}

func turnIDFromRunID(runID string) string {
	return runAttemptSuffix.ReplaceAllString(strings.TrimSpace(runID), "")
}
