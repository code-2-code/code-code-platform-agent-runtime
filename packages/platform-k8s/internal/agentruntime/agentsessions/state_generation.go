package agentsessions

import (
	"strings"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
)

func observeStateGeneration(status *platformv1alpha1.AgentSessionResourceStatus, session *agentsessionv1.AgentSessionSpec, generation int64, warmStateReady bool) (int64, string) {
	currentHomeStateID := strings.TrimSpace(session.GetHomeStateRef().GetHomeStateId())
	if !warmStateReady || currentHomeStateID == "" {
		return 0, ""
	}
	previousGeneration := int64(0)
	previousHomeStateID := ""
	if status != nil {
		previousGeneration = status.StateGeneration
		previousHomeStateID = strings.TrimSpace(status.ObservedHomeStateID)
	}
	if previousGeneration == 0 || previousHomeStateID != currentHomeStateID {
		return generation, currentHomeStateID
	}
	return previousGeneration, currentHomeStateID
}
