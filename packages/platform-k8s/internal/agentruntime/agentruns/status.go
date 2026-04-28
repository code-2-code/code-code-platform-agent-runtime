package agentruns

import (
	"context"
	"reflect"

	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/platform/resourceops"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func updateStatus(ctx context.Context, client ctrlclient.Client, key types.NamespacedName, next *platformv1alpha1.AgentRunResourceStatus) error {
	return resourceops.UpdateStatus(ctx, client, key, func(current *platformv1alpha1.AgentRunResource) error {
		status := next.DeepCopy()
		if status.ResultSummary == nil && current.Status.ResultSummary != nil {
			status.ResultSummary = &platformv1alpha1.AgentRunResultSummary{
				Status:       current.Status.ResultSummary.Status,
				ErrorCode:    current.Status.ResultSummary.ErrorCode,
				ErrorMessage: current.Status.ResultSummary.ErrorMessage,
				Retryable:    current.Status.ResultSummary.Retryable,
			}
		}
		current.Status = *status
		return nil
	}, func() *platformv1alpha1.AgentRunResource {
		return &platformv1alpha1.AgentRunResource{}
	})
}

func statusSemanticallyEqual(previous *platformv1alpha1.AgentRunResourceStatus, next *platformv1alpha1.AgentRunResourceStatus) bool {
	return reflect.DeepEqual(normalizedStatus(previous), normalizedStatus(next))
}

func normalizedStatus(status *platformv1alpha1.AgentRunResourceStatus) *platformv1alpha1.AgentRunResourceStatus {
	if status == nil {
		return nil
	}
	out := status.DeepCopy()
	out.UpdatedAt = nil
	for index := range out.Conditions {
		out.Conditions[index].LastTransitionTime = metav1.Time{}
	}
	return out
}
