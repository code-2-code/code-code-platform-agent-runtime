package agentsessions

import (
	"strings"

	capv1 "code-code.internal/go-contract/agent/cap/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentresourceconfig"
)

func resourceConfigReadinessMessage(config *capv1.AgentResources, realized agentresourceconfig.Revisions) string {
	if config == nil {
		return "Resource config is missing."
	}
	if agentresourceconfig.Ready(config, realized) {
		return "Resource config is ready."
	}
	desiredSnapshotID := strings.TrimSpace(config.GetSnapshotId())
	if desiredSnapshotID == "" {
		return "Resource config requires snapshotId."
	}
	pending := agentresourceconfig.PendingSubjects(config, realized)
	if len(pending) == 0 {
		return "Resource config reload is pending."
	}
	names := make([]string, 0, len(pending))
	for _, subject := range pending {
		names = append(names, agentresourceconfig.SubjectSlug(subject))
	}
	return "Resource config reload is pending for " + strings.Join(names, ", ") + "."
}

func realizedResourceConfigRevisions(resource *platformv1alpha1.AgentSessionResource) agentresourceconfig.Revisions {
	if resource == nil {
		return agentresourceconfig.Revisions{}
	}
	return agentresourceconfig.Revisions{
		Rule:  strings.TrimSpace(resource.Status.RealizedRuleRevision),
		Skill: strings.TrimSpace(resource.Status.RealizedSkillRevision),
		MCP:   strings.TrimSpace(resource.Status.RealizedMCPRevision),
	}
}
