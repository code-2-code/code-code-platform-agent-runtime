package agentsessionactions

import (
	"testing"
	"time"

	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestActionViewFromResource(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 17, 20, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		resource *platformv1alpha1.AgentSessionActionResource
		display  agentsessionactionv1.AgentSessionActionDisplayPhase
		canStop  bool
		canRetry bool
	}{
		{
			name:     "pending action is queued",
			resource: pendingActionResource("action-queued", now),
			display:  agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_QUEUED,
			canStop:  true,
		},
		{
			name: "transient pending action is retrying",
			resource: func() *platformv1alpha1.AgentSessionActionResource {
				action := pendingActionResource("action-retrying", now)
				action.Status = platformv1alpha1.AgentSessionActionResourceStatus{
					CommonStatusFields: platformv1alpha1.CommonStatusFields{ObservedGeneration: 1},
					Phase:              platformv1alpha1.AgentSessionActionResourcePhasePending,
					FailureClass:       platformv1alpha1.AgentSessionActionResourceFailureClassTransient,
					RetryCount:         2,
					NextRetryAt:        &metav1.Time{Time: now.Add(time.Minute)},
				}
				return action
			}(),
			display: agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_RETRYING,
			canStop: true,
		},
		{
			name: "fallback pending action is fallbacking",
			resource: func() *platformv1alpha1.AgentSessionActionResource {
				action := pendingActionResource("action-fallbacking", now)
				action.Status = platformv1alpha1.AgentSessionActionResourceStatus{
					CommonStatusFields: platformv1alpha1.CommonStatusFields{ObservedGeneration: 1},
					Phase:              platformv1alpha1.AgentSessionActionResourcePhasePending,
					FailureClass:       platformv1alpha1.AgentSessionActionResourceFailureClassTransient,
					AttemptCount:       1,
					CandidateIndex:     1,
				}
				return action
			}(),
			display: agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_FALLBACKING,
			canStop: true,
		},
		{
			name: "stop requested action is stopping",
			resource: func() *platformv1alpha1.AgentSessionActionResource {
				action := pendingActionResource("action-stopping", now)
				action.Spec.Action.StopRequested = true
				action.Status = platformv1alpha1.AgentSessionActionResourceStatus{
					CommonStatusFields: platformv1alpha1.CommonStatusFields{ObservedGeneration: 1},
					Phase:              platformv1alpha1.AgentSessionActionResourcePhaseRunning,
					RunID:              "action-stopping",
				}
				return action
			}(),
			display: agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_STOPPING,
		},
		{
			name: "terminal run_turn action can retry",
			resource: func() *platformv1alpha1.AgentSessionActionResource {
				action := pendingActionResource("action-failed", now)
				action.Status = platformv1alpha1.AgentSessionActionResourceStatus{
					CommonStatusFields: platformv1alpha1.CommonStatusFields{ObservedGeneration: 1},
					Phase:              platformv1alpha1.AgentSessionActionResourcePhaseFailed,
					FailureClass:       platformv1alpha1.AgentSessionActionResourceFailureClassManualRetry,
				}
				return action
			}(),
			display:  agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_FAILED,
			canRetry: true,
		},
		{
			name: "stopped reload action cannot retry",
			resource: func() *platformv1alpha1.AgentSessionActionResource {
				action := pendingReloadSubjectAction(
					"action-stopped",
					9,
					agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_SKILL,
					resourceConfigWithSubjects("resources-v2"),
					now,
				)
				action.Status = platformv1alpha1.AgentSessionActionResourceStatus{
					CommonStatusFields: platformv1alpha1.CommonStatusFields{ObservedGeneration: 1},
					Phase:              platformv1alpha1.AgentSessionActionResourcePhaseCanceled,
				}
				return action
			}(),
			display: agentsessionactionv1.AgentSessionActionDisplayPhase_AGENT_SESSION_ACTION_DISPLAY_PHASE_STOPPED,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			view := actionViewFromResource(tt.resource)
			if got, want := view.GetDisplayPhase(), tt.display; got != want {
				t.Fatalf("display_phase = %v, want %v", got, want)
			}
			if got, want := view.GetCanStop(), tt.canStop; got != want {
				t.Fatalf("can_stop = %t, want %t", got, want)
			}
			if got, want := view.GetCanRetry(), tt.canRetry; got != want {
				t.Fatalf("can_retry = %t, want %t", got, want)
			}
		})
	}
}

func TestActionStateIncludesView(t *testing.T) {
	t.Parallel()

	action := pendingActionResource("action-1", time.Date(2026, 4, 17, 20, 0, 0, 0, time.UTC))
	action.Status = platformv1alpha1.AgentSessionActionResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{ObservedGeneration: 1},
		Phase:              platformv1alpha1.AgentSessionActionResourcePhaseFailed,
		FailureClass:       platformv1alpha1.AgentSessionActionResourceFailureClassManualRetry,
		AttemptCount:       2,
		CandidateIndex:     1,
	}

	state, err := actionStateFromResource(action)
	if err != nil {
		t.Fatalf("actionStateFromResource() error = %v", err)
	}
	if state.GetStatus().GetView() == nil {
		t.Fatal("view = nil, want derived view")
	}
	if got, want := state.GetStatus().GetView().GetCanRetry(), true; got != want {
		t.Fatalf("can_retry = %t, want %t", got, want)
	}
	if got, want := state.GetStatus().GetAttemptCount(), int32(2); got != want {
		t.Fatalf("attempt_count = %d, want %d", got, want)
	}
	if got, want := state.GetStatus().GetCandidateIndex(), int32(1); got != want {
		t.Fatalf("candidate_index = %d, want %d", got, want)
	}
}
