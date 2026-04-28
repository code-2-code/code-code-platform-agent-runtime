package agentsessionactions

import (
	"testing"
	"time"

	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentruns"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessions"
)

func TestNewReconcilerUsesDefaultRetryPolicy(t *testing.T) {
	t.Parallel()

	client := newClient(t, readySessionResource())
	sessions := fakeActionSessionRepository{client: client, namespace: "code-code"}
	slots, err := agentsessions.NewActiveRunManager(sessions, "code-code")
	if err != nil {
		t.Fatalf("NewActiveRunManager() error = %v", err)
	}
	runs, err := agentruns.NewService(client, client, "code-code", nil, agentruns.WithActiveRunSlots(slots))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	reconciler, err := NewReconciler(ReconcilerConfig{
		Client:    client,
		Namespace: "code-code",
		Runs:      runs,
		Store:     newFakeActionStore(client),
		Sessions:  sessions,
	})
	if err != nil {
		t.Fatalf("NewReconciler() error = %v", err)
	}
	if got, want := reconciler.retryPolicy, DefaultRetryPolicy(); got != want {
		t.Fatalf("retry policy = %+v, want %+v", got, want)
	}
}

func TestNewReconcilerUsesConfiguredRetryPolicy(t *testing.T) {
	t.Parallel()

	client := newClient(t, readySessionResource())
	sessions := fakeActionSessionRepository{client: client, namespace: "code-code"}
	slots, err := agentsessions.NewActiveRunManager(sessions, "code-code")
	if err != nil {
		t.Fatalf("NewActiveRunManager() error = %v", err)
	}
	runs, err := agentruns.NewService(client, client, "code-code", nil, agentruns.WithActiveRunSlots(slots))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	input := RetryPolicy{
		MaxRetries:  1,
		BaseBackoff: 3 * time.Second,
		MaxBackoff:  9 * time.Second,
	}
	reconciler, err := NewReconciler(ReconcilerConfig{
		Client:      client,
		Namespace:   "code-code",
		Runs:        runs,
		Store:       newFakeActionStore(client),
		Sessions:    sessions,
		RetryPolicy: &input,
	})
	if err != nil {
		t.Fatalf("NewReconciler() error = %v", err)
	}
	if got, want := reconciler.retryPolicy, input; got != want {
		t.Fatalf("retry policy = %+v, want %+v", got, want)
	}
}

func TestScheduleRetryStatusUsesPolicyBackoff(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 17, 16, 0, 0, 0, time.UTC)
	action := pendingActionResource("action-1", now)
	policy := RetryPolicy{
		MaxRetries:  2,
		BaseBackoff: 5 * time.Second,
		MaxBackoff:  20 * time.Second,
	}

	status, result := scheduleRetryStatus(
		action,
		now,
		"temporary failure",
		platformv1alpha1.AgentSessionActionResourceFailureClassManualRetry,
		policy,
	)

	if got, want := status.RetryCount, int32(1); got != want {
		t.Fatalf("retry_count = %d, want %d", got, want)
	}
	if status.NextRetryAt == nil || !status.NextRetryAt.Time.Equal(now.Add(5*time.Second)) {
		t.Fatalf("next_retry_at = %v, want %v", status.NextRetryAt, now.Add(5*time.Second))
	}
	if got, want := result.RequeueAfter, 5*time.Second; got != want {
		t.Fatalf("requeue_after = %s, want %s", got, want)
	}
}

func TestScheduleRetryStatusCanDisableAutomaticRetry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 17, 16, 0, 0, 0, time.UTC)
	action := pendingActionResource("action-1", now)
	policy := RetryPolicy{
		MaxRetries: 0,
	}

	status, _ := scheduleRetryStatus(
		action,
		now,
		"temporary failure",
		platformv1alpha1.AgentSessionActionResourceFailureClassManualRetry,
		policy,
	)

	if got, want := status.Phase, platformv1alpha1.AgentSessionActionResourcePhaseFailed; got != want {
		t.Fatalf("phase = %q, want %q", got, want)
	}
	if status.NextRetryAt != nil {
		t.Fatalf("next_retry_at = %v, want nil", status.NextRetryAt)
	}
	if got, want := status.RetryCount, int32(0); got != want {
		t.Fatalf("retry_count = %d, want %d", got, want)
	}
}
