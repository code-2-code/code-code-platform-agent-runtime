package agentsessionactions

import (
	"strings"

	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	"code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/platform/protostate"
)

func actionStateFromResource(resource *v1alpha1.AgentSessionActionResource) (*agentsessionactionv1.AgentSessionActionState, error) {
	if resource == nil || resource.Spec.Action == nil {
		return nil, validation("action resource is invalid")
	}
	spec := cloneActionSpec(resource.Spec.Action)
	if spec.GetActionId() == "" {
		spec.ActionId = resource.Name
	}
	return &agentsessionactionv1.AgentSessionActionState{
		Generation: resource.Generation,
		Spec:       spec,
		Status: &agentsessionactionv1.AgentSessionActionStatus{
			ActionId:           spec.GetActionId(),
			Phase:              toProtoActionPhase(resource.Status.Phase),
			ObservedGeneration: resource.Status.ObservedGeneration,
			Message:            resource.Status.Message,
			Run:                actionRunRef(resource.Status.RunID),
			CreatedAt:          protostate.Timestamp(createdAt(resource)),
			UpdatedAt:          protostate.Timestamp(resource.Status.UpdatedAt),
			FailureClass:       toProtoActionFailureClass(resource.Status.FailureClass),
			RetryCount:         resource.Status.RetryCount,
			NextRetryAt:        protostate.Timestamp(resource.Status.NextRetryAt),
			View:               actionViewFromResource(resource),
			AttemptCount:       resource.Status.AttemptCount,
			CandidateIndex:     resource.Status.CandidateIndex,
		},
	}, nil
}

func actionRunRef(runID string) *agentsessionactionv1.AgentSessionActionRunRef {
	if strings.TrimSpace(runID) == "" {
		return nil
	}
	return &agentsessionactionv1.AgentSessionActionRunRef{RunId: strings.TrimSpace(runID)}
}

func toProtoActionPhase(phase v1alpha1.AgentSessionActionResourcePhase) agentsessionactionv1.AgentSessionActionPhase {
	switch phase {
	case v1alpha1.AgentSessionActionResourcePhasePending:
		return agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_PENDING
	case v1alpha1.AgentSessionActionResourcePhaseRunning:
		return agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_RUNNING
	case v1alpha1.AgentSessionActionResourcePhaseSucceeded:
		return agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_SUCCEEDED
	case v1alpha1.AgentSessionActionResourcePhaseFailed:
		return agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_FAILED
	case v1alpha1.AgentSessionActionResourcePhaseCanceled:
		return agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_CANCELED
	default:
		return agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_UNSPECIFIED
	}
}

func toProtoActionFailureClass(class v1alpha1.AgentSessionActionResourceFailureClass) agentsessionactionv1.AgentSessionActionFailureClass {
	switch class {
	case v1alpha1.AgentSessionActionResourceFailureClassBlocked:
		return agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_BLOCKED
	case v1alpha1.AgentSessionActionResourceFailureClassTransient:
		return agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_TRANSIENT
	case v1alpha1.AgentSessionActionResourceFailureClassPermanent:
		return agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_PERMANENT
	case v1alpha1.AgentSessionActionResourceFailureClassManualRetry:
		return agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_MANUAL_RETRY
	default:
		return agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_UNSPECIFIED
	}
}
