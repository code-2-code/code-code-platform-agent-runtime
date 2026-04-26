package session

import (
	"strings"
	"time"

	conditionv1 "code-code.internal/go-contract/platform/condition/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type agentSessionCondition struct {
	Type               string `json:"type,omitempty"`
	Status             string `json:"status,omitempty"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

func conditionsFromResource(items []agentSessionCondition) []*conditionv1.Condition {
	if len(items) == 0 {
		return nil
	}
	out := make([]*conditionv1.Condition, 0, len(items))
	for _, item := range items {
		condition := &conditionv1.Condition{
			Type:               item.Type,
			Status:             conditionStatusFromResource(item.Status),
			Reason:             item.Reason,
			Message:            item.Message,
			ObservedGeneration: item.ObservedGeneration,
		}
		if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(item.LastTransitionTime)); err == nil {
			condition.LastTransitionTime = timestamppb.New(parsed)
		}
		out = append(out, condition)
	}
	return out
}

func conditionsToResource(items []*conditionv1.Condition) []agentSessionCondition {
	if len(items) == 0 {
		return nil
	}
	out := make([]agentSessionCondition, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		condition := agentSessionCondition{
			Type:               item.GetType(),
			Status:             resourceConditionStatus(item.GetStatus()),
			Reason:             item.GetReason(),
			Message:            item.GetMessage(),
			ObservedGeneration: item.GetObservedGeneration(),
		}
		if item.GetLastTransitionTime() != nil {
			condition.LastTransitionTime = item.GetLastTransitionTime().AsTime().UTC().Format(time.RFC3339Nano)
		}
		out = append(out, condition)
	}
	return out
}

func conditionStatusFromResource(status string) conditionv1.ConditionStatus {
	switch strings.TrimSpace(status) {
	case "True", "true", "CONDITION_STATUS_TRUE":
		return conditionv1.ConditionStatus_CONDITION_STATUS_TRUE
	case "False", "false", "CONDITION_STATUS_FALSE":
		return conditionv1.ConditionStatus_CONDITION_STATUS_FALSE
	case "Unknown", "unknown", "CONDITION_STATUS_UNKNOWN":
		return conditionv1.ConditionStatus_CONDITION_STATUS_UNKNOWN
	default:
		return conditionv1.ConditionStatus_CONDITION_STATUS_UNSPECIFIED
	}
}

func resourceConditionStatus(status conditionv1.ConditionStatus) string {
	switch status {
	case conditionv1.ConditionStatus_CONDITION_STATUS_TRUE:
		return "True"
	case conditionv1.ConditionStatus_CONDITION_STATUS_FALSE:
		return "False"
	case conditionv1.ConditionStatus_CONDITION_STATUS_UNKNOWN:
		return "Unknown"
	default:
		return ""
	}
}
