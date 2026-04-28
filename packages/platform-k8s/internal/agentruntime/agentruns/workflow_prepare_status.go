package agentruns

import (
	"fmt"
	"strings"
	"time"

	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/workflows"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func prepareJobStatuses(resource *platformv1alpha1.AgentRunResource, workflowState *WorkflowState, fallback agentrunv1.AgentRunPrepareJobPhase) []*agentrunv1.AgentRunPrepareJobStatus {
	if resource == nil || resource.Spec.Run == nil || len(resource.Spec.Run.GetPrepareJobs()) == 0 {
		return nil
	}
	jobs := resource.Spec.Run.GetPrepareJobs()
	out := make([]*agentrunv1.AgentRunPrepareJobStatus, 0, len(jobs))
	for index, job := range jobs {
		if job == nil {
			continue
		}
		status := &agentrunv1.AgentRunPrepareJobStatus{
			JobId: job.GetJobId(),
			Phase: fallback,
		}
		if node := prepareWorkflowNode(workflowState, index, job); node != nil {
			status.Phase = preparePhaseFromWorkflow(node.Phase, fallback)
			status.Message = strings.TrimSpace(node.Message)
			status.StartedAt = timestampFromTime(node.StartedAt)
			status.FinishedAt = timestampFromTime(node.FinishedAt)
		}
		if strings.TrimSpace(status.Message) == "" {
			status.Message = prepareStatusMessage(job, status.Phase)
		}
		out = append(out, status)
	}
	return out
}

func prepareWorkflowNode(state *WorkflowState, index int, job *agentrunv1.AgentRunPrepareJob) *workflows.NodeState {
	if state == nil || job == nil {
		return nil
	}
	stepName := prepareStepName(index, job)
	for i := range state.Nodes {
		node := &state.Nodes[i]
		if nodeMatchesStep(node, stepName) {
			return node
		}
	}
	return nil
}

func nodeMatchesStep(node *workflows.NodeState, stepName string) bool {
	if node == nil || strings.TrimSpace(stepName) == "" {
		return false
	}
	if strings.TrimSpace(node.DisplayName) == stepName || strings.TrimSpace(node.Name) == stepName {
		return true
	}
	return strings.HasSuffix(strings.TrimSpace(node.Name), "."+stepName)
}

func preparePhaseFromWorkflow(phase string, fallback agentrunv1.AgentRunPrepareJobPhase) agentrunv1.AgentRunPrepareJobPhase {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "pending":
		return agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_PENDING
	case "running":
		return agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_RUNNING
	case "succeeded":
		return agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_SUCCEEDED
	case "failed", "error":
		return agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_FAILED
	case "canceled", "cancelled":
		return agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_CANCELED
	case "skipped", "omitted":
		return agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_SKIPPED
	default:
		return fallback
	}
}

func prepareFallbackPhase(phase string) agentrunv1.AgentRunPrepareJobPhase {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "succeeded":
		return agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_SUCCEEDED
	case "canceled", "cancelled":
		return agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_CANCELED
	default:
		return agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_PENDING
	}
}

func timestampFromTime(value *time.Time) *timestamppb.Timestamp {
	if value == nil || value.IsZero() {
		return nil
	}
	return timestamppb.New(value.UTC())
}

func prepareStatusMessage(job *agentrunv1.AgentRunPrepareJob, phase agentrunv1.AgentRunPrepareJobPhase) string {
	name := strings.TrimSpace(job.GetJobType())
	if name == "" {
		name = strings.TrimSpace(job.GetJobId())
	}
	if name == "" {
		name = "prepare job"
	}
	switch phase {
	case agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_PENDING:
		return fmt.Sprintf("%s is pending.", name)
	case agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_RUNNING:
		return fmt.Sprintf("%s is running.", name)
	case agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_SUCCEEDED:
		return fmt.Sprintf("%s completed.", name)
	case agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_FAILED:
		return fmt.Sprintf("%s failed.", name)
	case agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_CANCELED:
		return fmt.Sprintf("%s canceled.", name)
	case agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_SKIPPED:
		return fmt.Sprintf("%s skipped.", name)
	default:
		return ""
	}
}
