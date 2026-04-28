package agentruns

import (
	"fmt"
	"strings"

	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	"code-code.internal/platform-k8s/internal/platform/temporalruntime"
)

func prepareStepName(index int, job *agentrunv1.AgentRunPrepareJob) string {
	id := ""
	if job != nil {
		id = job.GetJobId()
	}
	part := temporalruntime.IDPart(id, fmt.Sprintf("job-%d", index+1))
	name := fmt.Sprintf("prepare-%02d-%s", index+1, part)
	if len(name) > 63 {
		return strings.Trim(name[:63], "-")
	}
	return name
}
