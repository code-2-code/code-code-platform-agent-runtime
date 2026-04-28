package protostate

import (
	conditionv1 "code-code.internal/go-contract/platform/condition/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Conditions(conditions []metav1.Condition) []*conditionv1.Condition {
	if len(conditions) == 0 {
		return nil
	}
	out := make([]*conditionv1.Condition, 0, len(conditions))
	for i := range conditions {
		out = append(out, &conditionv1.Condition{
			Type:               conditions[i].Type,
			Status:             ConditionStatus(conditions[i].Status),
			Reason:             conditions[i].Reason,
			Message:            conditions[i].Message,
			ObservedGeneration: conditions[i].ObservedGeneration,
			LastTransitionTime: timestamppb.New(conditions[i].LastTransitionTime.UTC()),
		})
	}
	return out
}

func ConditionStatus(status metav1.ConditionStatus) conditionv1.ConditionStatus {
	switch status {
	case metav1.ConditionTrue:
		return conditionv1.ConditionStatus_CONDITION_STATUS_TRUE
	case metav1.ConditionFalse:
		return conditionv1.ConditionStatus_CONDITION_STATUS_FALSE
	default:
		return conditionv1.ConditionStatus_CONDITION_STATUS_UNKNOWN
	}
}

func Timestamp(value *metav1.Time) *timestamppb.Timestamp {
	if value == nil || value.IsZero() {
		return nil
	}
	return timestamppb.New(value.UTC())
}
