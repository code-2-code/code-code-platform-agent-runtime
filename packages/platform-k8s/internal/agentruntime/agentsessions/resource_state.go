package agentsessions

import (
	"strings"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	conditionv1 "code-code.internal/go-contract/platform/condition/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceFromState materializes the internal session resource shape used by
// agent-runtime reconcilers from the product-owned session repository state.
func ResourceFromState(state *agentsessionv1.AgentSessionState, namespace string) (*platformv1alpha1.AgentSessionResource, error) {
	if state == nil || state.GetSpec() == nil {
		return nil, validation("session state is invalid")
	}
	sessionID := strings.TrimSpace(state.GetSpec().GetSessionId())
	if sessionID == "" {
		return nil, validation("session state session_id is empty")
	}
	return &platformv1alpha1.AgentSessionResource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: platformv1alpha1.GroupVersion.String(),
			Kind:       platformv1alpha1.KindAgentSessionResource,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       sessionID,
			Namespace:  strings.TrimSpace(namespace),
			Generation: state.GetGeneration(),
		},
		Spec: platformv1alpha1.AgentSessionResourceSpec{
			Session: state.GetSpec(),
		},
		Status: resourceStatusFromState(state.GetStatus()),
	}, nil
}

func resourceStatusFromState(status *agentsessionv1.AgentSessionStatus) platformv1alpha1.AgentSessionResourceStatus {
	if status == nil {
		return platformv1alpha1.AgentSessionResourceStatus{}
	}
	out := platformv1alpha1.AgentSessionResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: status.GetObservedGeneration(),
			Conditions:         sessionMetaConditions(status.GetConditions()),
		},
		Phase:                    resourceSessionPhase(status.GetPhase()),
		RuntimeConfigGeneration:  status.GetRuntimeConfigGeneration(),
		ResourceConfigGeneration: status.GetResourceConfigGeneration(),
		RealizedRuleRevision:     status.GetRealizedRuleRevision(),
		RealizedSkillRevision:    status.GetRealizedSkillRevision(),
		RealizedMCPRevision:      status.GetRealizedMcpRevision(),
		ObservedHomeStateID:      status.GetObservedHomeStateId(),
		StateGeneration:          status.GetStateGeneration(),
		Message:                  status.GetMessage(),
		ActiveRunID:              strings.TrimSpace(status.GetActiveRun().GetRunId()),
	}
	if status.GetUpdatedAt() != nil {
		out.UpdatedAt = timePtr(status.GetUpdatedAt().AsTime())
	}
	return out
}

func resourceSessionPhase(phase agentsessionv1.AgentSessionPhase) platformv1alpha1.AgentSessionResourcePhase {
	switch phase {
	case agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_PENDING:
		return platformv1alpha1.AgentSessionResourcePhasePending
	case agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_READY:
		return platformv1alpha1.AgentSessionResourcePhaseReady
	case agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_RUNNING:
		return platformv1alpha1.AgentSessionResourcePhaseRunning
	case agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_FAILED:
		return platformv1alpha1.AgentSessionResourcePhaseFailed
	default:
		return ""
	}
}

func sessionMetaConditions(items []*conditionv1.Condition) []metav1.Condition {
	if len(items) == 0 {
		return nil
	}
	out := make([]metav1.Condition, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		condition := metav1.Condition{
			Type:               item.GetType(),
			Status:             sessionMetaConditionStatus(item.GetStatus()),
			Reason:             item.GetReason(),
			Message:            item.GetMessage(),
			ObservedGeneration: item.GetObservedGeneration(),
		}
		if item.GetLastTransitionTime() != nil {
			condition.LastTransitionTime = metav1.NewTime(item.GetLastTransitionTime().AsTime())
		}
		out = append(out, condition)
	}
	return out
}

func sessionMetaConditionStatus(status conditionv1.ConditionStatus) metav1.ConditionStatus {
	switch status {
	case conditionv1.ConditionStatus_CONDITION_STATUS_TRUE:
		return metav1.ConditionTrue
	case conditionv1.ConditionStatus_CONDITION_STATUS_FALSE:
		return metav1.ConditionFalse
	case conditionv1.ConditionStatus_CONDITION_STATUS_UNKNOWN:
		return metav1.ConditionUnknown
	default:
		return ""
	}
}
