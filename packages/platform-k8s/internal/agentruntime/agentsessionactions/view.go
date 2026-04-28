package agentsessionactions

import (
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
)

func actionViewFromResource(resource *platformv1alpha1.AgentSessionActionResource) *agentsessionactionv1.AgentSessionActionView {
	return &agentsessionactionv1.AgentSessionActionView{
		DisplayPhase: actionDisplayPhase(resource),
		CanStop:      canStopAction(resource),
		CanRetry:     canRetryAction(resource),
	}
}

func actionDisplayPhase(resource *platformv1alpha1.AgentSessionActionResource) agentsessionactionv1.AgentSessionActionDisplayPhase {
	if resource == nil || resource.Spec.Action == nil {
		return agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_UNSPECIFIED
	}
	if resource.Spec.Action.GetStopRequested() && !isTerminalPhase(resource.Status.Phase) {
		return agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_STOPPING
	}
	switch resource.Status.Phase {
	case platformv1alpha1.AgentSessionActionResourcePhasePending:
		if resource.Status.FailureClass == platformv1alpha1.AgentSessionActionResourceFailureClassTransient && resource.Status.NextRetryAt != nil {
			return agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_RETRYING
		}
		if resource.Status.AttemptCount > 0 && resource.Status.CandidateIndex > 0 {
			return agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_FALLBACKING
		}
		return agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_QUEUED
	case platformv1alpha1.AgentSessionActionResourcePhaseRunning:
		return agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_RUNNING
	case platformv1alpha1.AgentSessionActionResourcePhaseSucceeded:
		return agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_SUCCEEDED
	case platformv1alpha1.AgentSessionActionResourcePhaseFailed:
		return agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_FAILED
	case platformv1alpha1.AgentSessionActionResourcePhaseCanceled:
		return agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_STOPPED
	default:
		return agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_QUEUED
	}
}

func canStopAction(resource *platformv1alpha1.AgentSessionActionResource) bool {
	return resource != nil && resource.Spec.Action != nil && !resource.Spec.Action.GetStopRequested() && !isTerminalPhase(resource.Status.Phase)
}

func canRetryAction(resource *platformv1alpha1.AgentSessionActionResource) bool {
	if resource == nil || resource.Spec.Action == nil {
		return false
	}
	if resource.Spec.Action.GetType() != agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RUN_TURN {
		return false
	}
	switch resource.Status.Phase {
	case platformv1alpha1.AgentSessionActionResourcePhaseFailed,
		platformv1alpha1.AgentSessionActionResourcePhaseCanceled:
		return true
	default:
		return false
	}
}
