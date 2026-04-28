package agentresourceconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	capv1 "code-code.internal/go-contract/agent/cap/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	"google.golang.org/protobuf/proto"
)

type Revisions struct {
	Rule  string
	Skill string
	MCP   string
}

type SubjectSnapshot struct {
	Subject         agentsessionactionv1.AgentSessionActionSubject
	SubjectRevision string
	ResourceConfig  *capv1.AgentResources
}

func DesiredRevisions(config *capv1.AgentResources) Revisions {
	return Revisions{
		Rule:  snapshotRevision(config, agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_RULE),
		Skill: snapshotRevision(config, agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_SKILL),
		MCP:   snapshotRevision(config, agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_MCP),
	}
}

func SnapshotID(config *capv1.AgentResources) string {
	return snapshotRevision(config, agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_RESOURCE_CONFIG)
}

func Snapshot(config *capv1.AgentResources, subject agentsessionactionv1.AgentSessionActionSubject) *SubjectSnapshot {
	filtered := filter(config, subject)
	if filtered == nil {
		return nil
	}
	return &SubjectSnapshot{
		Subject:         subject,
		SubjectRevision: revision(filtered),
		ResourceConfig:  filtered,
	}
}

func PendingSubjects(config *capv1.AgentResources, realized Revisions) []agentsessionactionv1.AgentSessionActionSubject {
	desired := DesiredRevisions(config)
	pending := make([]agentsessionactionv1.AgentSessionActionSubject, 0, 3)
	if desired.Rule != strings.TrimSpace(realized.Rule) {
		pending = append(pending, agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_RULE)
	}
	if desired.Skill != strings.TrimSpace(realized.Skill) {
		pending = append(pending, agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_SKILL)
	}
	if desired.MCP != strings.TrimSpace(realized.MCP) {
		pending = append(pending, agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_MCP)
	}
	return pending
}

func Ready(config *capv1.AgentResources, realized Revisions) bool {
	desired := DesiredRevisions(config)
	return desired.Rule == strings.TrimSpace(realized.Rule) &&
		desired.Skill == strings.TrimSpace(realized.Skill) &&
		desired.MCP == strings.TrimSpace(realized.MCP)
}

func Matches(realized Revisions, snapshot *agentsessionactionv1.AgentSessionReloadSubjectSnapshot) bool {
	if snapshot == nil {
		return false
	}
	switch snapshot.GetSubject() {
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_RESOURCE_CONFIG:
		return strings.TrimSpace(realized.Rule) == snapshotRevision(snapshot.GetResourceConfig(), agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_RULE) &&
			strings.TrimSpace(realized.Skill) == snapshotRevision(snapshot.GetResourceConfig(), agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_SKILL) &&
			strings.TrimSpace(realized.MCP) == snapshotRevision(snapshot.GetResourceConfig(), agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_MCP)
	default:
		return revisionForSubject(realized, snapshot.GetSubject()) == strings.TrimSpace(snapshot.GetSubjectRevision())
	}
}

func Apply(realized *Revisions, snapshot *agentsessionactionv1.AgentSessionReloadSubjectSnapshot) {
	if realized == nil || snapshot == nil {
		return
	}
	desired := DesiredRevisions(snapshot.GetResourceConfig())
	switch snapshot.GetSubject() {
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_RESOURCE_CONFIG:
		realized.Rule = desired.Rule
		realized.Skill = desired.Skill
		realized.MCP = desired.MCP
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_RULE:
		realized.Rule = desired.Rule
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_SKILL:
		realized.Skill = desired.Skill
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_MCP:
		realized.MCP = desired.MCP
	}
}

func SubjectSlug(subject agentsessionactionv1.AgentSessionActionSubject) string {
	switch subject {
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_RESOURCE_CONFIG:
		return "resource-config"
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_RULE:
		return "rule"
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_SKILL:
		return "skill"
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_MCP:
		return "mcp"
	default:
		return "unknown"
	}
}

func revisionForSubject(realized Revisions, subject agentsessionactionv1.AgentSessionActionSubject) string {
	switch subject {
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_RULE:
		return strings.TrimSpace(realized.Rule)
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_SKILL:
		return strings.TrimSpace(realized.Skill)
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_MCP:
		return strings.TrimSpace(realized.MCP)
	default:
		return ""
	}
}

func snapshotRevision(config *capv1.AgentResources, subject agentsessionactionv1.AgentSessionActionSubject) string {
	filtered := filter(config, subject)
	if filtered == nil {
		return ""
	}
	return revision(filtered)
}

func filter(config *capv1.AgentResources, subject agentsessionactionv1.AgentSessionActionSubject) *capv1.AgentResources {
	if config == nil {
		return nil
	}
	filtered := &capv1.AgentResources{SnapshotId: strings.TrimSpace(config.GetSnapshotId())}
	switch subject {
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_RESOURCE_CONFIG:
		return proto.Clone(config).(*capv1.AgentResources)
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_RULE:
		for _, item := range config.GetInstructions() {
			if item != nil && item.GetKind() == capv1.InstructionKind_INSTRUCTION_KIND_RULE {
				filtered.Instructions = append(filtered.Instructions, proto.Clone(item).(*capv1.InstructionResource))
			}
		}
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_SKILL:
		for _, item := range config.GetInstructions() {
			if item != nil && item.GetKind() == capv1.InstructionKind_INSTRUCTION_KIND_SKILL {
				filtered.Instructions = append(filtered.Instructions, proto.Clone(item).(*capv1.InstructionResource))
			}
		}
	case agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_MCP:
		for _, item := range config.GetToolBindings() {
			if item != nil && item.GetKind() == capv1.ToolKind_TOOL_KIND_MCP {
				filtered.ToolBindings = append(filtered.ToolBindings, proto.Clone(item).(*capv1.ToolBinding))
			}
		}
	default:
		return nil
	}
	return filtered
}

func revision(config *capv1.AgentResources) string {
	normalized := proto.Clone(config).(*capv1.AgentResources)
	normalized.SnapshotId = ""
	if len(normalized.GetInstructions()) == 0 && len(normalized.GetToolBindings()) == 0 {
		return ""
	}
	payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(normalized)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
