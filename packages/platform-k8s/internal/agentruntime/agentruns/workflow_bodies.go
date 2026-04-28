package agentruns

import (
	"encoding/json"
	"fmt"

	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessions"
)

func prepareRequestBody(run *platformv1alpha1.AgentRunResource, job *agentrunv1.AgentRunPrepareJob, config agentRunWorkflowConfig) (string, error) {
	if run == nil || run.Spec.Run == nil || job == nil {
		return "", fmt.Errorf("platformk8s/agentruns: prepare request is invalid")
	}
	spec := run.Spec.Run
	auth := spec.GetAuthRequirement()
	return marshalWorkflowBody(map[string]any{
		"runtimeNamespace":         config.ExecutionNamespace,
		"sessionId":                spec.GetSessionId(),
		"runId":                    spec.GetRunId(),
		"agentRunResourceName":     run.Name,
		"workspacePVCName":         agentsessions.WorkspacePVCName(spec.GetSessionId(), spec.GetWorkspaceId()),
		"homeStatePVCName":         agentsessions.HomeStatePVCName(spec.GetSessionId(), spec.GetHomeStateId()),
		"runtimeWorkspaceDir":      spec.GetRuntimeEnvironment().GetWorkspaceDir(),
		"runtimeDataDir":           spec.GetRuntimeEnvironment().GetDataDir(),
		"containerImage":           spec.GetContainerImage(),
		"providerId":               spec.GetProviderId(),
		"providerSurfaceBindingId": auth.GetProviderSurfaceBindingId(),
		"runtimeUrl":               auth.GetRuntimeUrl(),
		"authMaterializationKey":   auth.GetMaterializationKey(),
		"job": map[string]any{
			"jobId":          job.GetJobId(),
			"cliId":          job.GetCliId(),
			"jobType":        job.GetJobType(),
			"runType":        prepareRunType(job.GetRunType()),
			"changeKey":      job.GetChangeKey(),
			"cleanup":        job.GetCleanup(),
			"parametersYaml": job.GetParametersYaml(),
		},
	})
}

func cleanupRequestBody(run *platformv1alpha1.AgentRunResource, config agentRunWorkflowConfig) (string, error) {
	if run == nil || run.Spec.Run == nil {
		return "", fmt.Errorf("platformk8s/agentruns: cleanup request is invalid")
	}
	jobs := make([]map[string]any, 0, len(run.Spec.Run.GetPrepareJobs()))
	for _, job := range run.Spec.Run.GetPrepareJobs() {
		if job == nil || !job.GetCleanup() {
			continue
		}
		jobs = append(jobs, map[string]any{
			"jobId":          job.GetJobId(),
			"cliId":          job.GetCliId(),
			"jobType":        job.GetJobType(),
			"runType":        prepareRunType(job.GetRunType()),
			"changeKey":      job.GetChangeKey(),
			"cleanup":        job.GetCleanup(),
			"parametersYaml": job.GetParametersYaml(),
		})
	}
	return marshalWorkflowBody(map[string]any{
		"runtimeNamespace":     config.ExecutionNamespace,
		"sessionId":            run.Spec.Run.GetSessionId(),
		"runId":                run.Spec.Run.GetRunId(),
		"agentRunResourceName": run.Name,
		"jobs":                 jobs,
	})
}

func prepareRunType(value agentrunv1.AgentRunPrepareJobRunType) string {
	switch value {
	case agentrunv1.AgentRunPrepareJobRunType_AGENT_RUN_PREPARE_JOB_RUN_TYPE_INIT:
		return "init"
	case agentrunv1.AgentRunPrepareJobRunType_AGENT_RUN_PREPARE_JOB_RUN_TYPE_PER_RUN:
		return "per-run"
	case agentrunv1.AgentRunPrepareJobRunType_AGENT_RUN_PREPARE_JOB_RUN_TYPE_ON_CHANGED:
		return "on-changed"
	default:
		return ""
	}
}

func marshalWorkflowBody(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("platformk8s/agentruns: marshal workflow body: %w", err)
	}
	return string(body), nil
}
