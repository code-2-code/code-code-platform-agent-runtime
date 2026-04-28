package sessionapi

import (
	"context"

	resultv1 "code-code.internal/go-contract/agent/result/v1"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentruns"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessionactions"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessions"
)

type agentSessionService interface {
	Get(ctx context.Context, sessionID string) (*agentsessionv1.AgentSessionState, error)
}

type agentSessionActionService interface {
	Get(ctx context.Context, actionID string) (*agentsessionactionv1.AgentSessionActionState, error)
	Create(ctx context.Context, sessionID string, request *agentsessionactions.CreateRequest) (*agentsessionactionv1.AgentSessionActionState, error)
	ResetWarmState(ctx context.Context, sessionID string, request *agentsessionactions.ResetWarmStateRequest) (*agentsessionactionv1.AgentSessionActionState, error)
	Stop(ctx context.Context, actionID string) (*agentsessionactionv1.AgentSessionActionState, error)
	Retry(ctx context.Context, sourceActionID string, request *agentsessionactions.RetryRequest) (*agentsessionactionv1.AgentSessionActionState, error)
}

type agentRunService interface {
	Get(ctx context.Context, runID string) (*agentrunv1.AgentRunState, error)
	PublishTerminalResult(ctx context.Context, runID string, result *resultv1.RunResult) error
}

var (
	_ agentSessionService       = (*agentsessions.Service)(nil)
	_ agentSessionActionService = (*agentsessionactions.Service)(nil)
	_ agentRunService           = (*agentruns.Service)(nil)
)
