package agentsessionactions

import (
	"context"
	"strings"
	"testing"
	"time"

	capv1 "code-code.internal/go-contract/agent/cap/v1"
	agentcorev1 "code-code.internal/go-contract/agent/core/v1"
	inputv1 "code-code.internal/go-contract/agent/input/v1"
	apiprotocolv1 "code-code.internal/go-contract/api_protocol/v1"
	credentialv1 "code-code.internal/go-contract/credential/v1"
	modelv1 "code-code.internal/go-contract/model/v1"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentresourceconfig"
	"code-code.internal/platform-k8s/internal/agentruntime/agentruns"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessions"
	"code-code.internal/platform-k8s/internal/platform/testutil"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestService(t *testing.T, objects ...runtime.Object) *Service {
	t.Helper()
	client := newClient(t, objects...)
	store := newFakeActionStore(client)
	sessions := fakeActionSessionRepository{client: client, namespace: "code-code"}
	service, err := NewService(store, sessions, "code-code", nil, newTestExecutionResolver(t))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func newTestReconciler(t *testing.T, client ctrlclient.Client) *Reconciler {
	t.Helper()
	store := newFakeActionStore(client)
	sessions := fakeActionSessionRepository{client: client, namespace: "code-code"}
	slots, err := agentsessions.NewActiveRunManager(sessions, "code-code")
	if err != nil {
		t.Fatalf("NewActiveRunManager() error = %v", err)
	}
	runs, err := agentruns.NewService(client, client, "code-code", nil, agentruns.WithActiveRunSlots(slots))
	if err != nil {
		t.Fatalf("NewRunService() error = %v", err)
	}
	reconciler, err := NewReconciler(ReconcilerConfig{
		Client:    client,
		Namespace: "code-code",
		Now: func() time.Time {
			return time.Date(2026, 4, 17, 16, 0, 0, 0, time.UTC)
		},
		Runs:     runs,
		Store:    store,
		Sessions: sessions,
	})
	if err != nil {
		t.Fatalf("NewReconciler() error = %v", err)
	}
	return reconciler
}

type fakeActionStore struct {
	client    ctrlclient.Client
	namespace string
}

func newFakeActionStore(client ctrlclient.Client) *fakeActionStore {
	return &fakeActionStore{client: client, namespace: "code-code"}
}

func (s *fakeActionStore) Get(ctx context.Context, actionID string) (*platformv1alpha1.AgentSessionActionResource, error) {
	resource := &platformv1alpha1.AgentSessionActionResource{}
	if err := s.client.Get(ctx, types.NamespacedName{Namespace: s.namespace, Name: strings.TrimSpace(actionID)}, resource); err != nil {
		return nil, err
	}
	return resource, nil
}

func (s *fakeActionStore) Create(ctx context.Context, resource *platformv1alpha1.AgentSessionActionResource) error {
	if resource == nil {
		return apierrors.NewBadRequest("action resource is nil")
	}
	normalizeActionResource(resource, s.namespace, 1)
	resource.ResourceVersion = ""
	return s.client.Create(ctx, resource)
}

func (s *fakeActionStore) Update(ctx context.Context, actionID string, mutate func(*platformv1alpha1.AgentSessionActionResource) error) (*platformv1alpha1.AgentSessionActionResource, error) {
	resource, err := s.Get(ctx, actionID)
	if err != nil {
		return nil, err
	}
	if mutate != nil {
		if err := mutate(resource); err != nil {
			return nil, err
		}
	}
	if resource.Generation == 0 {
		resource.Generation = 1
	} else {
		resource.Generation++
	}
	if err := s.client.Update(ctx, resource); err != nil {
		return nil, err
	}
	return s.Get(ctx, actionID)
}

func (s *fakeActionStore) UpdateStatus(ctx context.Context, actionID string, next *platformv1alpha1.AgentSessionActionResourceStatus) (*platformv1alpha1.AgentSessionActionResource, error) {
	resource, err := s.Get(ctx, actionID)
	if err != nil {
		return nil, err
	}
	if next != nil {
		resource.Status = *next.DeepCopy()
	}
	if err := s.client.Status().Update(ctx, resource); err != nil {
		return nil, err
	}
	return s.Get(ctx, actionID)
}

func (s *fakeActionStore) ListBySession(ctx context.Context, sessionID string) ([]platformv1alpha1.AgentSessionActionResource, error) {
	list := &platformv1alpha1.AgentSessionActionResourceList{}
	if err := s.client.List(ctx, list, ctrlclient.InNamespace(s.namespace)); err != nil {
		return nil, err
	}
	out := make([]platformv1alpha1.AgentSessionActionResource, 0, len(list.Items))
	for i := range list.Items {
		action := list.Items[i].Spec.Action
		if action == nil || strings.TrimSpace(action.GetSessionId()) != strings.TrimSpace(sessionID) {
			continue
		}
		out = append(out, list.Items[i])
	}
	return out, nil
}

func (s *fakeActionStore) HasNonterminalResetWarmState(ctx context.Context, sessionID string) (bool, error) {
	items, err := s.ListBySession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	for i := range items {
		action := items[i].Spec.Action
		if action == nil || action.GetType() != agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RESET_WARM_STATE {
			continue
		}
		if !isTerminalPhase(items[i].Status.Phase) {
			return true, nil
		}
	}
	return false, nil
}

type fakeActionSessionRepository struct {
	client    ctrlclient.Client
	namespace string
}

func (r fakeActionSessionRepository) Get(ctx context.Context, sessionID string) (*agentsessionv1.AgentSessionState, error) {
	return r.load(ctx, sessionID)
}

func (r fakeActionSessionRepository) Update(ctx context.Context, sessionID string, spec *agentsessionv1.AgentSessionSpec) (*agentsessionv1.AgentSessionState, error) {
	resource := &platformv1alpha1.AgentSessionResource{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: strings.TrimSpace(sessionID)}, resource); err != nil {
		return nil, err
	}
	resource.Spec.Session = proto.Clone(spec).(*agentsessionv1.AgentSessionSpec)
	if resource.Generation == 0 {
		resource.Generation = 1
	} else {
		resource.Generation++
	}
	if err := r.client.Update(ctx, resource); err != nil {
		return nil, err
	}
	return r.load(ctx, sessionID)
}

func (r fakeActionSessionRepository) UpdateStatus(ctx context.Context, sessionID string, status *agentsessionv1.AgentSessionStatus) (*agentsessionv1.AgentSessionState, error) {
	resource := &platformv1alpha1.AgentSessionResource{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: strings.TrimSpace(sessionID)}, resource); err != nil {
		return nil, err
	}
	state, err := agentsessions.StateFromResource(resource)
	if err != nil {
		return nil, err
	}
	if status != nil {
		state.Status = proto.Clone(status).(*agentsessionv1.AgentSessionStatus)
	}
	next, err := agentsessions.ResourceFromState(state, r.namespace)
	if err != nil {
		return nil, err
	}
	resource.Status = next.Status
	if err := r.client.Status().Update(ctx, resource); err != nil {
		return nil, err
	}
	return r.load(ctx, sessionID)
}

func (r fakeActionSessionRepository) ClaimActiveRun(ctx context.Context, sessionID string, runID string) (*agentsessionv1.AgentSessionState, error) {
	resource := &platformv1alpha1.AgentSessionResource{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: strings.TrimSpace(sessionID)}, resource); err != nil {
		return nil, err
	}
	activeRunID := strings.TrimSpace(resource.Status.ActiveRunID)
	if activeRunID != "" && activeRunID != strings.TrimSpace(runID) {
		return nil, apierrors.NewConflict(schema.GroupResource{Group: platformv1alpha1.GroupName, Resource: "sessions"}, sessionID, nil)
	}
	if activeRunID == "" && !fakeActionSessionDispatchReady(resource) {
		return nil, apierrors.NewInvalid(schema.GroupKind{Group: platformv1alpha1.GroupName, Kind: platformv1alpha1.KindAgentSessionResource}, sessionID, nil)
	}
	resource.Status.ActiveRunID = strings.TrimSpace(runID)
	resource.Status.Phase = platformv1alpha1.AgentSessionResourcePhaseRunning
	if err := r.client.Status().Update(ctx, resource); err != nil {
		return nil, err
	}
	return r.load(ctx, sessionID)
}

func (r fakeActionSessionRepository) ReleaseActiveRun(ctx context.Context, sessionID string, runID string) (bool, error) {
	resource := &platformv1alpha1.AgentSessionResource{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: strings.TrimSpace(sessionID)}, resource); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if strings.TrimSpace(resource.Status.ActiveRunID) != strings.TrimSpace(runID) {
		return false, nil
	}
	resource.Status.ActiveRunID = ""
	resource.Status.Phase = platformv1alpha1.AgentSessionResourcePhasePending
	if err := r.client.Status().Update(ctx, resource); err != nil {
		return false, err
	}
	return true, nil
}

func fakeActionSessionDispatchReady(resource *platformv1alpha1.AgentSessionResource) bool {
	if resource == nil {
		return false
	}
	return fakeActionConditionTrue(resource.Status.Conditions, string(platformcontract.AgentSessionConditionTypeWorkspaceReady)) &&
		fakeActionConditionTrue(resource.Status.Conditions, string(platformcontract.AgentSessionConditionTypeWarmStateReady))
}

func fakeActionConditionTrue(conditions []metav1.Condition, conditionType string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType && condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r fakeActionSessionRepository) load(ctx context.Context, sessionID string) (*agentsessionv1.AgentSessionState, error) {
	resource := &platformv1alpha1.AgentSessionResource{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: strings.TrimSpace(sessionID)}, resource); err != nil {
		return nil, err
	}
	return agentsessions.StateFromResource(resource)
}

func newClient(t *testing.T, objects ...runtime.Object) ctrlclient.Client {
	t.Helper()
	scheme := testutil.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(corev1) error = %v", err)
	}
	clientObjects := append([]runtime.Object{
		testCredentialDefinitionResource(),
	}, objects...)
	return ctrlclientfake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(clientObjects...).
		WithStatusSubresource(
			&platformv1alpha1.AgentSessionResource{},
			&platformv1alpha1.AgentSessionActionResource{},
			&platformv1alpha1.AgentRunResource{},
		).
		Build()
}

func requestFor(name string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "code-code", Name: name}}
}

func getActionResource(t *testing.T, ctx context.Context, client ctrlclient.Client, name string) *platformv1alpha1.AgentSessionActionResource {
	t.Helper()
	resource := &platformv1alpha1.AgentSessionActionResource{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code", Name: name}, resource); err != nil {
		t.Fatalf("Get(action) error = %v", err)
	}
	return resource
}

func getRunResource(t *testing.T, ctx context.Context, client ctrlclient.Client, name string) *platformv1alpha1.AgentRunResource {
	t.Helper()
	resource := &platformv1alpha1.AgentRunResource{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code", Name: name}, resource); err != nil {
		t.Fatalf("Get(run) error = %v", err)
	}
	return resource
}

func readySessionResource() *platformv1alpha1.AgentSessionResource {
	resourceConfig := resourceConfigWithSubjects("resources-v1")
	desired := agentresourceconfig.DesiredRevisions(resourceConfig)
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
				ResourceConfig: resourceConfig,
				WorkspaceRef:   &agentsessionv1.AgentSessionWorkspaceRef{WorkspaceId: "workspace-1"},
				HomeStateRef:   &agentsessionv1.AgentSessionHomeStateRef{HomeStateId: "home-1"},
			},
		},
		Status: platformv1alpha1.AgentSessionResourceStatus{
			CommonStatusFields: platformv1alpha1.CommonStatusFields{
				Conditions: readySessionConditions(7),
			},
			Phase:                    platformv1alpha1.AgentSessionResourcePhaseReady,
			RuntimeConfigGeneration:  7,
			ResourceConfigGeneration: 7,
			RealizedRuleRevision:     desired.Rule,
			RealizedSkillRevision:    desired.Skill,
			RealizedMCPRevision:      desired.MCP,
			StateGeneration:          7,
		},
	}
}

func pendingActionResource(actionID string, createdAt time.Time) *platformv1alpha1.AgentSessionActionResource {
	return &platformv1alpha1.AgentSessionActionResource{
		TypeMeta: metav1.TypeMeta{APIVersion: platformv1alpha1.GroupVersion.String(), Kind: platformv1alpha1.KindAgentSessionActionResource},
		ObjectMeta: metav1.ObjectMeta{
			Name:              actionID,
			Namespace:         "code-code",
			CreationTimestamp: metav1.NewTime(createdAt),
			Labels:            actionLabels("session-1", agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RUN_TURN),
		},
		Spec: platformv1alpha1.AgentSessionActionResourceSpec{
			Action: &agentsessionactionv1.AgentSessionActionSpec{
				ActionId:  actionID,
				SessionId: "session-1",
				Type:      agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RUN_TURN,
				InputSnapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot{
					Snapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot_RunTurn{
						RunTurn: testRunTurnSnapshot(actionID, "hello", "gpt-5"),
					},
				},
			},
		},
	}
}

func pendingReloadSubjectAction(actionID string, generation int64, subject agentsessionactionv1.AgentSessionActionSubject, config *capv1.AgentResources, createdAt time.Time) *platformv1alpha1.AgentSessionActionResource {
	snapshot := agentresourceconfig.Snapshot(config, subject)
	return &platformv1alpha1.AgentSessionActionResource{
		TypeMeta: metav1.TypeMeta{APIVersion: platformv1alpha1.GroupVersion.String(), Kind: platformv1alpha1.KindAgentSessionActionResource},
		ObjectMeta: metav1.ObjectMeta{
			Name:              actionID,
			Namespace:         "code-code",
			CreationTimestamp: metav1.NewTime(createdAt),
			Labels:            actionLabels("session-1", agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RELOAD_SUBJECT),
		},
		Spec: platformv1alpha1.AgentSessionActionResourceSpec{
			Action: &agentsessionactionv1.AgentSessionActionSpec{
				ActionId:  actionID,
				SessionId: "session-1",
				Type:      agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RELOAD_SUBJECT,
				InputSnapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot{
					Snapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot_ReloadSubject{
						ReloadSubject: &agentsessionactionv1.AgentSessionReloadSubjectSnapshot{
							SessionGeneration: generation,
							Subject:           subject,
							SnapshotId:        config.GetSnapshotId(),
							SubjectRevision:   snapshot.SubjectRevision,
							ResourceConfig:    snapshot.ResourceConfig,
						},
					},
				},
			},
		},
	}
}

func resourceConfigWithSubjects(snapshotID string) *capv1.AgentResources {
	return &capv1.AgentResources{
		SnapshotId: snapshotID,
		Instructions: []*capv1.InstructionResource{
			{Kind: capv1.InstructionKind_INSTRUCTION_KIND_RULE, Name: "rule-1", Content: "do not leak secrets"},
			{Kind: capv1.InstructionKind_INSTRUCTION_KIND_SKILL, Name: "skill-1", Content: "python"},
		},
		ToolBindings: []*capv1.ToolBinding{
			{Name: "mcp-1", Kind: capv1.ToolKind_TOOL_KIND_MCP, Target: "mcp://server-1"},
		},
	}
}

func testRunRequest(runID string, prompt string, model string) *agentcorev1.RunRequest {
	parameters := (*structpb.Struct)(nil)
	if model != "" {
		parameters = mustStruct(map[string]any{"model": model})
	}
	return &agentcorev1.RunRequest{
		RunId: runID,
		Input: &inputv1.RunInput{
			Text:       prompt,
			Parameters: parameters,
		},
	}
}

func testRunTurnSnapshot(runID string, prompt string, model string) *agentsessionactionv1.AgentSessionRunTurnSnapshot {
	runRequest := testRunRequest(runID, prompt, model)
	runRequest.ResolvedProviderModel = testResolvedProviderModel("openai-default", "https://api.openai.com/v1", model)
	authRequirement := &agentrunv1.AgentRunAuthRequirement{
		ProviderId:               "codex",
		ProviderSurfaceBindingId: "openai-default",
		AuthStatus:               "bound",
		RuntimeUrl:               "https://api.openai.com/v1",
		MaterializationKey:       "codex.openai-api-key",
	}
	return &agentsessionactionv1.AgentSessionRunTurnSnapshot{
		RunRequest:               runRequest,
		SessionGeneration:        7,
		RuntimeConfigGeneration:  3,
		ResourceConfigGeneration: 4,
		StateGeneration:          5,
		ProviderId:               "codex",
		ExecutionClass:           "default",
		ContainerImage:           "ghcr.io/openai/codex:latest",
		CpuRequest:               "1000m",
		MemoryRequest:            "2Gi",
		AuthRequirement:          authRequirement,
		RuntimeCandidates: []*agentsessionactionv1.AgentSessionRuntimeCandidate{{
			ResolvedProviderModel: testResolvedProviderModel("openai-default", "https://api.openai.com/v1", model),
			AuthRequirement:       authRequirement,
		}},
		RuntimeEnvironment: &agentcorev1.RuntimeEnvironment{
			WorkspaceDir: "/workspace",
			DataDir:      "/home/agent",
		},
		WorkspaceId: "workspace-1",
		HomeStateId: "home-1",
	}
}

func readySessionConditions(generation int64) []metav1.Condition {
	return []metav1.Condition{
		{
			Type:               string(platformcontract.AgentSessionConditionTypeWorkspaceReady),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: generation,
		},
		{
			Type:               string(platformcontract.AgentSessionConditionTypeWarmStateReady),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: generation,
		},
		{
			Type:               string(platformcontract.AgentSessionConditionTypeRuntimeConfigReady),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: generation,
		},
		{
			Type:               string(platformcontract.AgentSessionConditionTypeResourceConfigReady),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: generation,
		},
		{
			Type:               string(platformcontract.AgentSessionConditionTypeReadyForNextRun),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: generation,
		},
	}
}

func mustStruct(value map[string]any) *structpb.Struct {
	out, err := structpb.NewStruct(value)
	if err != nil {
		panic(err)
	}
	return out
}

func testProviderSurfaceBindingProvider() *providerv1.Provider {
	return testProvider("provider-openai", testProviderSurfaceBinding("provider-openai", "openai-default", "https://api.openai.com/v1", "gpt-5"))
}

func testFallbackProviderSurfaceBindingProvider() *providerv1.Provider {
	return testProvider("provider-openai-backup", testProviderSurfaceBinding("provider-openai-backup", "openai-backup", "https://backup.api.openai.com/v1", "gpt-4.1-mini"))
}

func testProvider(providerID string, endpoint *providerv1.ProviderSurfaceBinding) *providerv1.Provider {
	return &providerv1.Provider{
		ProviderId:  providerID,
		DisplayName: providerID,
		Surfaces:    []*providerv1.ProviderSurfaceBinding{endpoint},
	}
}

func testProviderSurfaceBinding(providerID, surfaceID, baseURL, modelID string) *providerv1.ProviderSurfaceBinding {
	return &providerv1.ProviderSurfaceBinding{
		SurfaceId: surfaceID,
		SourceRef: &providerv1.ProviderSurfaceSourceRef{
			Kind:      providerv1.ProviderSurfaceSourceKind_PROVIDER_SURFACE_SOURCE_KIND_CLI,
			Id:        providerID,
			SurfaceId: surfaceID,
		},
		ProviderCredentialRef: &providerv1.ProviderCredentialRef{
			ProviderCredentialId: "credential-openai",
		},
		Runtime: &providerv1.ProviderSurfaceRuntime{
			DisplayName: surfaceID,
			Origin:      providerv1.ProviderSurfaceOrigin_PROVIDER_SURFACE_ORIGIN_MANUAL,
			Access: &providerv1.ProviderSurfaceRuntime_Api{
				Api: &providerv1.ProviderAPISurfaceRuntime{
					Protocol: apiprotocolv1.Protocol_PROTOCOL_OPENAI_RESPONSES,
					BaseUrl:  baseURL,
				},
			},
			Catalog: &providerv1.ProviderModelCatalog{
				Source: providerv1.CatalogSource_CATALOG_SOURCE_MODEL_SERVICE,
				Models: []*providerv1.ProviderModelCatalogEntry{{
					ProviderModelId: modelID,
					ModelRef:        &modelv1.ModelRef{VendorId: "openai", ModelId: modelID},
				}},
			},
		},
	}
}

func testResolvedProviderModel(instanceID string, baseURL string, modelID string) *providerv1.ResolvedProviderModel {
	return &providerv1.ResolvedProviderModel{
		SurfaceId:       instanceID,
		ProviderModelId: modelID,
		Protocol:        apiprotocolv1.Protocol_PROTOCOL_OPENAI_RESPONSES,
		BaseUrl:         baseURL,
		Model: &modelv1.ResolvedModel{
			ModelId: modelID,
			EffectiveDefinition: &modelv1.ModelVersion{
				VendorId:         "openai",
				ModelId:          modelID,
				DisplayName:      modelID,
				PrimaryShape:     modelv1.ModelShape_MODEL_SHAPE_CHAT_COMPLETIONS,
				SupportedShapes:  []modelv1.ModelShape{modelv1.ModelShape_MODEL_SHAPE_CHAT_COMPLETIONS},
				InputModalities:  []modelv1.Modality{modelv1.Modality_MODALITY_TEXT},
				OutputModalities: []modelv1.Modality{modelv1.Modality_MODALITY_TEXT},
			},
		},
		Source: providerv1.CatalogSource_CATALOG_SOURCE_MODEL_SERVICE,
		Surface: &providerv1.ResolvedProviderSurface{
			Surface: &providerv1.ProviderSurfaceRuntime{
				Access: &providerv1.ProviderSurfaceRuntime_Api{
					Api: &providerv1.ProviderAPISurfaceRuntime{
						Protocol: apiprotocolv1.Protocol_PROTOCOL_OPENAI_RESPONSES,
						BaseUrl:  baseURL,
					},
				},
			},
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
