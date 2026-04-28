package agentsessions

import (
	"context"
	"fmt"
	"strings"

	capv1 "code-code.internal/go-contract/agent/cap/v1"
	agentprofilev1 "code-code.internal/go-contract/platform/agent_profile/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	mcpv1 "code-code.internal/go-contract/platform/mcp/v1"
	rulev1 "code-code.internal/go-contract/platform/rule/v1"
	skillv1 "code-code.internal/go-contract/platform/skill/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentresourceconfig"
	"google.golang.org/protobuf/proto"
)

type ProfileProjectionSource interface {
	GetProfile(ctx context.Context, profileID string) (*agentprofilev1.AgentProfile, int64, error)
	GetRule(ctx context.Context, ruleID string) (*rulev1.Rule, error)
	GetSkill(ctx context.Context, skillID string) (*skillv1.Skill, error)
	GetMCP(ctx context.Context, mcpID string) (*mcpv1.MCPServer, error)
}

type ProfileProjector struct {
	source ProfileProjectionSource
}

func NewProfileProjector(source ProfileProjectionSource) (*ProfileProjector, error) {
	if source == nil {
		return nil, fmt.Errorf("platformk8s/agentsessions: profile projection source is nil")
	}
	return &ProfileProjector{source: source}, nil
}

func (p *ProfileProjector) Project(ctx context.Context, base *agentsessionv1.AgentSessionSpec) (*agentsessionv1.AgentSessionSpec, error) {
	if base == nil {
		return nil, validation("session request is nil")
	}
	profileID := strings.TrimSpace(base.GetProfileId())
	if profileID == "" {
		return proto.Clone(base).(*agentsessionv1.AgentSessionSpec), nil
	}
	profile, generation, err := p.loadProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	runtimeConfig, err := runtimeConfigFromProfile(profile)
	if err != nil {
		return nil, err
	}
	resourceConfig, err := p.resourceConfigFromProfile(ctx, profile)
	if err != nil {
		return nil, err
	}
	return &agentsessionv1.AgentSessionSpec{
		SessionId:         strings.TrimSpace(base.GetSessionId()),
		ProfileId:         profile.GetProfileId(),
		ProfileGeneration: generation,
		ProviderId:        strings.TrimSpace(profile.GetSelectionStrategy().GetProviderId()),
		ExecutionClass:    strings.TrimSpace(profile.GetSelectionStrategy().GetExecutionClass()),
		RuntimeConfig:     runtimeConfig,
		ResourceConfig:    resourceConfig,
		WorkspaceRef:      cloneWorkspaceRef(base.GetWorkspaceRef()),
		HomeStateRef:      cloneHomeStateRef(base.GetHomeStateRef()),
	}, nil
}

func (p *ProfileProjector) loadProfile(ctx context.Context, profileID string) (*agentprofilev1.AgentProfile, int64, error) {
	profile, generation, err := p.source.GetProfile(ctx, strings.TrimSpace(profileID))
	if err != nil {
		return nil, 0, err
	}
	if profile == nil {
		return nil, 0, validationf("profile %q is missing payload", profileID)
	}
	return profile, generation, nil
}

func runtimeConfigFromProfile(profile *agentprofilev1.AgentProfile) (*agentsessionv1.AgentSessionRuntimeConfig, error) {
	if profile == nil || profile.GetSelectionStrategy() == nil {
		return nil, validation("profile selection strategy is missing")
	}
	items := profile.GetSelectionStrategy().GetFallbacks()
	if len(items) == 0 || items[0] == nil {
		return nil, validationf("profile %q primary runtime candidate is missing", profile.GetProfileId())
	}
	primary := items[0]
	fallbackCapacity := 0
	if len(items) > 1 {
		fallbackCapacity = len(items) - 1
	}
	runtimeConfig := &agentsessionv1.AgentSessionRuntimeConfig{
		ProviderRuntimeRef: cloneProviderRuntimeRef(primary.GetProviderRuntimeRef()),
		PrimaryModelSelector: profileModelSelector(
			primary,
		),
		Fallbacks: make([]*agentsessionv1.AgentSessionRuntimeFallbackCandidate, 0, fallbackCapacity),
	}
	for _, item := range items[1:] {
		if item == nil {
			continue
		}
		runtimeConfig.Fallbacks = append(runtimeConfig.Fallbacks, fallbackFromProfile(item))
	}
	return runtimeConfig, nil
}

func profileModelSelector(candidate *agentprofilev1.AgentFallbackCandidate) *agentsessionv1.AgentSessionRuntimeModelSelector {
	if candidate == nil {
		return nil
	}
	switch selector := candidate.ModelSelector.(type) {
	case *agentprofilev1.AgentFallbackCandidate_ModelRef:
		return &agentsessionv1.AgentSessionRuntimeModelSelector{
			Selector: &agentsessionv1.AgentSessionRuntimeModelSelector_ModelRef{ModelRef: normalizeFallbackModelRef(selector.ModelRef)},
		}
	case *agentprofilev1.AgentFallbackCandidate_ProviderModelId:
		value := strings.TrimSpace(selector.ProviderModelId)
		if value == "" {
			return nil
		}
		return &agentsessionv1.AgentSessionRuntimeModelSelector{
			Selector: &agentsessionv1.AgentSessionRuntimeModelSelector_ProviderModelId{ProviderModelId: value},
		}
	default:
		return nil
	}
}

func fallbackFromProfile(candidate *agentprofilev1.AgentFallbackCandidate) *agentsessionv1.AgentSessionRuntimeFallbackCandidate {
	if candidate == nil {
		return nil
	}
	fallback := &agentsessionv1.AgentSessionRuntimeFallbackCandidate{
		ProviderRuntimeRef: cloneProviderRuntimeRef(candidate.GetProviderRuntimeRef()),
	}
	switch selector := candidate.ModelSelector.(type) {
	case *agentprofilev1.AgentFallbackCandidate_ModelRef:
		fallback.ModelSelector = &agentsessionv1.AgentSessionRuntimeFallbackCandidate_ModelRef{ModelRef: normalizeFallbackModelRef(selector.ModelRef)}
	case *agentprofilev1.AgentFallbackCandidate_ProviderModelId:
		fallback.ModelSelector = &agentsessionv1.AgentSessionRuntimeFallbackCandidate_ProviderModelId{ProviderModelId: strings.TrimSpace(selector.ProviderModelId)}
	}
	return fallback
}

func (p *ProfileProjector) resourceConfigFromProfile(ctx context.Context, profile *agentprofilev1.AgentProfile) (*capv1.AgentResources, error) {
	resourceConfig := &capv1.AgentResources{
		Instructions: make([]*capv1.InstructionResource, 0, len(profile.GetRuleIds())+len(profile.GetSkillIds())),
		ToolBindings: make([]*capv1.ToolBinding, 0, len(profile.GetMcpIds())),
	}
	for _, ruleID := range profile.GetRuleIds() {
		rule, err := p.loadRule(ctx, ruleID)
		if err != nil {
			return nil, err
		}
		resourceConfig.Instructions = append(resourceConfig.Instructions, &capv1.InstructionResource{
			Kind:    capv1.InstructionKind_INSTRUCTION_KIND_RULE,
			Name:    displayName(rule.GetName(), rule.GetRuleId()),
			Content: strings.TrimSpace(rule.GetContent()),
		})
	}
	for _, skillID := range profile.GetSkillIds() {
		skill, err := p.loadSkill(ctx, skillID)
		if err != nil {
			return nil, err
		}
		resourceConfig.Instructions = append(resourceConfig.Instructions, &capv1.InstructionResource{
			Kind:    capv1.InstructionKind_INSTRUCTION_KIND_SKILL,
			Name:    displayName(skill.GetName(), skill.GetSkillId()),
			Content: strings.TrimSpace(skill.GetContent()),
		})
	}
	for _, mcpID := range profile.GetMcpIds() {
		server, err := p.loadMCP(ctx, mcpID)
		if err != nil {
			return nil, err
		}
		resourceConfig.ToolBindings = append(resourceConfig.ToolBindings, &capv1.ToolBinding{
			Name:   displayName(server.GetName(), server.GetMcpId()),
			Kind:   capv1.ToolKind_TOOL_KIND_MCP,
			Target: "mcp://" + strings.TrimSpace(server.GetMcpId()),
		})
	}
	resourceConfig.SnapshotId = agentresourceconfig.SnapshotID(resourceConfig)
	return resourceConfig, nil
}

func (p *ProfileProjector) loadRule(ctx context.Context, ruleID string) (*rulev1.Rule, error) {
	rule, err := p.source.GetRule(ctx, strings.TrimSpace(ruleID))
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, validationf("rule %q is missing payload", ruleID)
	}
	return rule, nil
}

func (p *ProfileProjector) loadSkill(ctx context.Context, skillID string) (*skillv1.Skill, error) {
	skill, err := p.source.GetSkill(ctx, strings.TrimSpace(skillID))
	if err != nil {
		return nil, err
	}
	if skill == nil {
		return nil, validationf("skill %q is missing payload", skillID)
	}
	return skill, nil
}

func (p *ProfileProjector) loadMCP(ctx context.Context, mcpID string) (*mcpv1.MCPServer, error) {
	server, err := p.source.GetMCP(ctx, strings.TrimSpace(mcpID))
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, validationf("mcp %q is missing payload", mcpID)
	}
	return server, nil
}

func cloneWorkspaceRef(ref *agentsessionv1.AgentSessionWorkspaceRef) *agentsessionv1.AgentSessionWorkspaceRef {
	if ref == nil {
		return nil
	}
	return proto.Clone(ref).(*agentsessionv1.AgentSessionWorkspaceRef)
}

func cloneHomeStateRef(ref *agentsessionv1.AgentSessionHomeStateRef) *agentsessionv1.AgentSessionHomeStateRef {
	if ref == nil {
		return nil
	}
	return proto.Clone(ref).(*agentsessionv1.AgentSessionHomeStateRef)
}

func cloneProviderRuntimeRef(ref *providerv1.ProviderRuntimeRef) *providerv1.ProviderRuntimeRef {
	if ref == nil {
		return nil
	}
	return proto.Clone(ref).(*providerv1.ProviderRuntimeRef)
}

func displayName(name, fallback string) string {
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return strings.TrimSpace(fallback)
}
