package agentsessionactions

import (
	"reflect"
	"time"

	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func statusSemanticallyEqual(previous *platformv1alpha1.AgentSessionActionResourceStatus, next *platformv1alpha1.AgentSessionActionResourceStatus) bool {
	return reflect.DeepEqual(normalizedStatus(previous), normalizedStatus(next))
}

func normalizedStatus(status *platformv1alpha1.AgentSessionActionResourceStatus) *platformv1alpha1.AgentSessionActionResourceStatus {
	if status == nil {
		return nil
	}
	out := status.DeepCopy()
	out.UpdatedAt = nil
	return out
}

func timePtr(value time.Time) *metav1.Time {
	out := metav1.NewTime(value)
	return &out
}
