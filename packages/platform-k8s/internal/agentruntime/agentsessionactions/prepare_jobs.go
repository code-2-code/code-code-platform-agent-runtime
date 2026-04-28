package agentsessionactions

import (
	"fmt"
	"strings"

	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
)

func prepareJobsForSession(spec *agentsessionv1.AgentSessionSpec, auth *agentrunv1.AgentRunAuthRequirement) ([]*agentrunv1.AgentRunPrepareJob, error) {
	if spec == nil {
		return nil, validation("session spec is invalid")
	}
	jobs := make([]*agentrunv1.AgentRunPrepareJob, 0, len(spec.GetPrepareJobs())+1)
	seen := map[string]struct{}{}
	hasAuth := false
	for index, item := range spec.GetPrepareJobs() {
		job, err := prepareJobFromSession(spec, item, index)
		if err != nil {
			return nil, err
		}
		if job == nil {
			continue
		}
		if _, ok := seen[job.GetJobId()]; ok {
			return nil, validationf("session prepare job %q is duplicated", job.GetJobId())
		}
		seen[job.GetJobId()] = struct{}{}
		if strings.TrimSpace(job.GetJobType()) == "auth" {
			hasAuth = true
			job.Cleanup = true
			job.ParametersYaml = authPrepareParametersYAML(auth)
		}
		jobs = append(jobs, job)
	}
	if auth != nil && !hasAuth {
		jobs = append(jobs, defaultAuthPrepareJob(spec.GetProviderId(), auth))
	}
	return jobs, nil
}

func prepareJobFromSession(spec *agentsessionv1.AgentSessionSpec, item *agentsessionv1.AgentSessionPrepareJob, index int) (*agentrunv1.AgentRunPrepareJob, error) {
	if item == nil {
		return nil, nil
	}
	jobType := strings.TrimSpace(item.GetJobType())
	if jobType == "" {
		return nil, validationf("session prepare job %d job_type is required", index+1)
	}
	runType := item.GetRunType()
	if runType == agentrunv1.AgentRunPrepareJobRunType_AGENT_RUN_PREPARE_JOB_RUN_TYPE_UNSPECIFIED {
		runType = agentrunv1.AgentRunPrepareJobRunType_AGENT_RUN_PREPARE_JOB_RUN_TYPE_PER_RUN
	}
	return &agentrunv1.AgentRunPrepareJob{
		JobId:          prepareJobID(item.GetJobId(), index),
		CliId:          firstNonEmpty(item.GetCliId(), spec.GetProviderId()),
		JobType:        jobType,
		RunType:        runType,
		ChangeKey:      strings.TrimSpace(item.GetChangeKey()),
		Cleanup:        item.GetCleanup(),
		ParametersYaml: item.GetParametersYaml(),
	}, nil
}

func prepareJobID(jobID string, index int) string {
	if trimmed := strings.TrimSpace(jobID); trimmed != "" {
		return trimmed
	}
	return fmt.Sprintf("job-%02d", index+1)
}

func defaultAuthPrepareJob(cliID string, auth *agentrunv1.AgentRunAuthRequirement) *agentrunv1.AgentRunPrepareJob {
	return &agentrunv1.AgentRunPrepareJob{
		JobId:          "auth",
		CliId:          strings.TrimSpace(cliID),
		JobType:        "auth",
		RunType:        agentrunv1.AgentRunPrepareJobRunType_AGENT_RUN_PREPARE_JOB_RUN_TYPE_PER_RUN,
		Cleanup:        true,
		ParametersYaml: authPrepareParametersYAML(auth),
	}
}

func authPrepareParametersYAML(auth *agentrunv1.AgentRunAuthRequirement) string {
	if auth == nil {
		return ""
	}
	return fmt.Sprintf(
		"providerSurfaceBindingId: %q\nruntimeUrl: %q\nauthMaterializationKey: %q\n",
		strings.TrimSpace(auth.GetProviderSurfaceBindingId()),
		strings.TrimSpace(auth.GetRuntimeUrl()),
		strings.TrimSpace(auth.GetMaterializationKey()),
	)
}

func rebindAuthPrepareJobs(jobs []*agentrunv1.AgentRunPrepareJob, auth *agentrunv1.AgentRunAuthRequirement) {
	for _, job := range jobs {
		if job != nil && strings.TrimSpace(job.GetJobType()) == "auth" {
			job.Cleanup = true
			job.ParametersYaml = authPrepareParametersYAML(auth)
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
