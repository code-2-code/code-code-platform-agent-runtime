package agentsessionactions

import (
	"context"
	"testing"
	"time"

	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentresourceconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReconcilerCreatesRunForReadyHeadAction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	action := pendingActionResource("action-1", time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC))
	client := newClient(t, readySessionResource(), action)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := getActionResource(t, ctx, client, "action-1")
	if got, want := updated.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhaseRunning; got != want {
		t.Fatalf("phase = %q, want %q", got, want)
	}
	if got, want := updated.Status.RunID, "action-1"; got != want {
		t.Fatalf("run_id = %q, want %q", got, want)
	}
	run := getRunResource(t, ctx, client, "action-1")
	if got, want := run.Spec.Run.GetRequest().GetInput().GetText(), "hello"; got != want {
		t.Fatalf("run.request.input.text = %q, want %q", got, want)
	}
}

func TestReconcilerKeepsActionPendingWhenSessionNotReady(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	session := readySessionResource()
	session.Status.Phase = platformv1alpha1.AgentSessionResourcePhasePending
	session.Status.Conditions[0].Status = metav1.ConditionFalse
	action := pendingActionResource("action-1", time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC))
	client := newClient(t, session, action)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := getActionResource(t, ctx, client, "action-1")
	if got, want := updated.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhasePending; got != want {
		t.Fatalf("phase = %q, want %q", got, want)
	}
}

func TestReconcilerDispatchesFrozenActionWhenCurrentSessionIsNotReadyForNextRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	session := readySessionResource()
	session.Status.Phase = platformv1alpha1.AgentSessionResourcePhasePending
	session.Status.Conditions[len(session.Status.Conditions)-1].Status = metav1.ConditionFalse
	action := pendingActionResource("action-1", time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC))
	client := newClient(t, session, action)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := getActionResource(t, ctx, client, "action-1")
	if got, want := updated.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhaseRunning; got != want {
		t.Fatalf("phase = %q, want %q", got, want)
	}
	run := getRunResource(t, ctx, client, "action-1")
	if got, want := run.Spec.Run.GetContainerImage(), "ghcr.io/openai/codex:latest"; got != want {
		t.Fatalf("run.container_image = %q, want %q", got, want)
	}
}

func TestReconcilerCompletesActionFromTerminalRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	action := pendingActionResource("action-1", time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC))
	action.Status = platformv1alpha1.AgentSessionActionResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{ObservedGeneration: 1},
		Phase:              platformv1alpha1.AgentSessionActionResourcePhaseRunning,
		RunID:              "action-1",
		CreatedAt:          &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC)},
		UpdatedAt:          &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 2, 0, time.UTC)},
	}
	run := getTerminalRun("action-1")
	client := newClient(t, readySessionResource(), action, run)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := getActionResource(t, ctx, client, "action-1")
	if got, want := updated.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhaseSucceeded; got != want {
		t.Fatalf("phase = %q, want %q", got, want)
	}
}

func TestReconcilerSchedulesRetryBeforeFallback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	action := pendingActionResource("action-1", time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC))
	action.Status = platformv1alpha1.AgentSessionActionResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{ObservedGeneration: 1},
		Phase:              platformv1alpha1.AgentSessionActionResourcePhaseRunning,
		RunID:              "action-1",
		CreatedAt:          &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC)},
		UpdatedAt:          &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 2, 0, time.UTC)},
	}
	run := getFailedRun("action-1", true)
	client := newClient(t, readySessionResource(), action, run)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := getActionResource(t, ctx, client, "action-1")
	if got, want := updated.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhasePending; got != want {
		t.Fatalf("phase = %q, want %q", got, want)
	}
	if got, want := updated.Status.FailureClass, platformv1alpha1.AgentSessionActionResourceFailureClassTransient; got != want {
		t.Fatalf("failure_class = %q, want %q", got, want)
	}
	if got, want := updated.Status.RetryCount, int32(1); got != want {
		t.Fatalf("retry_count = %d, want %d", got, want)
	}
	if updated.Status.NextRetryAt == nil {
		t.Fatal("next_retry_at = nil, want scheduled retry")
	}
}

func TestReconcilerSchedulesAndCreatesFallbackAttemptAfterRetryBudgetExhausted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	action := pendingActionResource("action-1", time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC))
	action.Spec.Action.GetInputSnapshot().GetRunTurn().RuntimeCandidates = append(
		action.Spec.Action.GetInputSnapshot().GetRunTurn().GetRuntimeCandidates(),
		&agentsessionactionv1.AgentSessionRuntimeCandidate{
			ResolvedProviderModel: testResolvedProviderModel("openai-backup", "https://backup.api.openai.com/v1", "gpt-4.1-mini"),
			AuthRequirement: &agentrunv1.AgentRunAuthRequirement{
				ProviderId:               "codex",
				ProviderSurfaceBindingId: "openai-backup",
				AuthStatus:               "bound",
			},
		},
	)
	action.Status = platformv1alpha1.AgentSessionActionResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{ObservedGeneration: 1},
		Phase:              platformv1alpha1.AgentSessionActionResourcePhaseRunning,
		RunID:              "action-1",
		RetryCount:         automaticRetryLimit,
		AttemptCount:       1,
		CandidateIndex:     0,
		CreatedAt:          &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC)},
		UpdatedAt:          &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 2, 0, time.UTC)},
	}
	run := getFailedRun("action-1", true)
	client := newClient(t, readySessionResource(), action, run)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}

	scheduled := getActionResource(t, ctx, client, "action-1")
	if got, want := scheduled.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhasePending; got != want {
		t.Fatalf("scheduled phase = %q, want %q", got, want)
	}
	if got, want := scheduled.Status.CandidateIndex, int32(1); got != want {
		t.Fatalf("scheduled candidate_index = %d, want %d", got, want)
	}
	if got, want := scheduled.Status.RetryCount, int32(0); got != want {
		t.Fatalf("scheduled retry_count = %d, want %d", got, want)
	}
	if got, want := scheduled.Status.AttemptCount, int32(1); got != want {
		t.Fatalf("scheduled attempt_count = %d, want %d", got, want)
	}
	if got := scheduled.Status.RunID; got != "" {
		t.Fatalf("scheduled run_id = %q, want empty", got)
	}

	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}

	updated := getActionResource(t, ctx, client, "action-1")
	if got, want := updated.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhaseRunning; got != want {
		t.Fatalf("updated phase = %q, want %q", got, want)
	}
	if got, want := updated.Status.RunID, "action-1-attempt-2"; got != want {
		t.Fatalf("updated run_id = %q, want %q", got, want)
	}
	if got, want := updated.Status.AttemptCount, int32(2); got != want {
		t.Fatalf("updated attempt_count = %d, want %d", got, want)
	}
	if got, want := updated.Status.CandidateIndex, int32(1); got != want {
		t.Fatalf("updated candidate_index = %d, want %d", got, want)
	}
	retryRun := getRunResource(t, ctx, client, "action-1-attempt-2")
	if got, want := retryRun.Spec.Run.GetRequest().GetRunId(), "action-1-attempt-2"; got != want {
		t.Fatalf("retry run.request.run_id = %q, want %q", got, want)
	}
	if got, want := retryRun.Spec.Run.GetRequest().GetResolvedProviderModel().GetSurfaceId(), "openai-backup"; got != want {
		t.Fatalf("retry resolved_provider_model.surface_id = %q, want %q", got, want)
	}
	if got, want := retryRun.Spec.Run.GetRequest().GetResolvedProviderModel().GetProviderModelId(), "gpt-4.1-mini"; got != want {
		t.Fatalf("retry resolved_provider_model.provider_model_id = %q, want %q", got, want)
	}
	if got, want := retryRun.Spec.Run.GetAuthRequirement().GetProviderSurfaceBindingId(), "openai-backup"; got != want {
		t.Fatalf("retry auth_requirement.provider_surface_binding_id = %q, want %q", got, want)
	}
}

func TestReconcilerKeepsScheduledRetryWindow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	action := pendingActionResource("action-1", time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC))
	action.Status = platformv1alpha1.AgentSessionActionResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{ObservedGeneration: 1},
		Phase:              platformv1alpha1.AgentSessionActionResourcePhasePending,
		FailureClass:       platformv1alpha1.AgentSessionActionResourceFailureClassTransient,
		Message:            "AgentSessionAction automatic retry is scheduled: temporary api failure",
		RetryCount:         2,
		NextRetryAt:        &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 5, 0, time.UTC)},
		CreatedAt:          &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC)},
		UpdatedAt:          &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 2, 0, time.UTC)},
	}
	client := newClient(t, readySessionResource(), action)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := getActionResource(t, ctx, client, "action-1")
	if got, want := updated.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhasePending; got != want {
		t.Fatalf("phase = %q, want %q", got, want)
	}
	if got, want := updated.Status.FailureClass, platformv1alpha1.AgentSessionActionResourceFailureClassTransient; got != want {
		t.Fatalf("failure_class = %q, want %q", got, want)
	}
	if got, want := updated.Status.RetryCount, int32(2); got != want {
		t.Fatalf("retry_count = %d, want %d", got, want)
	}
	if updated.Status.NextRetryAt == nil || !updated.Status.NextRetryAt.Equal(action.Status.NextRetryAt) {
		t.Fatalf("next_retry_at = %v, want %v", updated.Status.NextRetryAt, action.Status.NextRetryAt)
	}
}

func TestReconcilerCancelsStoppedPendingAction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	action := pendingActionResource("action-1", time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC))
	action.Spec.Action.StopRequested = true
	client := newClient(t, readySessionResource(), action)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := getActionResource(t, ctx, client, "action-1")
	if got, want := updated.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhaseCanceled; got != want {
		t.Fatalf("phase = %q, want %q", got, want)
	}
}

func TestReconcilerKeepsRunningActionWhenStopRequested(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	action := pendingActionResource("action-1", time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC))
	action.Spec.Action.StopRequested = true
	action.Status = platformv1alpha1.AgentSessionActionResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{ObservedGeneration: 1},
		Phase:              platformv1alpha1.AgentSessionActionResourcePhaseRunning,
		RunID:              "action-1",
		CreatedAt:          &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC)},
		UpdatedAt:          &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 2, 0, time.UTC)},
	}
	run := getTerminalRun("action-1")
	run.Status.Phase = platformv1alpha1.AgentRunResourcePhaseRunning
	run.Status.Message = "still running"
	client := newClient(t, readySessionResource(), action, run)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := getActionResource(t, ctx, client, "action-1")
	if got, want := updated.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhaseRunning; got != want {
		t.Fatalf("phase = %q, want %q", got, want)
	}
	if got := updated.Status.Message; got == "" || got == "still running" {
		t.Fatalf("message = %q, want stop-requested running message", got)
	}
	runResource := getRunResource(t, ctx, client, "action-1")
	if !runResource.Spec.Run.GetCancelRequested() {
		t.Fatal("run cancel_requested = false, want true")
	}
}

func TestReconcilerCancelsStoppedActionAfterRunFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	action := pendingActionResource("action-1", time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC))
	action.Spec.Action.StopRequested = true
	action.Status = platformv1alpha1.AgentSessionActionResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{ObservedGeneration: 1},
		Phase:              platformv1alpha1.AgentSessionActionResourcePhaseRunning,
		RunID:              "action-1",
		CreatedAt:          &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC)},
		UpdatedAt:          &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 2, 0, time.UTC)},
	}
	run := getFailedRun("action-1", true)
	client := newClient(t, readySessionResource(), action, run)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := getActionResource(t, ctx, client, "action-1")
	if got, want := updated.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhaseCanceled; got != want {
		t.Fatalf("phase = %q, want %q", got, want)
	}
}

func TestReconcilerAppliesSkillReload(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	session := readySessionResource()
	session.Generation = 9
	session.Status.Phase = platformv1alpha1.AgentSessionResourcePhasePending
	session.Status.ResourceConfigGeneration = 4
	session.Status.Conditions[len(session.Status.Conditions)-2].Status = metav1.ConditionFalse
	session.Status.Conditions[len(session.Status.Conditions)-1].Status = metav1.ConditionFalse
	session.Spec.Session.ResourceConfig = resourceConfigWithSubjects("resources-v2")
	session.Spec.Session.ResourceConfig.Instructions[1].Content = "typescript"
	desired := agentresourceconfig.DesiredRevisions(session.Spec.Session.GetResourceConfig())
	session.Status.RealizedRuleRevision = desired.Rule
	session.Status.RealizedMCPRevision = desired.MCP
	action := pendingReloadSubjectAction(
		"action-1",
		9,
		agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_SKILL,
		session.Spec.Session.GetResourceConfig(),
		time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC),
	)
	client := newClient(t, session, action)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}

	updatedAction := getActionResource(t, ctx, client, "action-1")
	if got, want := updatedAction.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhaseSucceeded; got != want {
		t.Fatalf("phase = %q, want %q", got, want)
	}
	updatedSession := &platformv1alpha1.AgentSessionResource{}
	if err := client.Get(ctx, ctrlclient.ObjectKey{Namespace: "code-code", Name: "session-1"}, updatedSession); err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	if got, want := updatedSession.Status.RealizedSkillRevision, desired.Skill; got != want {
		t.Fatalf("realized skill revision = %q, want %q", got, want)
	}
	if got, want := updatedSession.Status.RealizedRuleRevision, desired.Rule; got != want {
		t.Fatalf("realized rule revision = %q, want %q", got, want)
	}
	if got, want := updatedSession.Status.RealizedMCPRevision, desired.MCP; got != want {
		t.Fatalf("realized mcp revision = %q, want %q", got, want)
	}
}

func TestReconcilerAppliesForceResourceConfigReload(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	session := readySessionResource()
	session.Generation = 9
	session.Status.Phase = platformv1alpha1.AgentSessionResourcePhasePending
	session.Status.RealizedRuleRevision = ""
	session.Status.RealizedSkillRevision = ""
	session.Status.RealizedMCPRevision = ""
	session.Status.Conditions[len(session.Status.Conditions)-2].Status = metav1.ConditionFalse
	session.Status.Conditions[len(session.Status.Conditions)-1].Status = metav1.ConditionFalse
	session.Spec.Session.ResourceConfig = resourceConfigWithSubjects("resources-v2")
	desired := agentresourceconfig.DesiredRevisions(session.Spec.Session.GetResourceConfig())
	action := pendingReloadSubjectAction(
		"action-1",
		9,
		agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_RESOURCE_CONFIG,
		session.Spec.Session.GetResourceConfig(),
		time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC),
	)
	client := newClient(t, session, action)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}

	updatedSession := &platformv1alpha1.AgentSessionResource{}
	if err := client.Get(ctx, ctrlclient.ObjectKey{Namespace: "code-code", Name: "session-1"}, updatedSession); err != nil {
		t.Fatalf("Get(session) error = %v", err)
	}
	if got, want := updatedSession.Status.RealizedRuleRevision, desired.Rule; got != want {
		t.Fatalf("realized rule revision = %q, want %q", got, want)
	}
	if got, want := updatedSession.Status.RealizedSkillRevision, desired.Skill; got != want {
		t.Fatalf("realized skill revision = %q, want %q", got, want)
	}
	if got, want := updatedSession.Status.RealizedMCPRevision, desired.MCP; got != want {
		t.Fatalf("realized mcp revision = %q, want %q", got, want)
	}
}

func TestReconcilerCancelsSupersededSkillReload(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	session := readySessionResource()
	session.Generation = 10
	session.Spec.Session.ResourceConfig = resourceConfigWithSubjects("resources-v3")
	action := pendingReloadSubjectAction(
		"action-1",
		9,
		agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_SKILL,
		resourceConfigWithSubjects("resources-v2"),
		time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC),
	)
	client := newClient(t, session, action)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor("action-1")); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := getActionResource(t, ctx, client, "action-1")
	if got, want := updated.Status.Phase, platformv1alpha1.AgentSessionActionResourcePhaseCanceled; got != want {
		t.Fatalf("phase = %q, want %q", got, want)
	}
}

func getTerminalRun(runID string) *platformv1alpha1.AgentRunResource {
	return &platformv1alpha1.AgentRunResource{
		TypeMeta:   metav1.TypeMeta{APIVersion: platformv1alpha1.GroupVersion.String(), Kind: platformv1alpha1.KindAgentRunResource},
		ObjectMeta: metav1.ObjectMeta{Name: runID, Namespace: "code-code"},
		Spec: platformv1alpha1.AgentRunResourceSpec{
			Run: &agentrunv1.AgentRunSpec{
				RunId:                    runID,
				SessionId:                "session-1",
				Request:                  testRunRequest(runID, "hello", "gpt-5"),
				ProviderId:               "codex",
				ExecutionClass:           "default",
				ContainerImage:           "ghcr.io/openai/codex:latest",
				CpuRequest:               "1000m",
				MemoryRequest:            "2Gi",
				SessionGeneration:        1,
				RuntimeConfigGeneration:  1,
				ResourceConfigGeneration: 1,
				StateGeneration:          1,
				AuthRequirement: &agentrunv1.AgentRunAuthRequirement{
					ProviderId:               "codex",
					ProviderSurfaceBindingId: "openai-default",
					AuthStatus:               "bound",
				},
			},
		},
		Status: platformv1alpha1.AgentRunResourceStatus{
			CommonStatusFields: platformv1alpha1.CommonStatusFields{
				ObservedGeneration: 1,
				Conditions: []metav1.Condition{
					{
						Type:   string(platformcontract.AgentRunConditionTypeAccepted),
						Status: metav1.ConditionTrue,
					},
				},
			},
			Phase:     platformv1alpha1.AgentRunResourcePhaseSucceeded,
			Message:   "done",
			UpdatedAt: &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 3, 0, time.UTC)},
		},
	}
}

func getFailedRun(runID string, retryable bool) *platformv1alpha1.AgentRunResource {
	run := getTerminalRun(runID)
	run.Status.Phase = platformv1alpha1.AgentRunResourcePhaseFailed
	run.Status.Message = "run failed"
	run.Status.ResultSummary = &platformv1alpha1.AgentRunResultSummary{
		Status:       "RUN_STATUS_FAILED",
		ErrorCode:    "upstream_unavailable",
		ErrorMessage: "temporary upstream failure",
		Retryable:    retryable,
	}
	return run
}
