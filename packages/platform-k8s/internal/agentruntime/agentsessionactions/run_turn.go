package agentsessionactions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	resultv1 "code-code.internal/go-contract/agent/result/v1"
	domainerror "code-code.internal/go-contract/domainerror"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentruns"
	"google.golang.org/protobuf/proto"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) deriveRunTurnStatus(ctx context.Context, resource *platformv1alpha1.AgentSessionActionResource, now time.Time) (*platformv1alpha1.AgentSessionActionResourceStatus, ctrl.Result, error) {
	snapshot := resource.Spec.Action.GetInputSnapshot().GetRunTurn()
	if snapshot == nil {
		return failedStatus(resource, now, "AgentSessionAction run_turn requires input_snapshot.run_turn."), ctrl.Result{}, nil
	}
	if _, err := r.loadSession(ctx, resource.Spec.Action.GetSessionId()); err != nil {
		if apierrors.IsNotFound(err) {
			return failedStatus(resource, now, "AgentSession referenced session no longer exists."), ctrl.Result{}, nil
		}
		status, result := scheduleRetryStatus(resource, now, err.Error(), platformv1alpha1.AgentSessionActionResourceFailureClassManualRetry, r.retryPolicy)
		return status, result, nil
	}
	if strings.TrimSpace(resource.Status.RunID) == "" {
		runState, attemptCount, err := r.createOrGetRun(ctx, resource, snapshot)
		if err != nil {
			var validation *domainerror.ValidationError
			if errors.As(err, &validation) {
				return pendingBlockedStatus(resource, now, validation.Error()), ctrl.Result{}, nil
			}
			status, result := scheduleRetryStatus(resource, now, err.Error(), platformv1alpha1.AgentSessionActionResourceFailureClassManualRetry, r.retryPolicy)
			return status, result, nil
		}
		return runningStatus(resource, now, runState.GetSpec().GetRunId(), runState.GetStatus().GetMessage(), attemptCount, currentCandidateIndex(resource, snapshot)), ctrl.Result{RequeueAfter: runningRequeueAfter}, nil
	}
	runState, err := r.runs.Get(ctx, resource.Status.RunID)
	if err != nil {
		if apierrors.IsNotFound(err) {
			runState, attemptCount, createErr := r.createOrGetRun(ctx, resource, snapshot)
			if createErr == nil {
				return runningStatus(resource, now, runState.GetSpec().GetRunId(), runState.GetStatus().GetMessage(), attemptCount, currentCandidateIndex(resource, snapshot)), ctrl.Result{RequeueAfter: runningRequeueAfter}, nil
			}
			status, result := scheduleRetryStatus(resource, now, "AgentSessionAction run is missing and will be recreated.", platformv1alpha1.AgentSessionActionResourceFailureClassManualRetry, r.retryPolicy)
			return status, result, nil
		}
		status, result := scheduleRetryStatus(resource, now, err.Error(), platformv1alpha1.AgentSessionActionResourceFailureClassManualRetry, r.retryPolicy)
		return status, result, nil
	}
	switch runState.GetStatus().GetPhase() {
	case agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_SUCCEEDED:
		return terminalStatus(resource, now, platformv1alpha1.AgentSessionActionResourcePhaseSucceeded, runState.GetStatus().GetMessage(), ""), ctrl.Result{}, nil
	case agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_FAILED:
		return failedRunTurnStatus(resource, now, snapshot, runState, r.retryPolicy), ctrl.Result{}, nil
	case agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_CANCELED:
		return terminalStatus(resource, now, platformv1alpha1.AgentSessionActionResourcePhaseCanceled, runState.GetStatus().GetMessage(), ""), ctrl.Result{}, nil
	default:
		return runningStatus(resource, now, resource.Status.RunID, runState.GetStatus().GetMessage(), currentAttemptCount(resource), currentCandidateIndex(resource, snapshot)), ctrl.Result{RequeueAfter: runningRequeueAfter}, nil
	}
}

func failedRunTurnStatus(
	resource *platformv1alpha1.AgentSessionActionResource,
	now time.Time,
	snapshot *agentsessionactionv1.AgentSessionRunTurnSnapshot,
	runState *agentrunv1.AgentRunState,
	retryPolicy RetryPolicy,
) *platformv1alpha1.AgentSessionActionResourceStatus {
	message := runFailureMessage(runState)
	if runResultIsRetryable(runState.GetStatus().GetResult()) {
		if retryBudgetAvailable(resource, retryPolicy) {
			status, _ := scheduleRetryStatus(resource, now, message, platformv1alpha1.AgentSessionActionResourceFailureClassTransient, retryPolicy)
			return status
		}
		if nextCandidateIndex, ok := nextCandidateIndex(resource, snapshot); ok {
			return scheduleFallbackStatus(resource, now, message, nextCandidateIndex)
		}
		return terminalStatus(resource, now, platformv1alpha1.AgentSessionActionResourcePhaseFailed, message, platformv1alpha1.AgentSessionActionResourceFailureClassManualRetry)
	}
	return terminalStatus(resource, now, platformv1alpha1.AgentSessionActionResourcePhaseFailed, message, platformv1alpha1.AgentSessionActionResourceFailureClassPermanent)
}

func runFailureMessage(runState *agentrunv1.AgentRunState) string {
	if runState == nil {
		return ""
	}
	if result := runState.GetStatus().GetResult(); result != nil && result.GetError() != nil {
		if message := strings.TrimSpace(result.GetError().GetMessage()); message != "" {
			return message
		}
	}
	return runState.GetStatus().GetMessage()
}

func runResultIsRetryable(result *resultv1.RunResult) bool {
	return result != nil && result.GetError() != nil && result.GetError().GetRetryable()
}

func (r *Reconciler) createOrGetRun(ctx context.Context, resource *platformv1alpha1.AgentSessionActionResource, snapshot *agentsessionactionv1.AgentSessionRunTurnSnapshot) (*agentrunv1.AgentRunState, int32, error) {
	runID, attemptCount := nextRunAttempt(resource)
	selectedSnapshot, err := snapshotForCandidate(snapshot, currentCandidateIndex(resource, snapshot), runID)
	if err != nil {
		return nil, attemptCount, err
	}
	runState, err := r.runs.Create(ctx, resource.Spec.Action.GetSessionId(), &agentruns.CreateRequest{
		RunID:    runID,
		Snapshot: selectedSnapshot,
	})
	if err == nil {
		return runState, attemptCount, nil
	}
	var alreadyExists *domainerror.AlreadyExistsError
	if errors.As(err, &alreadyExists) {
		runState, getErr := r.runs.Get(ctx, runID)
		return runState, attemptCount, getErr
	}
	return nil, attemptCount, err
}

func currentAttemptCount(resource *platformv1alpha1.AgentSessionActionResource) int32 {
	if resource == nil {
		return 0
	}
	if resource.Status.AttemptCount > 0 {
		return resource.Status.AttemptCount
	}
	if strings.TrimSpace(resource.Status.RunID) != "" {
		return 1
	}
	return 0
}

func currentCandidateIndex(resource *platformv1alpha1.AgentSessionActionResource, snapshot *agentsessionactionv1.AgentSessionRunTurnSnapshot) int32 {
	if resource == nil || snapshot == nil {
		return 0
	}
	index := resource.Status.CandidateIndex
	if index < 0 || int(index) >= len(snapshot.GetRuntimeCandidates()) {
		return 0
	}
	return index
}

func nextCandidateIndex(resource *platformv1alpha1.AgentSessionActionResource, snapshot *agentsessionactionv1.AgentSessionRunTurnSnapshot) (int32, bool) {
	next := currentCandidateIndex(resource, snapshot) + 1
	if snapshot == nil || int(next) >= len(snapshot.GetRuntimeCandidates()) {
		return 0, false
	}
	return next, true
}

func nextRunAttempt(resource *platformv1alpha1.AgentSessionActionResource) (string, int32) {
	if resource == nil || resource.Spec.Action == nil {
		return "", 0
	}
	if runID := strings.TrimSpace(resource.Status.RunID); runID != "" {
		return runID, currentAttemptCount(resource)
	}
	attemptCount := currentAttemptCount(resource) + 1
	return attemptRunID(resource.Spec.Action.GetActionId(), attemptCount), attemptCount
}

func attemptRunID(actionID string, attemptCount int32) string {
	actionID = strings.TrimSpace(actionID)
	if attemptCount <= 1 {
		return actionID
	}
	return fmt.Sprintf("%s-attempt-%d", actionID, attemptCount)
}

func snapshotForCandidate(snapshot *agentsessionactionv1.AgentSessionRunTurnSnapshot, candidateIndex int32, runID string) (*agentsessionactionv1.AgentSessionRunTurnSnapshot, error) {
	if snapshot == nil {
		return nil, validation("run_turn snapshot is nil")
	}
	if candidateIndex < 0 || int(candidateIndex) >= len(snapshot.GetRuntimeCandidates()) {
		return nil, validation("run_turn runtime candidate index is invalid")
	}
	next := proto.Clone(snapshot).(*agentsessionactionv1.AgentSessionRunTurnSnapshot)
	candidate := next.GetRuntimeCandidates()[candidateIndex]
	next.RunRequest.RunId = runID
	next.RunRequest.ResolvedProviderModel = cloneResolvedProviderModel(candidate.GetResolvedProviderModel())
	next.AuthRequirement = cloneAuthRequirement(candidate.GetAuthRequirement())
	rebindAuthPrepareJobs(next.GetPrepareJobs(), next.GetAuthRequirement())
	return next, nil
}
