package agentsessions

import (
	"strings"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/platform/protostate"
)

func sessionStateFromResource(resource *platformv1alpha1.AgentSessionResource) (*agentsessionv1.AgentSessionState, error) {
	if resource == nil || resource.Spec.Session == nil {
		return nil, validation("session resource is invalid")
	}
	spec := resource.Spec.Session
	if spec.GetSessionId() == "" {
		spec.SessionId = resource.Name
	}
	if spec.GetSessionId() != resource.Name {
		return nil, validationf("session id %q does not match resource name %q", spec.GetSessionId(), resource.Name)
	}
	return &agentsessionv1.AgentSessionState{
		Generation: resource.Generation,
		Spec:       spec,
		Status:     sessionStatusFromResourceStatus(resource, &resource.Status),
	}, nil
}

// StateFromResource converts the in-memory session resource shape into the
// product-owned session state contract. It is exported for package-level
// adapters and tests that still materialize sessions as Kubernetes resources at
// their boundary.
func StateFromResource(resource *platformv1alpha1.AgentSessionResource) (*agentsessionv1.AgentSessionState, error) {
	return sessionStateFromResource(resource)
}

func sessionStatusFromResourceStatus(resource *platformv1alpha1.AgentSessionResource, status *platformv1alpha1.AgentSessionResourceStatus) *agentsessionv1.AgentSessionStatus {
	sessionID := ""
	if resource != nil {
		sessionID = firstNonEmpty(resource.Spec.Session.GetSessionId(), resource.GetName())
	}
	if status == nil {
		return &agentsessionv1.AgentSessionStatus{SessionId: sessionID}
	}
	return &agentsessionv1.AgentSessionStatus{
		SessionId:                sessionID,
		Phase:                    toProtoSessionPhase(status.Phase),
		ObservedGeneration:       status.ObservedGeneration,
		RuntimeConfigGeneration:  status.RuntimeConfigGeneration,
		ResourceConfigGeneration: status.ResourceConfigGeneration,
		StateGeneration:          status.StateGeneration,
		Message:                  status.Message,
		ActiveRun:                activeRunRef(status.ActiveRunID),
		Conditions:               protostate.Conditions(status.Conditions),
		UpdatedAt:                protostate.Timestamp(status.UpdatedAt),
		RealizedRuleRevision:     status.RealizedRuleRevision,
		RealizedSkillRevision:    status.RealizedSkillRevision,
		RealizedMcpRevision:      status.RealizedMCPRevision,
		ObservedHomeStateId:      status.ObservedHomeStateID,
	}
}

func activeRunRef(runID string) *agentsessionv1.AgentSessionActiveRunRef {
	if strings.TrimSpace(runID) == "" {
		return nil
	}
	return &agentsessionv1.AgentSessionActiveRunRef{RunId: strings.TrimSpace(runID)}
}

func toProtoSessionPhase(phase platformv1alpha1.AgentSessionResourcePhase) agentsessionv1.AgentSessionPhase {
	switch phase {
	case platformv1alpha1.AgentSessionResourcePhasePending:
		return agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_PENDING
	case platformv1alpha1.AgentSessionResourcePhaseReady:
		return agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_READY
	case platformv1alpha1.AgentSessionResourcePhaseRunning:
		return agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_RUNNING
	case platformv1alpha1.AgentSessionResourcePhaseFailed:
		return agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_FAILED
	default:
		return agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_UNSPECIFIED
	}
}
