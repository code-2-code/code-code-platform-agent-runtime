package agentsessionactions

import (
	"context"
	"time"

	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) deriveStoppedStatus(
	ctx context.Context,
	resource *platformv1alpha1.AgentSessionActionResource,
	now time.Time,
) (*platformv1alpha1.AgentSessionActionResourceStatus, ctrl.Result, error) {
	if resource.Spec.Action == nil || resource.Spec.Action.GetType() != agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RUN_TURN {
		return canceledStatus(resource, now, "AgentSessionAction stop was requested."), ctrl.Result{}, nil
	}
	return r.deriveStoppedRunTurnStatus(ctx, resource, now)
}

func (r *Reconciler) deriveStoppedRunTurnStatus(
	ctx context.Context,
	resource *platformv1alpha1.AgentSessionActionResource,
	now time.Time,
) (*platformv1alpha1.AgentSessionActionResourceStatus, ctrl.Result, error) {
	snapshot := resource.Spec.Action.GetInputSnapshot().GetRunTurn()
	attemptCount := currentAttemptCount(resource)
	candidateIndex := currentCandidateIndex(resource, snapshot)
	if resource == nil || resource.Spec.Action == nil {
		return canceledStatus(resource, now, "AgentSessionAction stop was requested."), ctrl.Result{}, nil
	}
	if resource.Status.RunID == "" {
		return canceledStatus(resource, now, "AgentSessionAction stop was requested."), ctrl.Result{}, nil
	}
	if _, err := r.runs.Cancel(ctx, resource.Status.RunID); err != nil {
		if apierrors.IsNotFound(err) {
			return canceledStatus(resource, now, "AgentSessionAction stop was requested."), ctrl.Result{}, nil
		}
		return runningStatus(resource, now, resource.Status.RunID, "AgentSessionAction stop was requested. Waiting for run cancel acceptance.", attemptCount, candidateIndex), ctrl.Result{RequeueAfter: runningRequeueAfter}, nil
	}
	runState, err := r.runs.Get(ctx, resource.Status.RunID)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return canceledStatus(resource, now, "AgentSessionAction stop was requested."), ctrl.Result{}, nil
		}
		return runningStatus(resource, now, resource.Status.RunID, "AgentSessionAction stop was requested. Waiting for current run observation.", attemptCount, candidateIndex), ctrl.Result{RequeueAfter: runningRequeueAfter}, nil
	}
	switch runState.GetStatus().GetPhase() {
	case agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_SUCCEEDED:
		return terminalStatus(resource, now, platformv1alpha1.AgentSessionActionResourcePhaseSucceeded, runState.GetStatus().GetMessage(), ""), ctrl.Result{}, nil
	case agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_FAILED,
		agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_CANCELED:
		return canceledStatus(resource, now, "AgentSessionAction stop was requested."), ctrl.Result{}, nil
	default:
		return runningStatus(resource, now, resource.Status.RunID, "AgentSessionAction stop was requested. Current run will not be retried.", attemptCount, candidateIndex), ctrl.Result{RequeueAfter: runningRequeueAfter}, nil
	}
}
