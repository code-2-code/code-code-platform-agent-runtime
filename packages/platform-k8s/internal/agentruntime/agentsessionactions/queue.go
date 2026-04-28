package agentsessionactions

import (
	"context"
	"sort"
	"strings"
	"time"

	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func listSessionActions(ctx context.Context, store Store, sessionID string) ([]platformv1alpha1.AgentSessionActionResource, error) {
	items, err := store.ListBySession(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		left := actionCreatedAt(&items[i])
		right := actionCreatedAt(&items[j])
		if left.Equal(right) {
			return items[i].Name < items[j].Name
		}
		return left.Before(right)
	})
	return items, nil
}

func queueOwnerID(items []platformv1alpha1.AgentSessionActionResource) string {
	active := firstActiveAction(items)
	if active != "" {
		return active
	}
	for i := range items {
		if isTerminalPhase(items[i].Status.Phase) {
			continue
		}
		return strings.TrimSpace(items[i].Name)
	}
	return ""
}

func firstActiveAction(items []platformv1alpha1.AgentSessionActionResource) string {
	for i := range items {
		if isActiveAction(&items[i]) {
			return strings.TrimSpace(items[i].Name)
		}
	}
	return ""
}

func isActiveAction(resource *platformv1alpha1.AgentSessionActionResource) bool {
	if resource == nil {
		return false
	}
	if isTerminalPhase(resource.Status.Phase) {
		return false
	}
	if strings.TrimSpace(resource.Status.RunID) != "" {
		return true
	}
	return resource.Status.Phase == platformv1alpha1.AgentSessionActionResourcePhaseRunning
}

func actionCreatedAt(resource *platformv1alpha1.AgentSessionActionResource) time.Time {
	if value := createdAt(resource); value != nil && !value.IsZero() {
		return value.UTC()
	}
	return time.Time{}
}

func createdAt(resource *platformv1alpha1.AgentSessionActionResource) *metav1.Time {
	if resource == nil {
		return nil
	}
	if resource.Status.CreatedAt != nil && !resource.Status.CreatedAt.IsZero() {
		return resource.Status.CreatedAt
	}
	if resource.CreationTimestamp.IsZero() {
		return nil
	}
	value := metav1.NewTime(resource.CreationTimestamp.UTC())
	return &value
}
