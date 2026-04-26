package session

import (
	"context"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
)

type Repository interface {
	Get(context.Context, string) (*agentsessionv1.AgentSessionState, error)
	Create(context.Context, *agentsessionv1.AgentSessionSpec) (*agentsessionv1.AgentSessionState, error)
	Update(context.Context, string, *agentsessionv1.AgentSessionSpec) (*agentsessionv1.AgentSessionState, error)
	UpdateStatus(context.Context, string, *agentsessionv1.AgentSessionStatus) (*agentsessionv1.AgentSessionState, error)
}
