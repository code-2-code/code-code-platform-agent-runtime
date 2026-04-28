package agentsessionactions

import (
	"strings"

	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
)

const (
	sessionIDLabelKey = "agentsessionaction.code-code.internal/session-id"
	typeLabelKey      = "agentsessionaction.code-code.internal/type"
)

func actionLabels(sessionID string, actionType agentsessionactionv1.AgentSessionActionType) map[string]string {
	labels := map[string]string{
		sessionIDLabelKey: strings.TrimSpace(sessionID),
	}
	if actionType != agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_UNSPECIFIED {
		labels[typeLabelKey] = strings.TrimSpace(actionType.String())
	}
	return labels
}
