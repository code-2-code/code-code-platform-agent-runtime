package agentruns

import (
	"strings"

	resultv1 "code-code.internal/go-contract/agent/result/v1"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/platform/protostate"
	"google.golang.org/protobuf/proto"
)

func runStateFromResource(resource *platformv1alpha1.AgentRunResource) (*agentrunv1.AgentRunState, error) {
	if resource == nil || resource.Spec.Run == nil {
		return nil, validation("run resource is invalid")
	}
	spec := resource.Spec.Run
	if spec.GetRunId() == "" {
		spec.RunId = resource.Name
	}
	if spec.GetRunId() != resource.Name {
		return nil, validationf("run id %q does not match resource name %q", spec.GetRunId(), resource.Name)
	}
	return &agentrunv1.AgentRunState{
		Generation: resource.Generation,
		Spec:       spec,
		Status: &agentrunv1.AgentRunStatus{
			RunId:              spec.GetRunId(),
			Phase:              toProtoRunPhase(resource.Status.Phase),
			ObservedGeneration: resource.Status.ObservedGeneration,
			Message:            resource.Status.Message,
			Workload:           workloadRef(resource.Status.WorkloadID),
			Conditions:         protostate.Conditions(resource.Status.Conditions),
			Result:             resultFromSummary(resource.Status.ResultSummary),
			UpdatedAt:          protostate.Timestamp(resource.Status.UpdatedAt),
			PrepareJobs:        clonePrepareJobStatuses(resource.Status.PrepareJobs),
		},
	}, nil
}

func workloadRef(workloadID string) *agentrunv1.WorkloadRef {
	if strings.TrimSpace(workloadID) == "" {
		return nil
	}
	return &agentrunv1.WorkloadRef{WorkloadId: strings.TrimSpace(workloadID)}
}

func resultFromSummary(summary *platformv1alpha1.AgentRunResultSummary) *resultv1.RunResult {
	if summary == nil {
		return nil
	}
	status := resultv1.RunStatus_RUN_STATUS_UNSPECIFIED
	if value, ok := resultv1.RunStatus_value[strings.TrimSpace(summary.Status)]; ok {
		status = resultv1.RunStatus(value)
	}
	result := &resultv1.RunResult{Status: status}
	if strings.TrimSpace(summary.ErrorCode) != "" || strings.TrimSpace(summary.ErrorMessage) != "" || summary.Retryable {
		result.Error = &resultv1.RunError{
			Code:      strings.TrimSpace(summary.ErrorCode),
			Message:   strings.TrimSpace(summary.ErrorMessage),
			Retryable: summary.Retryable,
		}
	}
	return result
}

func clonePrepareJobStatuses(items []*agentrunv1.AgentRunPrepareJobStatus) []*agentrunv1.AgentRunPrepareJobStatus {
	if len(items) == 0 {
		return nil
	}
	out := make([]*agentrunv1.AgentRunPrepareJobStatus, 0, len(items))
	for _, item := range items {
		if item != nil {
			out = append(out, proto.Clone(item).(*agentrunv1.AgentRunPrepareJobStatus))
		}
	}
	return out
}

func toProtoRunPhase(phase platformv1alpha1.AgentRunResourcePhase) agentrunv1.AgentRunPhase {
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
