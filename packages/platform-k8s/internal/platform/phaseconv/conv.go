package phaseconv

import (
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
)

// Session phase conversions.

// ToK8sSessionPhase converts a proto AgentSessionPhase to K8s resource phase.
func ToK8sSessionPhase(phase agentsessionv1.AgentSessionPhase) platformv1alpha1.AgentSessionResourcePhase {
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

// FromK8sSessionPhase converts a K8s resource phase to proto AgentSessionPhase.
func FromK8sSessionPhase(phase platformv1alpha1.AgentSessionResourcePhase) agentsessionv1.AgentSessionPhase {
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

// Run phase conversions.

// ToK8sRunPhase converts a proto AgentRunPhase to K8s resource phase.
func ToK8sRunPhase(phase agentrunv1.AgentRunPhase) platformv1alpha1.AgentRunResourcePhase {
	switch phase {
	case agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_PENDING:
		return platformv1alpha1.AgentRunResourcePhasePending
	case agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_SCHEDULED:
		return platformv1alpha1.AgentRunResourcePhaseScheduled
	case agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_RUNNING:
		return platformv1alpha1.AgentRunResourcePhaseRunning
	case agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_SUCCEEDED:
		return platformv1alpha1.AgentRunResourcePhaseSucceeded
	case agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_FAILED:
		return platformv1alpha1.AgentRunResourcePhaseFailed
	case agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_CANCELED:
		return platformv1alpha1.AgentRunResourcePhaseCanceled
	default:
		return ""
	}
}

// FromK8sRunPhase converts a K8s resource phase to proto AgentRunPhase.
func FromK8sRunPhase(phase platformv1alpha1.AgentRunResourcePhase) agentrunv1.AgentRunPhase {
	switch phase {
	case platformv1alpha1.AgentRunResourcePhasePending:
		return agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_PENDING
	case platformv1alpha1.AgentRunResourcePhaseScheduled:
		return agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_SCHEDULED
	case platformv1alpha1.AgentRunResourcePhaseRunning:
		return agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_RUNNING
	case platformv1alpha1.AgentRunResourcePhaseSucceeded:
		return agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_SUCCEEDED
	case platformv1alpha1.AgentRunResourcePhaseFailed:
		return agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_FAILED
	case platformv1alpha1.AgentRunResourcePhaseCanceled:
		return agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_CANCELED
	default:
		return agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_UNSPECIFIED
	}
}

// Action phase conversions.

// ToK8sActionPhase converts a proto AgentSessionActionPhase to K8s resource phase.
func ToK8sActionPhase(phase agentsessionactionv1.AgentSessionActionPhase) platformv1alpha1.AgentSessionActionResourcePhase {
	switch phase {
	case agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_PENDING:
		return platformv1alpha1.AgentSessionActionResourcePhasePending
	case agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_RUNNING:
		return platformv1alpha1.AgentSessionActionResourcePhaseRunning
	case agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_SUCCEEDED:
		return platformv1alpha1.AgentSessionActionResourcePhaseSucceeded
	case agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_FAILED:
		return platformv1alpha1.AgentSessionActionResourcePhaseFailed
	case agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_CANCELED:
		return platformv1alpha1.AgentSessionActionResourcePhaseCanceled
	default:
		return ""
	}
}

// FromK8sActionPhase converts a K8s resource phase to proto AgentSessionActionPhase.
func FromK8sActionPhase(phase platformv1alpha1.AgentSessionActionResourcePhase) agentsessionactionv1.AgentSessionActionPhase {
	switch phase {
	case platformv1alpha1.AgentSessionActionResourcePhasePending:
		return agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_PENDING
	case platformv1alpha1.AgentSessionActionResourcePhaseRunning:
		return agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_RUNNING
	case platformv1alpha1.AgentSessionActionResourcePhaseSucceeded:
		return agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_SUCCEEDED
	case platformv1alpha1.AgentSessionActionResourcePhaseFailed:
		return agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_FAILED
	case platformv1alpha1.AgentSessionActionResourcePhaseCanceled:
		return agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_CANCELED
	default:
		return agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_UNSPECIFIED
	}
}

// Action failure class conversion.

// ToK8sActionFailureClass converts a proto AgentSessionActionFailureClass to K8s resource failure class.
func ToK8sActionFailureClass(class agentsessionactionv1.AgentSessionActionFailureClass) platformv1alpha1.AgentSessionActionResourceFailureClass {
	switch class {
	case agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_BLOCKED:
		return platformv1alpha1.AgentSessionActionResourceFailureClassBlocked
	case agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_TRANSIENT:
		return platformv1alpha1.AgentSessionActionResourceFailureClassTransient
	case agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_PERMANENT:
		return platformv1alpha1.AgentSessionActionResourceFailureClassPermanent
	case agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_MANUAL_RETRY:
		return platformv1alpha1.AgentSessionActionResourceFailureClassManualRetry
	default:
		return ""
	}
}

// FromK8sActionFailureClass converts a K8s resource failure class to proto AgentSessionActionFailureClass.
func FromK8sActionFailureClass(class platformv1alpha1.AgentSessionActionResourceFailureClass) agentsessionactionv1.AgentSessionActionFailureClass {
	switch class {
	case platformv1alpha1.AgentSessionActionResourceFailureClassBlocked:
		return agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_BLOCKED
	case platformv1alpha1.AgentSessionActionResourceFailureClassTransient:
		return agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_TRANSIENT
	case platformv1alpha1.AgentSessionActionResourceFailureClassPermanent:
		return agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_PERMANENT
	case platformv1alpha1.AgentSessionActionResourceFailureClassManualRetry:
		return agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_MANUAL_RETRY
	default:
		return agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_UNSPECIFIED
	}
}
