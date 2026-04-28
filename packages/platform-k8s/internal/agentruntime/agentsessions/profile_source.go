package agentsessions

import (
	"context"
	"fmt"
	"strings"

	agentprofilev1 "code-code.internal/go-contract/platform/agent_profile/v1"
	managementv1 "code-code.internal/go-contract/platform/management/v1"
	mcpv1 "code-code.internal/go-contract/platform/mcp/v1"
	profileservicev1 "code-code.internal/go-contract/platform/profile/v1"
	rulev1 "code-code.internal/go-contract/platform/rule/v1"
	skillv1 "code-code.internal/go-contract/platform/skill/v1"
)

type RemoteProfileProjectionSource struct {
	client profileservicev1.ProfileServiceClient
}

func NewRemoteProfileProjectionSource(client profileservicev1.ProfileServiceClient) (*RemoteProfileProjectionSource, error) {
	if client == nil {
		return nil, fmt.Errorf("platformk8s/agentsessions: profile service client is nil")
	}
	return &RemoteProfileProjectionSource{client: client}, nil
}

func (s *RemoteProfileProjectionSource) GetProfile(ctx context.Context, profileID string) (*agentprofilev1.AgentProfile, int64, error) {
	response, err := s.client.GetAgentProfile(ctx, &managementv1.GetAgentProfileRequest{ProfileId: strings.TrimSpace(profileID)})
	if err != nil {
		return nil, 0, err
	}
	return response.GetProfile(), response.GetGeneration(), nil
}

func (s *RemoteProfileProjectionSource) GetRule(ctx context.Context, ruleID string) (*rulev1.Rule, error) {
	response, err := s.client.GetRule(ctx, &managementv1.GetRuleRequest{RuleId: strings.TrimSpace(ruleID)})
	if err != nil {
		return nil, err
	}
	return response.GetRule(), nil
}

func (s *RemoteProfileProjectionSource) GetSkill(ctx context.Context, skillID string) (*skillv1.Skill, error) {
	response, err := s.client.GetSkill(ctx, &managementv1.GetSkillRequest{SkillId: strings.TrimSpace(skillID)})
	if err != nil {
		return nil, err
	}
	return response.GetSkill(), nil
}

func (s *RemoteProfileProjectionSource) GetMCP(ctx context.Context, mcpID string) (*mcpv1.MCPServer, error) {
	response, err := s.client.GetMCPServer(ctx, &managementv1.GetMCPServerRequest{McpId: strings.TrimSpace(mcpID)})
	if err != nil {
		return nil, err
	}
	return response.GetMcp(), nil
}
