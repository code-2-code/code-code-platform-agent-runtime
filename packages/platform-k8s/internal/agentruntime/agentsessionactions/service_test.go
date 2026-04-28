package agentsessionactions

import (
	"context"
	"strings"
	"testing"
	"time"

	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateActionFreezesRunTurnSnapshot(t *testing.T) {
	t.Parallel()

	service := newTestService(t, readySessionResource())
	state, err := service.Create(context.Background(), "session-1", &CreateRequest{
		ActionID:   "action-1",
		TurnID:     "turn-1",
		RunRequest: testRunRequest("ignored", "hello", "gpt-5"),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got, want := state.GetSpec().GetActionId(), "action-1"; got != want {
		t.Fatalf("action_id = %q, want %q", got, want)
	}
	if got, want := state.GetSpec().GetSessionId(), "session-1"; got != want {
		t.Fatalf("session_id = %q, want %q", got, want)
	}
	snapshot := state.GetSpec().GetInputSnapshot().GetRunTurn()
	if snapshot == nil {
		t.Fatal("run_turn = nil, want frozen snapshot")
	}
	if got, want := snapshot.GetRunRequest().GetInput().GetText(), "hello"; got != want {
		t.Fatalf("input.text = %q, want %q", got, want)
	}
	if got, want := snapshot.GetRunRequest().GetInput().GetParameters().GetFields()["model"].GetStringValue(), "gpt-5"; got != want {
		t.Fatalf("input.parameters.model = %q, want %q", got, want)
	}
	if got, want := snapshot.GetSessionGeneration(), int64(7); got != want {
		t.Fatalf("session_generation = %d, want %d", got, want)
	}
	if got, want := snapshot.GetContainerImage(), "ghcr.io/openai/codex:latest"; got != want {
		t.Fatalf("container_image = %q, want %q", got, want)
	}
	if got, want := snapshot.GetAuthRequirement().GetProviderSurfaceBindingId(), "openai-default"; got != want {
		t.Fatalf("auth_requirement.provider_surface_binding_id = %q, want %q", got, want)
	}
	if got, want := snapshot.GetRunRequest().GetResolvedProviderModel().GetProviderModelId(), "gpt-5"; got != want {
		t.Fatalf("resolved_provider_model.provider_model_id = %q, want %q", got, want)
	}
	if got, want := len(snapshot.GetRuntimeCandidates()), 1; got != want {
		t.Fatalf("runtime_candidates len = %d, want %d", got, want)
	}
	if got, want := snapshot.GetWorkspaceId(), "workspace-1"; got != want {
		t.Fatalf("workspace_id = %q, want %q", got, want)
	}
	if got, want := snapshot.GetHomeStateId(), "home-1"; got != want {
		t.Fatalf("home_state_id = %q, want %q", got, want)
	}
	if got, want := snapshot.GetRuntimeEnvironment().GetWorkspaceDir(), "/workspace"; got != want {
		t.Fatalf("runtime_environment.workspace_dir = %q, want %q", got, want)
	}
	if got, want := snapshot.GetRuntimeEnvironment().GetDataDir(), "/home/agent"; got != want {
		t.Fatalf("runtime_environment.data_dir = %q, want %q", got, want)
	}
	if got, want := len(snapshot.GetPrepareJobs()), 1; got != want {
		t.Fatalf("prepare_jobs len = %d, want %d", got, want)
	}
	if got, want := snapshot.GetPrepareJobs()[0].GetJobType(), "auth"; got != want {
		t.Fatalf("prepare_jobs[0].job_type = %q, want %q", got, want)
	}
}

func TestCreateActionFreezesRuntimeFallbackCandidates(t *testing.T) {
	t.Parallel()

	session := readySessionResource()
	session.Spec.Session.RuntimeConfig.Fallbacks = []*agentsessionv1.AgentSessionRuntimeFallbackCandidate{{
		ProviderRuntimeRef: &providerv1.ProviderRuntimeRef{SurfaceId: "openai-backup"},
		ModelSelector: &agentsessionv1.AgentSessionRuntimeFallbackCandidate_ProviderModelId{
			ProviderModelId: "gpt-4.1-mini",
		},
	}}
	service := newTestService(t, session)

	state, err := service.Create(context.Background(), "session-1", &CreateRequest{
		ActionID:   "action-1",
		TurnID:     "turn-1",
		RunRequest: testRunRequest("ignored", "hello", "gpt-5"),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	snapshot := state.GetSpec().GetInputSnapshot().GetRunTurn()
	if got, want := len(snapshot.GetRuntimeCandidates()), 2; got != want {
		t.Fatalf("runtime_candidates len = %d, want %d", got, want)
	}
	if got, want := snapshot.GetRuntimeCandidates()[1].GetResolvedProviderModel().GetSurfaceId(), "openai-backup"; got != want {
		t.Fatalf("fallback surface_id = %q, want %q", got, want)
	}
	if got, want := snapshot.GetRuntimeCandidates()[1].GetResolvedProviderModel().GetProviderModelId(), "gpt-4.1-mini"; got != want {
		t.Fatalf("fallback provider_model_id = %q, want %q", got, want)
	}
	selected, err := snapshotForCandidate(snapshot, 1, "action-1-attempt-2")
	if err != nil {
		t.Fatalf("snapshotForCandidate() error = %v", err)
	}
	if !strings.Contains(selected.GetPrepareJobs()[0].GetParametersYaml(), "https://backup.api.openai.com/v1") {
		t.Fatalf("fallback auth prepare yaml = %q, want fallback runtime URL", selected.GetPrepareJobs()[0].GetParametersYaml())
	}
}

func TestCreateActionFreezesSessionPrepareJobs(t *testing.T) {
	t.Parallel()

	session := readySessionResource()
	session.Spec.Session.PrepareJobs = []*agentsessionv1.AgentSessionPrepareJob{
		{
			JobId:          "rules",
			CliId:          "codex",
			JobType:        "rules",
			RunType:        agentrunv1.AgentRunPrepareJobRunType_AGENT_RUN_PREPARE_JOB_RUN_TYPE_ON_CHANGED,
			ChangeKey:      "rules-v1",
			ParametersYaml: "path: rules.yaml\n",
		},
		{
			JobId:   "auth",
			JobType: "auth",
			Cleanup: true,
		},
	}
	service := newTestService(t, session)

	state, err := service.Create(context.Background(), "session-1", &CreateRequest{
		ActionID:   "action-1",
		TurnID:     "turn-1",
		RunRequest: testRunRequest("ignored", "hello", "gpt-5"),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	jobs := state.GetSpec().GetInputSnapshot().GetRunTurn().GetPrepareJobs()
	if got, want := len(jobs), 2; got != want {
		t.Fatalf("prepare_jobs len = %d, want %d", got, want)
	}
	if got, want := jobs[0].GetJobType(), "rules"; got != want {
		t.Fatalf("prepare_jobs[0].job_type = %q, want %q", got, want)
	}
	if got, want := jobs[0].GetRunType(), agentrunv1.AgentRunPrepareJobRunType_AGENT_RUN_PREPARE_JOB_RUN_TYPE_ON_CHANGED; got != want {
		t.Fatalf("prepare_jobs[0].run_type = %s, want %s", got, want)
	}
	if got, want := jobs[0].GetParametersYaml(), "path: rules.yaml\n"; got != want {
		t.Fatalf("prepare_jobs[0].parameters_yaml = %q, want %q", got, want)
	}
	if got, want := jobs[1].GetJobType(), "auth"; got != want {
		t.Fatalf("prepare_jobs[1].job_type = %q, want %q", got, want)
	}
	if !strings.Contains(jobs[1].GetParametersYaml(), "authMaterializationKey") {
		t.Fatalf("prepare_jobs[1].parameters_yaml = %q, want auth materialization data", jobs[1].GetParametersYaml())
	}
}

func TestCreateActionRejectsMissingSession(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	if _, err := service.Create(context.Background(), "session-1", &CreateRequest{
		RunRequest: testRunRequest("ignored", "hello", ""),
	}); err == nil {
		t.Fatal("Create() error = nil, want missing session error")
	}
}

func TestCreateActionRejectsSessionNotReadyForNextRun(t *testing.T) {
	t.Parallel()

	session := readySessionResource()
	session.Status.Conditions[len(session.Status.Conditions)-1].Status = metav1.ConditionFalse
	service := newTestService(t, session)
	if _, err := service.Create(context.Background(), "session-1", &CreateRequest{
		RunRequest: testRunRequest("ignored", "hello", ""),
	}); err == nil {
		t.Fatal("Create() error = nil, want readiness error")
	}
}

func TestCreateActionRejectsPendingWarmStateReset(t *testing.T) {
	t.Parallel()

	service := newTestService(t,
		readySessionResource(),
		pendingResetWarmStateAction("reset-1", "session-1", "home-1", "home-2"),
	)
	if _, err := service.Create(context.Background(), "session-1", &CreateRequest{
		RunRequest: testRunRequest("ignored", "hello", ""),
	}); err == nil {
		t.Fatal("Create() error = nil, want pending reset rejection")
	}
}

func TestStopMarksActionStopRequested(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	action := pendingActionResource("action-1", time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC))
	service := newTestService(t, readySessionResource(), action)

	state, err := service.Stop(ctx, "action-1")
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !state.GetSpec().GetStopRequested() {
		t.Fatal("stop_requested = false, want true")
	}
	resource, err := service.store.Get(ctx, "action-1")
	if err != nil {
		t.Fatalf("Get(action) error = %v", err)
	}
	if !resource.Spec.Action.GetStopRequested() {
		t.Fatal("resource stop_requested = false, want true")
	}
}

func TestRetryCreatesNewRunTurnActionFromSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	source := pendingActionResource("action-1", time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC))
	source.Status = platformv1alpha1.AgentSessionActionResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{ObservedGeneration: 1},
		Phase:              platformv1alpha1.AgentSessionActionResourcePhaseFailed,
		FailureClass:       platformv1alpha1.AgentSessionActionResourceFailureClassManualRetry,
		CreatedAt:          &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC)},
		UpdatedAt:          &metav1.Time{Time: time.Date(2026, 4, 17, 16, 0, 2, 0, time.UTC)},
	}
	service := newTestService(t, readySessionResource(), source)

	state, err := service.Retry(ctx, "action-1", &RetryRequest{TurnID: "action-2"})
	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if got, want := state.GetSpec().GetActionId(), "action-2"; got != want {
		t.Fatalf("action_id = %q, want %q", got, want)
	}
	snapshot := state.GetSpec().GetInputSnapshot().GetRunTurn()
	if snapshot == nil {
		t.Fatal("run_turn = nil, want frozen retry snapshot")
	}
	if got, want := snapshot.GetRunRequest().GetInput().GetText(), "hello"; got != want {
		t.Fatalf("input.text = %q, want %q", got, want)
	}
}

func TestQueueOwnerUsesOldestPendingAction(t *testing.T) {
	t.Parallel()

	client := newClient(t,
		pendingActionResource("action-2", time.Date(2026, 4, 17, 16, 0, 2, 0, time.UTC)),
		pendingActionResource("action-1", time.Date(2026, 4, 17, 16, 0, 1, 0, time.UTC)),
	)
	items, err := listSessionActions(context.Background(), newFakeActionStore(client), "session-1")
	if err != nil {
		t.Fatalf("listSessionActions() error = %v", err)
	}
	if got, want := queueOwnerID(items), "action-1"; got != want {
		t.Fatalf("queue owner = %q, want %q", got, want)
	}
}
