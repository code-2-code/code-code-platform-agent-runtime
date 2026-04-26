package platform

import "fmt"

// AgentSessionConditionType identifies one stable condition type exposed by
// the platform control plane for agent sessions.
type AgentSessionConditionType string

const (
	AgentSessionConditionTypeWorkspaceReady      AgentSessionConditionType = "WorkspaceReady"
	AgentSessionConditionTypeWarmStateReady      AgentSessionConditionType = "WarmStateReady"
	AgentSessionConditionTypeRuntimeConfigReady  AgentSessionConditionType = "RuntimeConfigReady"
	AgentSessionConditionTypeResourceConfigReady AgentSessionConditionType = "ResourceConfigReady"
	AgentSessionConditionTypeReadyForNextRun     AgentSessionConditionType = "ReadyForNextRun"
)

// AgentSessionConditionReason identifies one stable CamelCase reason category.
type AgentSessionConditionReason string

const (
	AgentSessionConditionReasonWorkspacePrepared          AgentSessionConditionReason = "WorkspacePrepared"
	AgentSessionConditionReasonWorkspaceUnavailable       AgentSessionConditionReason = "WorkspaceUnavailable"
	AgentSessionConditionReasonWarmStatePrepared          AgentSessionConditionReason = "WarmStatePrepared"
	AgentSessionConditionReasonWarmStateReset             AgentSessionConditionReason = "WarmStateReset"
	AgentSessionConditionReasonWarmStateUnavailable       AgentSessionConditionReason = "WarmStateUnavailable"
	AgentSessionConditionReasonRuntimeConfigPrepared      AgentSessionConditionReason = "RuntimeConfigPrepared"
	AgentSessionConditionReasonRuntimeConfigInvalid       AgentSessionConditionReason = "RuntimeConfigInvalid"
	AgentSessionConditionReasonRuntimeConfigRevoked       AgentSessionConditionReason = "RuntimeConfigRevoked"
	AgentSessionConditionReasonResourceConfigPrepared     AgentSessionConditionReason = "ResourceConfigPrepared"
	AgentSessionConditionReasonResourceConfigInvalid      AgentSessionConditionReason = "ResourceConfigInvalid"
	AgentSessionConditionReasonResourceConfigIncompatible AgentSessionConditionReason = "ResourceConfigIncompatible"
	AgentSessionConditionReasonReady                      AgentSessionConditionReason = "Ready"
	AgentSessionConditionReasonSessionNotReady            AgentSessionConditionReason = "SessionNotReady"
	AgentSessionConditionReasonActiveRunInProgress        AgentSessionConditionReason = "ActiveRunInProgress"
	AgentSessionConditionReasonSessionSuspended           AgentSessionConditionReason = "SessionSuspended"
	AgentSessionConditionReasonSessionClosed              AgentSessionConditionReason = "SessionClosed"
)

var sessionConditionReasonsByType = map[AgentSessionConditionType]map[AgentSessionConditionReason]struct{}{
	AgentSessionConditionTypeWorkspaceReady: {
		AgentSessionConditionReasonWorkspacePrepared:    {},
		AgentSessionConditionReasonWorkspaceUnavailable: {},
	},
	AgentSessionConditionTypeWarmStateReady: {
		AgentSessionConditionReasonWarmStatePrepared:    {},
		AgentSessionConditionReasonWarmStateReset:       {},
		AgentSessionConditionReasonWarmStateUnavailable: {},
	},
	AgentSessionConditionTypeRuntimeConfigReady: {
		AgentSessionConditionReasonRuntimeConfigPrepared: {},
		AgentSessionConditionReasonRuntimeConfigInvalid:  {},
		AgentSessionConditionReasonRuntimeConfigRevoked:  {},
	},
	AgentSessionConditionTypeResourceConfigReady: {
		AgentSessionConditionReasonResourceConfigPrepared:     {},
		AgentSessionConditionReasonResourceConfigInvalid:      {},
		AgentSessionConditionReasonResourceConfigIncompatible: {},
	},
	AgentSessionConditionTypeReadyForNextRun: {
		AgentSessionConditionReasonReady:               {},
		AgentSessionConditionReasonSessionNotReady:     {},
		AgentSessionConditionReasonActiveRunInProgress: {},
		AgentSessionConditionReasonSessionSuspended:    {},
		AgentSessionConditionReasonSessionClosed:       {},
	},
}

// IsKnownAgentSessionConditionType reports whether value is part of the stable
// v1 condition type vocabulary.
func IsKnownAgentSessionConditionType(value string) bool {
	switch AgentSessionConditionType(value) {
	case AgentSessionConditionTypeWorkspaceReady,
		AgentSessionConditionTypeWarmStateReady,
		AgentSessionConditionTypeRuntimeConfigReady,
		AgentSessionConditionTypeResourceConfigReady,
		AgentSessionConditionTypeReadyForNextRun:
		return true
	default:
		return false
	}
}

// ValidateAgentSessionCondition validates one platform session condition entry.
func ValidateAgentSessionCondition(condition *AgentSessionCondition) error {
	if condition == nil {
		return fmt.Errorf("platform: agent session condition is nil")
	}
	if condition.Type == "" {
		return fmt.Errorf("platform: agent session condition type is empty")
	}
	if !IsKnownAgentSessionConditionType(condition.Type) {
		return fmt.Errorf("platform: agent session condition type %q is not part of the stable v1 vocabulary", condition.Type)
	}
	if condition.Status == agentSessionConditionStatusUnspecified() {
		return fmt.Errorf("platform: agent session condition status is unspecified")
	}
	if condition.Reason == "" {
		return fmt.Errorf("platform: agent session condition reason is empty")
	}
	if !camelCaseReasonPattern.MatchString(condition.Reason) {
		return fmt.Errorf("platform: agent session condition reason %q must be CamelCase", condition.Reason)
	}
	if !isAllowedReasonForSessionConditionType(AgentSessionConditionType(condition.Type), AgentSessionConditionReason(condition.Reason)) {
		return fmt.Errorf("platform: agent session condition reason %q is not allowed for type %q", condition.Reason, condition.Type)
	}
	if condition.ObservedGeneration < 0 {
		return fmt.Errorf("platform: agent session condition observed generation must be non-negative")
	}
	return nil
}

// ValidateAgentSessionStatus validates the condition set shape exposed by one
// session status.
func ValidateAgentSessionStatus(status *AgentSessionStatus) error {
	if status == nil {
		return fmt.Errorf("platform: agent session status is nil")
	}
	if status.SessionId == "" {
		return fmt.Errorf("platform: agent session status session id is empty")
	}
	if status.Phase == agentSessionPhaseUnspecified() {
		return fmt.Errorf("platform: agent session status phase is unspecified")
	}
	if status.ObservedGeneration < 0 {
		return fmt.Errorf("platform: agent session status observed generation must be non-negative")
	}
	if status.RuntimeConfigGeneration < 0 {
		return fmt.Errorf("platform: agent session status runtime config generation must be non-negative")
	}
	if status.ResourceConfigGeneration < 0 {
		return fmt.Errorf("platform: agent session status resource config generation must be non-negative")
	}
	if status.StateGeneration < 0 {
		return fmt.Errorf("platform: agent session status state generation must be non-negative")
	}
	seen := map[string]struct{}{}
	for _, condition := range status.Conditions {
		if err := ValidateAgentSessionCondition(condition); err != nil {
			return err
		}
		if condition.ObservedGeneration > status.ObservedGeneration {
			return fmt.Errorf("platform: session condition observed generation %d exceeds status observed generation %d", condition.ObservedGeneration, status.ObservedGeneration)
		}
		if _, ok := seen[condition.Type]; ok {
			return fmt.Errorf("platform: duplicate agent session condition type %q", condition.Type)
		}
		seen[condition.Type] = struct{}{}
	}
	return nil
}

func agentSessionConditionStatusUnspecified() AgentSessionConditionStatus {
	return 0
}

func agentSessionPhaseUnspecified() AgentSessionPhase {
	return 0
}

func isAllowedReasonForSessionConditionType(conditionType AgentSessionConditionType, reason AgentSessionConditionReason) bool {
	reasons, ok := sessionConditionReasonsByType[conditionType]
	if !ok {
		return false
	}
	_, ok = reasons[reason]
	return ok
}
