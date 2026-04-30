package phaseconv

import (
	"testing"

	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
)

func TestSessionPhaseRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		protoPhase agentsessionv1.AgentSessionPhase
		k8sPhase   platformv1alpha1.AgentSessionResourcePhase
	}{
		{
			name:       "pending",
			protoPhase: agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_PENDING,
			k8sPhase:   platformv1alpha1.AgentSessionResourcePhasePending,
		},
		{
			name:       "ready",
			protoPhase: agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_READY,
			k8sPhase:   platformv1alpha1.AgentSessionResourcePhaseReady,
		},
		{
			name:       "running",
			protoPhase: agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_RUNNING,
			k8sPhase:   platformv1alpha1.AgentSessionResourcePhaseRunning,
		},
		{
			name:       "failed",
			protoPhase: agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_FAILED,
			k8sPhase:   platformv1alpha1.AgentSessionResourcePhaseFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotK8s := ToK8sSessionPhase(tt.protoPhase)
			if gotK8s != tt.k8sPhase {
				t.Errorf("ToK8sSessionPhase(%v) = %q, want %q", tt.protoPhase, gotK8s, tt.k8sPhase)
			}

			gotProto := FromK8sSessionPhase(tt.k8sPhase)
			if gotProto != tt.protoPhase {
				t.Errorf("FromK8sSessionPhase(%q) = %v, want %v", tt.k8sPhase, gotProto, tt.protoPhase)
			}
		})
	}
}

func TestRunPhaseRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		protoPhase agentrunv1.AgentRunPhase
		k8sPhase   platformv1alpha1.AgentRunResourcePhase
	}{
		{
			name:       "pending",
			protoPhase: agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_PENDING,
			k8sPhase:   platformv1alpha1.AgentRunResourcePhasePending,
		},
		{
			name:       "scheduled",
			protoPhase: agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_SCHEDULED,
			k8sPhase:   platformv1alpha1.AgentRunResourcePhaseScheduled,
		},
		{
			name:       "running",
			protoPhase: agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_RUNNING,
			k8sPhase:   platformv1alpha1.AgentRunResourcePhaseRunning,
		},
		{
			name:       "succeeded",
			protoPhase: agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_SUCCEEDED,
			k8sPhase:   platformv1alpha1.AgentRunResourcePhaseSucceeded,
		},
		{
			name:       "failed",
			protoPhase: agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_FAILED,
			k8sPhase:   platformv1alpha1.AgentRunResourcePhaseFailed,
		},
		{
			name:       "canceled",
			protoPhase: agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_CANCELED,
			k8sPhase:   platformv1alpha1.AgentRunResourcePhaseCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotK8s := ToK8sRunPhase(tt.protoPhase)
			if gotK8s != tt.k8sPhase {
				t.Errorf("ToK8sRunPhase(%v) = %q, want %q", tt.protoPhase, gotK8s, tt.k8sPhase)
			}

			gotProto := FromK8sRunPhase(tt.k8sPhase)
			if gotProto != tt.protoPhase {
				t.Errorf("FromK8sRunPhase(%q) = %v, want %v", tt.k8sPhase, gotProto, tt.protoPhase)
			}
		})
	}
}

func TestActionPhaseRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		protoPhase agentsessionactionv1.AgentSessionActionPhase
		k8sPhase   platformv1alpha1.AgentSessionActionResourcePhase
	}{
		{
			name:       "pending",
			protoPhase: agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_PENDING,
			k8sPhase:   platformv1alpha1.AgentSessionActionResourcePhasePending,
		},
		{
			name:       "running",
			protoPhase: agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_RUNNING,
			k8sPhase:   platformv1alpha1.AgentSessionActionResourcePhaseRunning,
		},
		{
			name:       "succeeded",
			protoPhase: agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_SUCCEEDED,
			k8sPhase:   platformv1alpha1.AgentSessionActionResourcePhaseSucceeded,
		},
		{
			name:       "failed",
			protoPhase: agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_FAILED,
			k8sPhase:   platformv1alpha1.AgentSessionActionResourcePhaseFailed,
		},
		{
			name:       "canceled",
			protoPhase: agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_CANCELED,
			k8sPhase:   platformv1alpha1.AgentSessionActionResourcePhaseCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotK8s := ToK8sActionPhase(tt.protoPhase)
			if gotK8s != tt.k8sPhase {
				t.Errorf("ToK8sActionPhase(%v) = %q, want %q", tt.protoPhase, gotK8s, tt.k8sPhase)
			}

			gotProto := FromK8sActionPhase(tt.k8sPhase)
			if gotProto != tt.protoPhase {
				t.Errorf("FromK8sActionPhase(%q) = %v, want %v", tt.k8sPhase, gotProto, tt.protoPhase)
			}
		})
	}
}

func TestActionFailureClassRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		protoClass agentsessionactionv1.AgentSessionActionFailureClass
		k8sClass   platformv1alpha1.AgentSessionActionResourceFailureClass
	}{
		{
			name:       "blocked",
			protoClass: agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_BLOCKED,
			k8sClass:   platformv1alpha1.AgentSessionActionResourceFailureClassBlocked,
		},
		{
			name:       "transient",
			protoClass: agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_TRANSIENT,
			k8sClass:   platformv1alpha1.AgentSessionActionResourceFailureClassTransient,
		},
		{
			name:       "permanent",
			protoClass: agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_PERMANENT,
			k8sClass:   platformv1alpha1.AgentSessionActionResourceFailureClassPermanent,
		},
		{
			name:       "manual_retry",
			protoClass: agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_MANUAL_RETRY,
			k8sClass:   platformv1alpha1.AgentSessionActionResourceFailureClassManualRetry,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotK8s := ToK8sActionFailureClass(tt.protoClass)
			if gotK8s != tt.k8sClass {
				t.Errorf("ToK8sActionFailureClass(%v) = %q, want %q", tt.protoClass, gotK8s, tt.k8sClass)
			}

			gotProto := FromK8sActionFailureClass(tt.k8sClass)
			if gotProto != tt.protoClass {
				t.Errorf("FromK8sActionFailureClass(%q) = %v, want %v", tt.k8sClass, gotProto, tt.protoClass)
			}
		})
	}
}

func TestUnspecifiedAndEmptyValues(t *testing.T) {
	t.Run("session unspecified", func(t *testing.T) {
		if got := ToK8sSessionPhase(agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_UNSPECIFIED); got != "" {
			t.Errorf("ToK8sSessionPhase(UNSPECIFIED) = %q, want empty", got)
		}
		if got := FromK8sSessionPhase(""); got != agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_UNSPECIFIED {
			t.Errorf("FromK8sSessionPhase(empty) = %v, want UNSPECIFIED", got)
		}
	})

	t.Run("run unspecified", func(t *testing.T) {
		if got := ToK8sRunPhase(agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_UNSPECIFIED); got != "" {
			t.Errorf("ToK8sRunPhase(UNSPECIFIED) = %q, want empty", got)
		}
		if got := FromK8sRunPhase(""); got != agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_UNSPECIFIED {
			t.Errorf("FromK8sRunPhase(empty) = %v, want UNSPECIFIED", got)
		}
	})

	t.Run("action unspecified", func(t *testing.T) {
		if got := ToK8sActionPhase(agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_UNSPECIFIED); got != "" {
			t.Errorf("ToK8sActionPhase(UNSPECIFIED) = %q, want empty", got)
		}
		if got := FromK8sActionPhase(""); got != agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_UNSPECIFIED {
			t.Errorf("FromK8sActionPhase(empty) = %v, want UNSPECIFIED", got)
		}
	})

	t.Run("failure class unspecified", func(t *testing.T) {
		if got := ToK8sActionFailureClass(agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_UNSPECIFIED); got != "" {
			t.Errorf("ToK8sActionFailureClass(UNSPECIFIED) = %q, want empty", got)
		}
		if got := FromK8sActionFailureClass(""); got != agentsessionactionv1.AgentSessionActionFailureClass_AGENT_SESSION_ACTION_FAILURE_CLASS_UNSPECIFIED {
			t.Errorf("FromK8sActionFailureClass(empty) = %v, want UNSPECIFIED", got)
		}
	})
}
