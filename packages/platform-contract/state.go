package platform

import (
	"context"

	agentproviderbindingv1 "code-code.internal/go-contract/platform/agent_provider_binding/v1"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformconditionv1 "code-code.internal/go-contract/platform/condition/v1"
	workloadprofilev1 "code-code.internal/go-contract/platform/workload_profile/v1"
)

// AgentProviderBinding describes one stored agent provider binding owned by the
// platform.
type AgentProviderBinding = agentproviderbindingv1.AgentProviderBinding

// WorkloadProfile describes how the platform should realize one runtime
// workload.
type WorkloadProfile = workloadprofilev1.WorkloadProfile

// AgentSessionPhase describes the observed lifecycle phase of one agent
// session.
type AgentSessionPhase = agentsessionv1.AgentSessionPhase

const (
	AgentSessionPhasePending AgentSessionPhase = agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_PENDING
	AgentSessionPhaseReady   AgentSessionPhase = agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_READY
	AgentSessionPhaseRunning AgentSessionPhase = agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_RUNNING
	AgentSessionPhaseFailed  AgentSessionPhase = agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_FAILED
)

// IsTerminalAgentSessionPhase reports whether the session phase is terminal.
func IsTerminalAgentSessionPhase(p AgentSessionPhase) bool {
	return p == AgentSessionPhaseFailed
}

// AgentRunPhase describes the observed lifecycle phase of one agent run.
type AgentRunPhase = agentrunv1.AgentRunPhase

const (
	AgentRunPhasePending   AgentRunPhase = agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_PENDING
	AgentRunPhaseScheduled AgentRunPhase = agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_SCHEDULED
	AgentRunPhaseRunning   AgentRunPhase = agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_RUNNING
	AgentRunPhaseSucceeded AgentRunPhase = agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_SUCCEEDED
	AgentRunPhaseFailed    AgentRunPhase = agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_FAILED
	AgentRunPhaseCanceled  AgentRunPhase = agentrunv1.AgentRunPhase_AGENT_RUN_PHASE_CANCELED
)

// IsTerminalAgentRunPhase reports whether the run phase is terminal.
func IsTerminalAgentRunPhase(p AgentRunPhase) bool {
	return p == AgentRunPhaseSucceeded || p == AgentRunPhaseFailed || p == AgentRunPhaseCanceled
}

// AgentSessionActionType describes the kind of durable action stored for one
// agentSession.
type AgentSessionActionType = agentsessionactionv1.AgentSessionActionType

const (
	AgentSessionActionTypeRunTurn       AgentSessionActionType = agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RUN_TURN
	AgentSessionActionTypeReloadSubject AgentSessionActionType = agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RELOAD_SUBJECT
)

// AgentSessionActionPhase describes the observed lifecycle phase of one durable
// action.
type AgentSessionActionPhase = agentsessionactionv1.AgentSessionActionPhase

const (
	AgentSessionActionPhasePending   AgentSessionActionPhase = agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_PENDING
	AgentSessionActionPhaseRunning   AgentSessionActionPhase = agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_RUNNING
	AgentSessionActionPhaseSucceeded AgentSessionActionPhase = agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_SUCCEEDED
	AgentSessionActionPhaseFailed    AgentSessionActionPhase = agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_FAILED
	AgentSessionActionPhaseCanceled  AgentSessionActionPhase = agentsessionactionv1.AgentSessionActionPhase_AGENT_SESSION_ACTION_PHASE_CANCELED
)

// IsTerminalAgentSessionActionPhase reports whether the action phase is
// terminal.
func IsTerminalAgentSessionActionPhase(p AgentSessionActionPhase) bool {
	return p == AgentSessionActionPhaseSucceeded || p == AgentSessionActionPhaseFailed || p == AgentSessionActionPhaseCanceled
}

// AgentSessionConditionStatus describes the truth status of one observed
// session condition.
type AgentSessionConditionStatus = platformconditionv1.ConditionStatus

const (
	AgentSessionConditionStatusTrue    AgentSessionConditionStatus = platformconditionv1.ConditionStatus_CONDITION_STATUS_TRUE
	AgentSessionConditionStatusFalse   AgentSessionConditionStatus = platformconditionv1.ConditionStatus_CONDITION_STATUS_FALSE
	AgentSessionConditionStatusUnknown AgentSessionConditionStatus = platformconditionv1.ConditionStatus_CONDITION_STATUS_UNKNOWN
)

// ConditionStatus describes the truth status of one observed condition.
type ConditionStatus = platformconditionv1.ConditionStatus

const (
	ConditionStatusTrue    ConditionStatus = platformconditionv1.ConditionStatus_CONDITION_STATUS_TRUE
	ConditionStatusFalse   ConditionStatus = platformconditionv1.ConditionStatus_CONDITION_STATUS_FALSE
	ConditionStatusUnknown ConditionStatus = platformconditionv1.ConditionStatus_CONDITION_STATUS_UNKNOWN
)

// AgentSessionCondition describes one observed condition for an agent session.
type AgentSessionCondition = platformconditionv1.Condition

// AgentSessionStatus describes observed state for one agent session.
type AgentSessionStatus = agentsessionv1.AgentSessionStatus

// AgentSessionState combines desired and observed state for one agent session.
type AgentSessionState = agentsessionv1.AgentSessionState

// AgentRunCondition describes one observed condition for an agent run.
type AgentRunCondition = platformconditionv1.Condition

// AgentRunStatus describes observed state for one agent run.
type AgentRunStatus = agentrunv1.AgentRunStatus

// AgentRunState combines desired and observed state for one agent run.
type AgentRunState = agentrunv1.AgentRunState

// AgentSessionActionStatus describes observed state for one durable action.
type AgentSessionActionStatus = agentsessionactionv1.AgentSessionActionStatus

// AgentSessionActionState combines desired and observed state for one durable
// action.
type AgentSessionActionState = agentsessionactionv1.AgentSessionActionState

// AgentProviderBindingReader reads platform-owned agent provider bindings.
type AgentProviderBindingReader interface {
	// Get returns one platform-owned agent provider binding.
	Get(ctx context.Context, providerID string) (*AgentProviderBinding, error)
}

// WorkloadProfileReader reads runtime workload profiles.
type WorkloadProfileReader interface {
	// Get returns one configured workload profile.
	Get(ctx context.Context, profileID string) (*WorkloadProfile, error)
}

// AgentSessionReader reads desired and observed state for one agent session.
type AgentSessionReader interface {
	// Get returns desired and observed state for one agent session.
	Get(ctx context.Context, sessionID string) (*AgentSessionState, error)
}

// AgentSessionStatusWriter writes observed status for one agent session.
type AgentSessionStatusWriter interface {
	// UpdateStatus stores observed status for one agent session if the stored
	// desired generation still matches expectedGeneration. Implementations must
	// reject stale writes when expectedGeneration does not match the current
	// stored generation. sessionID and status.SessionId must identify the same
	// session.
	UpdateStatus(ctx context.Context, sessionID string, expectedGeneration int64, status *AgentSessionStatus) error
}

// AgentRunReader reads desired and observed state for one agent run.
type AgentRunReader interface {
	// Get returns desired and observed state for one agent run.
	Get(ctx context.Context, runID string) (*AgentRunState, error)
}

// AgentRunStatusWriter writes observed status for one agent run.
type AgentRunStatusWriter interface {
	// UpdateStatus stores observed status for one agent run if the stored desired
	// generation still matches expectedGeneration. Implementations must reject
	// stale writes when expectedGeneration does not match the current stored
	// generation. runID and status.RunId must identify the same run.
	UpdateStatus(ctx context.Context, runID string, expectedGeneration int64, status *AgentRunStatus) error
}
