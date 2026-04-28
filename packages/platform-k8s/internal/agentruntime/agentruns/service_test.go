package agentruns

import (
	"context"
	"testing"

	capv1 "code-code.internal/go-contract/agent/cap/v1"
	agentcorev1 "code-code.internal/go-contract/agent/core/v1"
	inputv1 "code-code.internal/go-contract/agent/input/v1"
	resultv1 "code-code.internal/go-contract/agent/result/v1"
	credentialv1 "code-code.internal/go-contract/credential/v1"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/timeline"
	k8scorev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCreateRunFreezesSessionGenerationsAndImage(t *testing.T) {
	t.Parallel()

	service := newTestService(t,
		testCredentialDefinitionResource(),
		testReadySessionResource(),
	)

	state, err := service.Create(context.Background(), "session-1", &CreateRequest{Snapshot: testRunTurnSnapshot("run-ignored", "hello", "gpt-5")})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got, want := state.GetSpec().GetSessionGeneration(), int64(7); got != want {
		t.Fatalf("session_generation = %d, want %d", got, want)
	}
	if got, want := state.GetSpec().GetProviderId(), "codex"; got != want {
		t.Fatalf("provider_id = %q, want %q", got, want)
	}
	if got, want := state.GetSpec().GetContainerImage(), "ghcr.io/openai/codex:latest"; got != want {
		t.Fatalf("container_image = %q, want %q", got, want)
	}
	if got := state.GetSpec().GetRequest().GetRunId(); got == "" {
		t.Fatal("request.run_id = empty, want generated run id")
	}
	if got, want := state.GetSpec().GetRequest().GetInput().GetText(), "hello"; got != want {
		t.Fatalf("input.text = %q, want %q", got, want)
	}
	if got, want := state.GetSpec().GetRequest().GetInput().GetParameters().GetFields()["model"].GetStringValue(), "gpt-5"; got != want {
		t.Fatalf("input.parameters.model = %q, want %q", got, want)
	}
	if got, want := state.GetSpec().GetAuthRequirement().GetProviderSurfaceBindingId(), "openai-default"; got != want {
		t.Fatalf("auth_requirement.provider_surface_binding_id = %q, want %q", got, want)
	}
	if got, want := state.GetSpec().GetAuthRequirement().GetAuthStatus(), "bound"; got != want {
		t.Fatalf("auth_requirement.auth_status = %q, want %q", got, want)
	}
	if got, want := state.GetSpec().GetWorkspaceId(), "workspace-1"; got != want {
		t.Fatalf("workspace_id = %q, want %q", got, want)
	}
	if got, want := state.GetSpec().GetHomeStateId(), "home-1"; got != want {
		t.Fatalf("home_state_id = %q, want %q", got, want)
	}
	if got, want := state.GetSpec().GetRuntimeEnvironment().GetWorkspaceDir(), "/workspace"; got != want {
		t.Fatalf("runtime_environment.workspace_dir = %q, want %q", got, want)
	}
	if got, want := state.GetSpec().GetRuntimeEnvironment().GetDataDir(), "/home/agent"; got != want {
		t.Fatalf("runtime_environment.data_dir = %q, want %q", got, want)
	}
}

func TestCreateRunRejectsSessionWithoutDispatchReadiness(t *testing.T) {
	t.Parallel()

	session := testReadySessionResource()
	session.Status.Phase = platformv1alpha1.AgentSessionResourcePhasePending
	session.Status.Conditions[0].Status = metav1.ConditionFalse
	service := newTestService(t, session)

	if _, err := service.Create(context.Background(), "session-1", &CreateRequest{Snapshot: testRunTurnSnapshot("run-ignored", "hello", "")}); err == nil {
		t.Fatal("Create() error = nil, want session dispatch gate error")
	}
}

func TestCreateRunUsesFrozenSnapshotWhenReadyForNextRunIsFalse(t *testing.T) {
	t.Parallel()

	session := testReadySessionResource()
	session.Status.Phase = platformv1alpha1.AgentSessionResourcePhasePending
	session.Status.Conditions[len(session.Status.Conditions)-1].Status = metav1.ConditionFalse
	service := newTestService(t, session)

	state, err := service.Create(context.Background(), "session-1", &CreateRequest{Snapshot: testRunTurnSnapshot("run-ignored", "hello", "")})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got, want := state.GetSpec().GetContainerImage(), "ghcr.io/openai/codex:latest"; got != want {
		t.Fatalf("container_image = %q, want %q", got, want)
	}
}

func TestCreateRunRecordsSubmittedTimelineEvent(t *testing.T) {
	t.Parallel()

	sink := &fakeTimelineSink{}
	service := newTestServiceWithSink(t, sink,
		testCredentialDefinitionResource(),
		testReadySessionResource(),
	)

	if _, err := service.Create(context.Background(), "session-1", &CreateRequest{Snapshot: testRunTurnSnapshot("run-ignored", "hello", "")}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("events = %d, want 1", len(sink.events))
	}
	if sink.events[0].EventType != "SUBMITTED" {
		t.Fatalf("event type = %q, want SUBMITTED", sink.events[0].EventType)
	}
}

func TestPublishTerminalResultStoresResult(t *testing.T) {
	t.Parallel()

	sink := &fakeTimelineSink{}
	service := newTestServiceWithSink(t, sink,
		testCredentialDefinitionResource(),
		testReadySessionResource(),
	)
	state, err := service.Create(context.Background(), "session-1", &CreateRequest{RunID: "run-1", Snapshot: testRunTurnSnapshot("run-1", "hello", "")})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := service.PublishTerminalResult(context.Background(), state.GetSpec().GetRunId(), &resultv1.RunResult{
		Status: resultv1.RunStatus_RUN_STATUS_COMPLETED,
	}); err != nil {
		t.Fatalf("PublishTerminalResult() error = %v", err)
	}
	stored, err := service.Get(context.Background(), state.GetSpec().GetRunId())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.GetStatus().GetResult().GetStatus() != resultv1.RunStatus_RUN_STATUS_COMPLETED {
		t.Fatalf("result status = %v, want completed", stored.GetStatus().GetResult().GetStatus())
	}
	if len(sink.events) != 2 {
		t.Fatalf("events = %d, want 2", len(sink.events))
	}
	if sink.events[1].EventType != "RESULT_RECORDED" {
		t.Fatalf("second event type = %q, want RESULT_RECORDED", sink.events[1].EventType)
	}
}

func TestCancelMarksRunCancelRequested(t *testing.T) {
	t.Parallel()

	service := newTestService(t,
		testCredentialDefinitionResource(),
		testReadySessionResource(),
	)
	state, err := service.Create(context.Background(), "session-1", &CreateRequest{RunID: "run-1", Snapshot: testRunTurnSnapshot("run-1", "hello", "")})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := service.Cancel(context.Background(), state.GetSpec().GetRunId())
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if !updated.GetSpec().GetCancelRequested() {
		t.Fatal("cancel_requested = false, want true")
	}
}

func TestCancelRecordsStopRequestedTimelineEvent(t *testing.T) {
	t.Parallel()

	sink := &fakeTimelineSink{}
	service := newTestServiceWithSink(t, sink,
		testCredentialDefinitionResource(),
		testReadySessionResource(),
	)
	if _, err := service.Create(context.Background(), "session-1", &CreateRequest{RunID: "run-1", Snapshot: testRunTurnSnapshot("run-1", "hello", "")}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, err := service.Cancel(context.Background(), "run-1"); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if len(sink.events) != 2 {
		t.Fatalf("events = %d, want 2", len(sink.events))
	}
	if got, want := sink.events[1].EventType, "STOP_REQUESTED"; got != want {
		t.Fatalf("second event type = %q, want %q", got, want)
	}
}

func newTestService(t *testing.T, objects ...runtime.Object) *Service {
	t.Helper()
	return newTestServiceWithSink(t, nil, objects...)
}

func newTestServiceWithSink(t *testing.T, sink timeline.Sink, objects ...runtime.Object) *Service {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := platformv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	if err := k8scorev1.AddToScheme(scheme); err != nil {
		t.Fatalf("corev1.AddToScheme() error = %v", err)
	}
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objects...).
		WithStatusSubresource(&platformv1alpha1.AgentSessionResource{}, &platformv1alpha1.AgentRunResource{}).
		Build()
	service, err := NewService(client, client, "code-code", sink, WithActiveRunSlots(newFakeActiveRunSlots(client)))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func testReadySessionResource() *platformv1alpha1.AgentSessionResource {
	return &platformv1alpha1.AgentSessionResource{
		TypeMeta: metav1.TypeMeta{APIVersion: platformv1alpha1.GroupVersion.String(), Kind: platformv1alpha1.KindAgentSessionResource},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "session-1",
			Namespace:  "code-code",
			Generation: 7,
		},
		Spec: platformv1alpha1.AgentSessionResourceSpec{
			Session: &agentsessionv1.AgentSessionSpec{
				SessionId:      "session-1",
				ProviderId:     "codex",
				ExecutionClass: "default",
				RuntimeConfig: &agentsessionv1.AgentSessionRuntimeConfig{
					ProviderRuntimeRef: &providerv1.ProviderRuntimeRef{SurfaceId: "openai-default"},
				},
				ResourceConfig: &capv1.AgentResources{SnapshotId: "resources-v1"},
				WorkspaceRef:   &agentsessionv1.AgentSessionWorkspaceRef{WorkspaceId: "workspace-1"},
				HomeStateRef:   &agentsessionv1.AgentSessionHomeStateRef{HomeStateId: "home-1"},
			},
		},
		Status: platformv1alpha1.AgentSessionResourceStatus{
			CommonStatusFields: platformv1alpha1.CommonStatusFields{
				Conditions: readySessionConditions(),
			},
			Phase:                    platformv1alpha1.AgentSessionResourcePhaseReady,
			RuntimeConfigGeneration:  3,
			ResourceConfigGeneration: 4,
			StateGeneration:          5,
		},
	}
}

func testCredentialDefinitionResource() *platformv1alpha1.CredentialDefinitionResource {
	return &platformv1alpha1.CredentialDefinitionResource{
		TypeMeta:   metav1.TypeMeta{APIVersion: platformv1alpha1.GroupVersion.String(), Kind: platformv1alpha1.KindCredentialDefinitionResource},
		ObjectMeta: metav1.ObjectMeta{Name: "credential-openai", Namespace: "code-code"},
		Spec: platformv1alpha1.CredentialDefinitionResourceSpec{
			Definition: &credentialv1.CredentialDefinition{
				CredentialId: "credential-openai",
				Kind:         credentialv1.CredentialKind_CREDENTIAL_KIND_API_KEY,
			},
		},
	}
}

func testRunRequest(runID string, prompt string, model string) *agentcorev1.RunRequest {
	return &agentcorev1.RunRequest{
		RunId: runID,
		Input: &inputv1.RunInput{
			Text:       prompt,
			Parameters: mustStruct(map[string]any{"model": model}),
		},
	}
}

func testRunTurnSnapshot(runID string, prompt string, model string) *agentsessionactionv1.AgentSessionRunTurnSnapshot {
	return &agentsessionactionv1.AgentSessionRunTurnSnapshot{
		RunRequest:               testRunRequest(runID, prompt, model),
		SessionGeneration:        7,
		RuntimeConfigGeneration:  3,
		ResourceConfigGeneration: 4,
		StateGeneration:          5,
		ProviderId:               "codex",
		ExecutionClass:           "default",
		ContainerImage:           "ghcr.io/openai/codex:latest",
		CpuRequest:               "1000m",
		MemoryRequest:            "2Gi",
		AuthRequirement: &agentrunv1.AgentRunAuthRequirement{
			ProviderId:               "codex",
			ProviderSurfaceBindingId: "openai-default",
			AuthStatus:               "bound",
		},
		RuntimeEnvironment: &agentcorev1.RuntimeEnvironment{
			WorkspaceDir: "/workspace",
			DataDir:      "/home/agent",
		},
		WorkspaceId: "workspace-1",
		HomeStateId: "home-1",
	}
}

func readySessionConditions() []metav1.Condition {
	return []metav1.Condition{
		{
			Type:   string(platformcontract.AgentSessionConditionTypeWorkspaceReady),
			Status: metav1.ConditionTrue,
		},
		{
			Type:   string(platformcontract.AgentSessionConditionTypeWarmStateReady),
			Status: metav1.ConditionTrue,
		},
		{
			Type:   string(platformcontract.AgentSessionConditionTypeRuntimeConfigReady),
			Status: metav1.ConditionTrue,
		},
		{
			Type:   string(platformcontract.AgentSessionConditionTypeResourceConfigReady),
			Status: metav1.ConditionTrue,
		},
		{
			Type:   string(platformcontract.AgentSessionConditionTypeReadyForNextRun),
			Status: metav1.ConditionTrue,
		},
	}
}
