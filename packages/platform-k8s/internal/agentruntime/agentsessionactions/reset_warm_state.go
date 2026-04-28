package agentsessionactions

import (
	"context"
	"strings"
	"time"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"google.golang.org/protobuf/proto"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) deriveResetWarmStateStatus(ctx context.Context, resource *platformv1alpha1.AgentSessionActionResource, now time.Time) (*platformv1alpha1.AgentSessionActionResourceStatus, ctrl.Result, error) {
	snapshot := resource.Spec.Action.GetInputSnapshot().GetResetWarmState()
	if snapshot == nil {
		return failedStatus(resource, now, "AgentSessionAction reset_warm_state requires input_snapshot.reset_warm_state."), ctrl.Result{}, nil
	}
	session, err := r.loadSession(ctx, resource.Spec.Action.GetSessionId())
	if err != nil {
		if apierrors.IsNotFound(err) {
			return failedStatus(resource, now, "AgentSession referenced session no longer exists."), ctrl.Result{}, nil
		}
		status, result := scheduleRetryStatus(resource, now, err.Error(), platformv1alpha1.AgentSessionActionResourceFailureClassPermanent, r.retryPolicy)
		return status, result, nil
	}
	if resetWarmStateSuperseded(session, snapshot) {
		return terminalStatus(resource, now, platformv1alpha1.AgentSessionActionResourcePhaseCanceled, canceledResetWarmStateMessage(), ""), ctrl.Result{}, nil
	}
	if resetWarmStateApplied(session, snapshot) {
		return terminalStatus(resource, now, platformv1alpha1.AgentSessionActionResourcePhaseSucceeded, completedResetWarmStateMessage(), ""), ctrl.Result{}, nil
	}
	if resource.Status.Phase != platformv1alpha1.AgentSessionActionResourcePhaseRunning {
		if err := r.applyResetWarmState(ctx, session.GetName(), snapshot); err != nil {
			status, result := scheduleRetryStatus(resource, now, err.Error(), platformv1alpha1.AgentSessionActionResourceFailureClassPermanent, r.retryPolicy)
			return status, result, nil
		}
		return runningStatus(resource, now, "", pendingResetWarmStateMessage(), resource.Status.AttemptCount, resource.Status.CandidateIndex), ctrl.Result{Requeue: true}, nil
	}
	return runningStatus(resource, now, "", pendingResetWarmStateMessage(), resource.Status.AttemptCount, resource.Status.CandidateIndex), ctrl.Result{RequeueAfter: runningRequeueAfter}, nil
}

func resetWarmStateSuperseded(session *platformv1alpha1.AgentSessionResource, snapshot *agentsessionactionv1.AgentSessionResetWarmStateSnapshot) bool {
	if session == nil || session.Spec.Session == nil || snapshot == nil {
		return true
	}
	current := strings.TrimSpace(session.Spec.Session.GetHomeStateRef().GetHomeStateId())
	source := strings.TrimSpace(snapshot.GetSourceHomeStateId())
	target := strings.TrimSpace(snapshot.GetTargetHomeStateId())
	return current != source && current != target
}

func resetWarmStateApplied(session *platformv1alpha1.AgentSessionResource, snapshot *agentsessionactionv1.AgentSessionResetWarmStateSnapshot) bool {
	if session == nil || session.Spec.Session == nil || snapshot == nil {
		return false
	}
	target := strings.TrimSpace(snapshot.GetTargetHomeStateId())
	if strings.TrimSpace(session.Spec.Session.GetHomeStateRef().GetHomeStateId()) != target {
		return false
	}
	if strings.TrimSpace(session.Status.ObservedHomeStateID) != target {
		return false
	}
	return warmStateReady(session) && session.Status.StateGeneration > 0
}

func (r *Reconciler) applyResetWarmState(ctx context.Context, sessionID string, snapshot *agentsessionactionv1.AgentSessionResetWarmStateSnapshot) error {
	state, err := r.sessions.Get(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return err
	}
	if state.GetSpec() == nil {
		return validationf("session %q is missing payload", sessionID)
	}
	next := proto.Clone(state.GetSpec()).(*agentsessionv1.AgentSessionSpec)
	next.HomeStateRef = &agentsessionv1.AgentSessionHomeStateRef{
		HomeStateId: strings.TrimSpace(snapshot.GetTargetHomeStateId()),
	}
	_, err = r.sessions.Update(ctx, sessionID, next)
	return err
}

func pendingResetWarmStateMessage() string {
	return "AgentSession warm state reset is waiting for new carrier observation."
}

func completedResetWarmStateMessage() string {
	return "AgentSession warm state reset completed."
}

func canceledResetWarmStateMessage() string {
	return "AgentSession warm state reset was superseded by a newer carrier."
}
