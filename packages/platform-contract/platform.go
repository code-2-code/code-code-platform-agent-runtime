// Package platform defines platform-owned control-plane contracts that compose
// agent, credential, model, network policy, provider, and workload resources.
package platform

import (
	"context"

	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
)

// AgentSessionSpec describes the desired state for one agent session.
type AgentSessionSpec = agentsessionv1.AgentSessionSpec

// AgentRunSpec describes the desired state for one agent run.
type AgentRunSpec = agentrunv1.AgentRunSpec

// AgentSessionActionSpec describes one durable action in one agentSession
// serialization domain.
type AgentSessionActionSpec = agentsessionactionv1.AgentSessionActionSpec

// AgentSessionCreator creates one desired agent session resource.
type AgentSessionCreator interface {
	// CreateSession validates, resolves submission-time defaults, and creates
	// one desired agent session.
	CreateSession(ctx context.Context, spec *AgentSessionSpec) (*AgentSessionRef, error)
}

// AgentSessionReconciler converges one stored agent session toward ready state.
type AgentSessionReconciler interface {
	// ReconcileSession drives one session toward its desired state and updates
	// status.
	ReconcileSession(ctx context.Context, sessionID string) error
}

// AgentRunCreator creates one desired agent run resource.
type AgentRunCreator interface {
	// CreateRun validates, freezes turn-bound generations, and creates one
	// desired agent run record.
	CreateRun(ctx context.Context, spec *AgentRunSpec) (*AgentRunRef, error)
}

// AgentRunReconciler converges one stored agent run toward runtime state.
type AgentRunReconciler interface {
	// ReconcileRun drives one run toward its desired state and updates status.
	ReconcileRun(ctx context.Context, runID string) error
}

// AgentSessionRef identifies one submitted agent session.
type AgentSessionRef = agentsessionv1.AgentSessionRef

// AgentRunRef identifies one submitted agent run.
type AgentRunRef = agentrunv1.AgentRunRef

// AgentSessionActionRef identifies one submitted durable action.
type AgentSessionActionRef = agentsessionactionv1.AgentSessionActionRef

// WorkloadRef identifies the platform workload backing one agent run.
type WorkloadRef = agentrunv1.WorkloadRef
