package agentruns

import (
	"strings"
	"time"

	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func deriveInvalidStatus(resource *platformv1alpha1.AgentRunResource, now time.Time) (*platformv1alpha1.AgentRunResourceStatus, bool) {
	if resource == nil {
		return failedStatus(0, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun resource is required."), true
	}
	if resource.Spec.Run == nil {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun spec.run is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetRunId()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun runId is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetSessionId()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun sessionId is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetExecutionClass()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun executionClass is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetProviderId()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun providerId is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetContainerImage()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun containerImage is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetCpuRequest()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun cpuRequest is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetMemoryRequest()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun memoryRequest is required."), true
	}
	if resource.Spec.Run.GetAuthRequirement() == nil {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun authRequirement is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetAuthRequirement().GetSurfaceId()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun authRequirement.surfaceId is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetAuthRequirement().GetProviderId()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun authRequirement.providerId is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetAuthRequirement().GetEndpointUrl()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun authRequirement.endpointUrl is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetAuthRequirement().GetMaterializationKey()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun authRequirement.materializationKey is required."), true
	}
	return nil, false
}

func pendingStatus(resource *platformv1alpha1.AgentRunResource, now time.Time) *platformv1alpha1.AgentRunResourceStatus {
	generation := resource.Generation
	message := "AgentRun is accepted."
	return &platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: generation,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), message, generation, now),
			},
		},
		Phase:       platformv1alpha1.AgentRunResourcePhasePending,
		Message:     message,
		PrepareJobs: prepareJobStatuses(resource, nil, agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_PENDING),
		UpdatedAt:   timePtr(now),
	}
}

func scheduledStatus(resource *platformv1alpha1.AgentRunResource, workloadID string, now time.Time) *platformv1alpha1.AgentRunResourceStatus {
	generation := resource.Generation
	message := "AgentRun workflow submitted."
	return &platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: generation,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), "AgentRun is accepted.", generation, now),
				newCondition(platformcontract.AgentRunConditionTypeWorkloadReady, true, string(platformcontract.AgentRunConditionReasonWorkloadCreated), message, generation, now),
			},
		},
		Phase:       platformv1alpha1.AgentRunResourcePhaseScheduled,
		Message:     message,
		WorkloadID:  workloadID,
		PrepareJobs: prepareJobStatuses(resource, nil, agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_PENDING),
		UpdatedAt:   timePtr(now),
	}
}

func observedWorkflowStatus(resource *platformv1alpha1.AgentRunResource, workloadID string, workflowState *WorkflowState, now time.Time) *platformv1alpha1.AgentRunResourceStatus {
	generation := resource.Generation
	if workflowState == nil {
		return scheduledStatus(resource, workloadID, now)
	}
	message := strings.TrimSpace(workflowState.Message)
	prepareJobs := prepareJobStatuses(resource, workflowState, prepareFallbackPhase(workflowState.Phase))
	switch strings.ToLower(strings.TrimSpace(workflowState.Phase)) {
	case "running":
		if message == "" {
			message = "AgentRun is running."
		}
		return &platformv1alpha1.AgentRunResourceStatus{
			CommonStatusFields: platformv1alpha1.CommonStatusFields{
				ObservedGeneration: generation,
				Conditions: []metav1.Condition{
					newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), "AgentRun is accepted.", generation, now),
					newCondition(platformcontract.AgentRunConditionTypeWorkloadReady, true, string(platformcontract.AgentRunConditionReasonRunStarted), message, generation, now),
				},
			},
			Phase:       platformv1alpha1.AgentRunResourcePhaseRunning,
			Message:     message,
			WorkloadID:  workloadID,
			PrepareJobs: prepareJobs,
			UpdatedAt:   timePtr(now),
		}
	case "succeeded":
		if message == "" {
			message = "AgentRun completed successfully."
		}
		next := terminalWorkflowStatus(platformv1alpha1.AgentRunResourcePhaseSucceeded, generation, workloadID, string(platformcontract.AgentRunConditionReasonRunSucceeded), message, now)
		next.PrepareJobs = prepareJobs
		return next
	case "failed", "error":
		if message == "" {
			message = "AgentRun workflow failed."
		}
		next := terminalWorkflowStatus(platformv1alpha1.AgentRunResourcePhaseFailed, generation, workloadID, string(platformcontract.AgentRunConditionReasonRunFailed), message, now)
		next.PrepareJobs = prepareJobs
		return next
	case "cancelled", "canceled":
		if message == "" {
			message = "AgentRun workflow canceled."
		}
		next := terminalWorkflowStatus(platformv1alpha1.AgentRunResourcePhaseCanceled, generation, workloadID, string(platformcontract.AgentRunConditionReasonRunCanceled), message, now)
		next.PrepareJobs = prepareJobs
		return next
	default:
		return scheduledStatus(resource, workloadID, now)
	}
}

func terminalWorkflowStatus(phase platformv1alpha1.AgentRunResourcePhase, generation int64, workloadID string, reason string, message string, now time.Time) *platformv1alpha1.AgentRunResourceStatus {
	return &platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: generation,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), "AgentRun is accepted.", generation, now),
				newCondition(platformcontract.AgentRunConditionTypeCompleted, true, reason, message, generation, now),
			},
		},
		Phase:      phase,
		Message:    message,
		WorkloadID: workloadID,
		UpdatedAt:  timePtr(now),
	}
}

func failedStatus(generation int64, now time.Time, reason string, message string) *platformv1alpha1.AgentRunResourceStatus {
	return &platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: generation,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, false, reason, message, generation, now),
			},
		},
		Phase:     platformv1alpha1.AgentRunResourcePhaseFailed,
		Message:   message,
		UpdatedAt: timePtr(now),
	}
}

func newCondition(conditionType platformcontract.AgentRunConditionType, accepted bool, reason string, message string, generation int64, now time.Time) metav1.Condition {
	status := metav1.ConditionFalse
	if accepted {
		status = metav1.ConditionTrue
	}
	return metav1.Condition{
		Type:               string(conditionType),
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.NewTime(now),
	}
}

func isTerminalPhase(phase platformv1alpha1.AgentRunResourcePhase) bool {
	switch phase {
	case platformv1alpha1.AgentRunResourcePhaseSucceeded,
		platformv1alpha1.AgentRunResourcePhaseFailed,
		platformv1alpha1.AgentRunResourcePhaseCanceled:
		return true
	default:
		return false
	}
}

func isTerminalWorkflowState(state *WorkflowState) bool {
	if state == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(state.Phase)) {
	case "succeeded", "failed", "error", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func timePtr(value time.Time) *metav1.Time {
	out := metav1.NewTime(value)
	return &out
}
