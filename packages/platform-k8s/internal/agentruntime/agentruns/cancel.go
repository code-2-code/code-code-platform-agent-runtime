package agentruns

import (
	"context"
	"strings"
	"time"

	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) reconcileCanceledRun(
	ctx context.Context,
	request ctrl.Request,
	resource *platformv1alpha1.AgentRunResource,
	previous *platformv1alpha1.AgentRunResourceStatus,
	now time.Time,
) (ctrl.Result, error) {
	if resource == nil || resource.Spec.Run == nil {
		return ctrl.Result{}, nil
	}
	workloadID := ""
	if previous != nil {
		workloadID = strings.TrimSpace(previous.WorkloadID)
	}
	if workloadID == "" {
		next := terminalWorkflowStatus(
			platformv1alpha1.AgentRunResourcePhaseCanceled,
			resource.Generation,
			"",
			string(platformcontract.AgentRunConditionReasonRunCanceled),
			"AgentRun cancel was requested before workload submission.",
			now,
		)
		next.PrepareJobs = prepareJobStatuses(resource, nil, agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_CANCELED)
		return r.updateObservedStatus(ctx, request, resource, previous, next, nil, ctrl.Result{})
	}
	if err := r.workflowRuntime.Cancel(ctx, workloadID); err != nil {
		if apierrors.IsNotFound(err) {
			next := terminalWorkflowStatus(
				platformv1alpha1.AgentRunResourcePhaseCanceled,
				resource.Generation,
				workloadID,
				string(platformcontract.AgentRunConditionReasonRunCanceled),
				"AgentRun workflow was canceled.",
				now,
			)
			next.PrepareJobs = prepareJobStatuses(resource, nil, agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_CANCELED)
			return r.updateObservedStatus(ctx, request, resource, previous, next, nil, ctrl.Result{})
		}
		return ctrl.Result{}, err
	}
	workflowState, err := r.workflowRuntime.Get(ctx, workloadID)
	if err != nil {
		if apierrors.IsNotFound(err) {
			next := terminalWorkflowStatus(
				platformv1alpha1.AgentRunResourcePhaseCanceled,
				resource.Generation,
				workloadID,
				string(platformcontract.AgentRunConditionReasonRunCanceled),
				"AgentRun workflow was canceled.",
				now,
			)
			next.PrepareJobs = prepareJobStatuses(resource, nil, agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_CANCELED)
			return r.updateObservedStatus(ctx, request, resource, previous, next, nil, ctrl.Result{})
		}
		return ctrl.Result{}, err
	}
	next := canceledWorkflowStatus(resource, workloadID, workflowState, now)
	return r.updateObservedStatus(ctx, request, resource, previous, next, workflowState, pollResultForPhase(next.Phase))
}

func canceledWorkflowStatus(resource *platformv1alpha1.AgentRunResource, workloadID string, workflowState *WorkflowState, now time.Time) *platformv1alpha1.AgentRunResourceStatus {
	next := observedWorkflowStatus(resource, workloadID, workflowState, now)
	if next == nil {
		return terminalWorkflowStatus(
			platformv1alpha1.AgentRunResourcePhaseCanceled,
			resource.Generation,
			workloadID,
			string(platformcontract.AgentRunConditionReasonRunCanceled),
			"AgentRun workflow was canceled.",
			now,
		)
	}
	switch next.Phase {
	case platformv1alpha1.AgentRunResourcePhaseScheduled, platformv1alpha1.AgentRunResourcePhaseRunning:
		next.Message = cancelRequestedMessage(workflowState)
		next.UpdatedAt = timePtr(now)
		return next
	default:
		return next
	}
}

func cancelRequestedMessage(workflowState *WorkflowState) string {
	message := "AgentRun cancel was requested."
	if workflowState == nil {
		return message
	}
	detail := strings.TrimSpace(workflowState.Message)
	if detail == "" {
		return message
	}
	return message + " " + detail
}
