package platform

import (
	"fmt"
	"regexp"
)

// AgentRunConditionType identifies one stable condition type exposed by the
// platform control plane.
type AgentRunConditionType string

const (
	AgentRunConditionTypeAccepted         AgentRunConditionType = "Accepted"
	AgentRunConditionTypeProviderResolved AgentRunConditionType = "ProviderResolved"
	AgentRunConditionTypeWorkloadReady    AgentRunConditionType = "WorkloadReady"
	AgentRunConditionTypeCompleted        AgentRunConditionType = "Completed"
)

// AgentRunConditionReason identifies one stable CamelCase reason category.
type AgentRunConditionReason string

const (
	AgentRunConditionReasonAccepted                 AgentRunConditionReason = "Accepted"
	AgentRunConditionReasonInvalidSpec              AgentRunConditionReason = "InvalidSpec"
	AgentRunConditionReasonProviderResolved         AgentRunConditionReason = "ProviderResolved"
	AgentRunConditionReasonProviderResolutionFailed AgentRunConditionReason = "ProviderResolutionFailed"
	AgentRunConditionReasonWorkloadCreated          AgentRunConditionReason = "WorkloadCreated"
	AgentRunConditionReasonWorkloadCreateFailed     AgentRunConditionReason = "WorkloadCreateFailed"
	AgentRunConditionReasonRunStarted               AgentRunConditionReason = "RunStarted"
	AgentRunConditionReasonRunSucceeded             AgentRunConditionReason = "RunSucceeded"
	AgentRunConditionReasonRunFailed                AgentRunConditionReason = "RunFailed"
	AgentRunConditionReasonRunCanceled              AgentRunConditionReason = "RunCanceled"
)

var camelCaseReasonPattern = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)

var conditionReasonsByType = map[AgentRunConditionType]map[AgentRunConditionReason]struct{}{
	AgentRunConditionTypeAccepted: {
		AgentRunConditionReasonAccepted:    {},
		AgentRunConditionReasonInvalidSpec: {},
	},
	AgentRunConditionTypeProviderResolved: {
		AgentRunConditionReasonProviderResolved:         {},
		AgentRunConditionReasonProviderResolutionFailed: {},
	},
	AgentRunConditionTypeWorkloadReady: {
		AgentRunConditionReasonWorkloadCreated:      {},
		AgentRunConditionReasonWorkloadCreateFailed: {},
		AgentRunConditionReasonRunStarted:           {},
	},
	AgentRunConditionTypeCompleted: {
		AgentRunConditionReasonRunSucceeded: {},
		AgentRunConditionReasonRunFailed:    {},
		AgentRunConditionReasonRunCanceled:  {},
	},
}

// IsKnownAgentRunConditionType reports whether value is part of the stable v1
// condition type vocabulary.
func IsKnownAgentRunConditionType(value string) bool {
	switch AgentRunConditionType(value) {
	case AgentRunConditionTypeAccepted,
		AgentRunConditionTypeProviderResolved,
		AgentRunConditionTypeWorkloadReady,
		AgentRunConditionTypeCompleted:
		return true
	default:
		return false
	}
}

// ValidateAgentRunCondition validates one platform condition entry.
func ValidateAgentRunCondition(condition *AgentRunCondition) error {
	if condition == nil {
		return fmt.Errorf("platform: agent run condition is nil")
	}
	if condition.Type == "" {
		return fmt.Errorf("platform: agent run condition type is empty")
	}
	if !IsKnownAgentRunConditionType(condition.Type) {
		return fmt.Errorf("platform: agent run condition type %q is not part of the stable v1 vocabulary", condition.Type)
	}
	if condition.Status == agentRunConditionStatusUnspecified() {
		return fmt.Errorf("platform: agent run condition status is unspecified")
	}
	if condition.Reason == "" {
		return fmt.Errorf("platform: agent run condition reason is empty")
	}
	if !camelCaseReasonPattern.MatchString(condition.Reason) {
		return fmt.Errorf("platform: agent run condition reason %q must be CamelCase", condition.Reason)
	}
	if !isAllowedReasonForConditionType(AgentRunConditionType(condition.Type), AgentRunConditionReason(condition.Reason)) {
		return fmt.Errorf("platform: agent run condition reason %q is not allowed for type %q", condition.Reason, condition.Type)
	}
	if condition.ObservedGeneration < 0 {
		return fmt.Errorf("platform: agent run condition observed generation must be non-negative")
	}
	return nil
}

// ValidateAgentRunStatus validates the condition set shape exposed by one run status.
func ValidateAgentRunStatus(status *AgentRunStatus) error {
	if status == nil {
		return fmt.Errorf("platform: agent run status is nil")
	}
	if status.RunId == "" {
		return fmt.Errorf("platform: agent run status run id is empty")
	}
	if status.Phase == agentRunPhaseUnspecified() {
		return fmt.Errorf("platform: agent run status phase is unspecified")
	}
	if status.ObservedGeneration < 0 {
		return fmt.Errorf("platform: agent run status observed generation must be non-negative")
	}
	seen := map[string]struct{}{}
	for _, condition := range status.Conditions {
		if err := ValidateAgentRunCondition(condition); err != nil {
			return err
		}
		if condition.ObservedGeneration > status.ObservedGeneration {
			return fmt.Errorf("platform: condition observed generation %d exceeds status observed generation %d", condition.ObservedGeneration, status.ObservedGeneration)
		}
		if _, ok := seen[condition.Type]; ok {
			return fmt.Errorf("platform: duplicate agent run condition type %q", condition.Type)
		}
		seen[condition.Type] = struct{}{}
	}
	return nil
}

func agentRunConditionStatusUnspecified() ConditionStatus {
	return 0
}

func agentRunPhaseUnspecified() AgentRunPhase {
	return 0
}

func isAllowedReasonForConditionType(conditionType AgentRunConditionType, reason AgentRunConditionReason) bool {
	reasons, ok := conditionReasonsByType[conditionType]
	if !ok {
		return false
	}
	_, ok = reasons[reason]
	return ok
}
