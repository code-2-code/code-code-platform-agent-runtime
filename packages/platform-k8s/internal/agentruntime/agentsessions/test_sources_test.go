package agentsessions

import (
	"context"
	"strings"

	apiprotocolv1 "code-code.internal/go-contract/api_protocol/v1"
	"code-code.internal/go-contract/domainerror"
	modelv1 "code-code.internal/go-contract/model/v1"
	agentprofilev1 "code-code.internal/go-contract/platform/agent_profile/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	conditionv1 "code-code.internal/go-contract/platform/condition/v1"
	mcpv1 "code-code.internal/go-contract/platform/mcp/v1"
	rulev1 "code-code.internal/go-contract/platform/rule/v1"
	skillv1 "code-code.internal/go-contract/platform/skill/v1"
	supportv1 "code-code.internal/go-contract/platform/support/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentexecution"
	"code-code.internal/platform-k8s/internal/platform/resourceops"
	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type testProfileSource struct {
	profiles map[string]testProfileState
	rules    map[string]*rulev1.Rule
	skills   map[string]*skillv1.Skill
	mcps     map[string]*mcpv1.MCPServer
}

type testProfileState struct {
	profile    *agentprofilev1.AgentProfile
	generation int64
}

func newTestProfileSource(objects []any) *testProfileSource {
	source := &testProfileSource{
		profiles: map[string]testProfileState{},
		rules:    map[string]*rulev1.Rule{},
		skills:   map[string]*skillv1.Skill{},
		mcps:     map[string]*mcpv1.MCPServer{},
	}
	for _, object := range objects {
		switch resource := object.(type) {
		case testProfileState:
			source.profiles[strings.TrimSpace(resource.profile.GetProfileId())] = resource
		case *rulev1.Rule:
			source.rules[strings.TrimSpace(resource.GetRuleId())] = resource
		case *skillv1.Skill:
			source.skills[strings.TrimSpace(resource.GetSkillId())] = resource
		case *mcpv1.MCPServer:
			source.mcps[strings.TrimSpace(resource.GetMcpId())] = resource
		}
	}
	return source
}

func (s *testProfileSource) GetProfile(_ context.Context, profileID string) (*agentprofilev1.AgentProfile, int64, error) {
	state, ok := s.profiles[strings.TrimSpace(profileID)]
	if !ok {
		return nil, 0, domainerror.NewNotFound("test profile %q not found", profileID)
	}
	return state.profile, state.generation, nil
}

func (s *testProfileSource) GetRule(_ context.Context, ruleID string) (*rulev1.Rule, error) {
	rule, ok := s.rules[strings.TrimSpace(ruleID)]
	if !ok {
		return nil, domainerror.NewNotFound("test rule %q not found", ruleID)
	}
	return rule, nil
}

func (s *testProfileSource) GetSkill(_ context.Context, skillID string) (*skillv1.Skill, error) {
	skill, ok := s.skills[strings.TrimSpace(skillID)]
	if !ok {
		return nil, domainerror.NewNotFound("test skill %q not found", skillID)
	}
	return skill, nil
}

func (s *testProfileSource) GetMCP(_ context.Context, mcpID string) (*mcpv1.MCPServer, error) {
	mcp, ok := s.mcps[strings.TrimSpace(mcpID)]
	if !ok {
		return nil, domainerror.NewNotFound("test mcp %q not found", mcpID)
	}
	return mcp, nil
}

type testRuntimeReferences struct {
	executionClasses map[string]map[string]struct{}
	surfaces         map[string]*agentexecution.SurfaceBindingProjection
}

func newTestRuntimeReferences(objects []any) testRuntimeReferences {
	refs := testRuntimeReferences{
		executionClasses: map[string]map[string]struct{}{},
		surfaces:         map[string]*agentexecution.SurfaceBindingProjection{},
	}
	for _, object := range objects {
		switch resource := object.(type) {
		case testCLIReference:
			cliID := strings.TrimSpace(resource.cliID)
			if refs.executionClasses[cliID] == nil {
				refs.executionClasses[cliID] = map[string]struct{}{}
			}
			for _, executionClass := range resource.executionClasses {
				refs.executionClasses[cliID][strings.TrimSpace(executionClass)] = struct{}{}
			}
		case *providerv1.Provider:
			for _, surface := range resource.GetSurfaces() {
				refs.surfaces[strings.TrimSpace(surface.GetSurfaceId())] = &agentexecution.SurfaceBindingProjection{
					Surface: surface,
				}
			}
		}
	}
	return refs
}

type testCLIReference struct {
	cliID            string
	executionClasses []string
}

func newTestCLIReference(cliID string, executionClasses ...string) testCLIReference {
	if len(executionClasses) == 0 {
		executionClasses = []string{"default"}
	}
	return testCLIReference{cliID: cliID, executionClasses: executionClasses}
}

func testProviderSurfaceBindingProvider(surfaceID string) *providerv1.Provider {
	surfaceID = strings.TrimSpace(surfaceID)
	return &providerv1.Provider{
		ProviderId: "test-provider",
		Surfaces: []*providerv1.ProviderSurfaceBinding{{
			SurfaceId:             surfaceID,
			ProviderCredentialRef: &providerv1.ProviderCredentialRef{ProviderCredentialId: "credential-openai"},
			Runtime: &providerv1.ProviderSurfaceRuntime{
				DisplayName: "Test surface",
				Origin:      providerv1.ProviderSurfaceOrigin_PROVIDER_SURFACE_ORIGIN_DERIVED,
				Access: &providerv1.ProviderSurfaceRuntime_Api{
					Api: &providerv1.ProviderAPISurfaceRuntime{
						Protocol: apiprotocolv1.Protocol_PROTOCOL_OPENAI_RESPONSES,
						BaseUrl:  "https://api.openai.test/v1",
					},
				},
				Catalog: &providerv1.ProviderModelCatalog{
					Models: []*providerv1.ProviderModelCatalogEntry{{
						ProviderModelId: "gpt-5",
						ModelRef:        &modelv1.ModelRef{VendorId: "openai", ModelId: "gpt-5"},
					}},
					Source: providerv1.CatalogSource_CATALOG_SOURCE_VENDOR_PRESET,
				},
			},
		}},
	}
}

func (r testRuntimeReferences) ExecutionClassExists(_ context.Context, providerID, executionClass string) error {
	if classes := r.executionClasses[strings.TrimSpace(providerID)]; classes != nil {
		if _, ok := classes[strings.TrimSpace(executionClass)]; ok {
			return nil
		}
	}
	return domainerror.NewValidation("test execution class %q is not declared by cli definition %q", executionClass, providerID)
}

func (r testRuntimeReferences) ResolveContainerImage(ctx context.Context, providerID, executionClass string) (*agentexecution.ContainerImage, error) {
	if err := r.ExecutionClassExists(ctx, providerID, executionClass); err != nil {
		return nil, err
	}
	return &agentexecution.ContainerImage{
		Image:         "ghcr.io/example/" + strings.TrimSpace(providerID) + ":latest",
		CPURequest:    "1000m",
		MemoryRequest: "2Gi",
	}, nil
}

func (r testRuntimeReferences) GetProviderSurfaceBinding(_ context.Context, surfaceID string) (*agentexecution.SurfaceBindingProjection, error) {
	surface, ok := r.surfaces[strings.TrimSpace(surfaceID)]
	if !ok {
		return nil, domainerror.NewNotFound("test provider surface binding %q not found", surfaceID)
	}
	return surface, nil
}

func (r testRuntimeReferences) GetCLI(_ context.Context, cliID string) (*supportv1.CLI, error) {
	cliID = strings.TrimSpace(cliID)
	if cliID == "" {
		return nil, domainerror.NewNotFound("test cli support is empty")
	}
	return &supportv1.CLI{
		CliId: cliID,
		ApiKeyProtocols: []*supportv1.APIKeyProtocolSupport{{
			Protocol: apiprotocolv1.Protocol_PROTOCOL_OPENAI_RESPONSES,
			AuthMaterialization: &supportv1.CLIAuthMaterialization{
				MaterializationKey:       cliID + ".openai-api-key",
				RuntimeUrlProjectionKind: supportv1.RuntimeProjectionKind_RUNTIME_PROJECTION_KIND_BASE_URL,
				IncludeRuntimeUrlHost:    true,
				RequestAuthInjection: &supportv1.RequestAuthInjection{
					HeaderNames:       []string{"authorization"},
					HeaderValuePrefix: "Bearer",
				},
			},
		}},
	}, nil
}

type testModelRegistry struct{}

func (testModelRegistry) ResolveRef(_ context.Context, modelIDOrAlias string) (*modelv1.ModelRef, error) {
	modelIDOrAlias = strings.TrimSpace(modelIDOrAlias)
	if modelIDOrAlias == "" {
		return nil, domainerror.NewValidation("test model id is empty")
	}
	return &modelv1.ModelRef{VendorId: "openai", ModelId: modelIDOrAlias}, nil
}

func (testModelRegistry) Resolve(_ context.Context, ref *modelv1.ModelRef, _ *modelv1.ModelOverride) (*modelv1.ResolvedModel, error) {
	return &modelv1.ResolvedModel{
		ModelId: strings.TrimSpace(ref.GetModelId()),
		EffectiveDefinition: &modelv1.ModelVersion{
			VendorId:         strings.TrimSpace(ref.GetVendorId()),
			ModelId:          strings.TrimSpace(ref.GetModelId()),
			DisplayName:      strings.TrimSpace(ref.GetModelId()),
			PrimaryShape:     modelv1.ModelShape_MODEL_SHAPE_CHAT_COMPLETIONS,
			SupportedShapes:  []modelv1.ModelShape{modelv1.ModelShape_MODEL_SHAPE_CHAT_COMPLETIONS},
			InputModalities:  []modelv1.Modality{modelv1.Modality_MODALITY_TEXT},
			OutputModalities: []modelv1.Modality{modelv1.Modality_MODALITY_TEXT},
		},
	}, nil
}

func testRuntimeObjectsFromAny(objects []any) []runtime.Object {
	out := make([]runtime.Object, 0, len(objects))
	for _, object := range objects {
		if runtimeObject, ok := object.(runtime.Object); ok {
			out = append(out, runtimeObject)
		}
	}
	return out
}

type fakeSessionRepository struct {
	client    ctrlclient.Client
	namespace string
}

func (r fakeSessionRepository) Get(ctx context.Context, sessionID string) (*agentsessionv1.AgentSessionState, error) {
	return r.load(ctx, sessionID)
}

func (r fakeSessionRepository) Update(ctx context.Context, sessionID string, spec *agentsessionv1.AgentSessionSpec) (*agentsessionv1.AgentSessionState, error) {
	key := types.NamespacedName{Namespace: r.namespace, Name: strings.TrimSpace(sessionID)}
	if err := resourceops.UpdateResource(ctx, r.client, key, func(current *platformv1alpha1.AgentSessionResource) error {
		current.Spec.Session = proto.Clone(spec).(*agentsessionv1.AgentSessionSpec)
		return nil
	}, func() *platformv1alpha1.AgentSessionResource {
		return &platformv1alpha1.AgentSessionResource{}
	}); err != nil {
		return nil, err
	}
	return r.load(ctx, sessionID)
}

func (r fakeSessionRepository) ClaimActiveRun(ctx context.Context, sessionID string, runID string) (*agentsessionv1.AgentSessionState, error) {
	key := types.NamespacedName{Namespace: r.namespace, Name: strings.TrimSpace(sessionID)}
	if err := resourceops.UpdateStatus(ctx, r.client, key, func(current *platformv1alpha1.AgentSessionResource) error {
		current.Status.ActiveRunID = strings.TrimSpace(runID)
		current.Status.Phase = platformv1alpha1.AgentSessionResourcePhaseRunning
		return nil
	}, func() *platformv1alpha1.AgentSessionResource {
		return &platformv1alpha1.AgentSessionResource{}
	}); err != nil {
		return nil, err
	}
	return r.load(ctx, sessionID)
}

func (r fakeSessionRepository) ReleaseActiveRun(ctx context.Context, sessionID string, runID string) (bool, error) {
	released := false
	key := types.NamespacedName{Namespace: r.namespace, Name: strings.TrimSpace(sessionID)}
	if err := resourceops.UpdateStatus(ctx, r.client, key, func(current *platformv1alpha1.AgentSessionResource) error {
		if strings.TrimSpace(current.Status.ActiveRunID) != strings.TrimSpace(runID) {
			return nil
		}
		current.Status.ActiveRunID = ""
		if current.Status.Phase == platformv1alpha1.AgentSessionResourcePhaseRunning {
			current.Status.Phase = platformv1alpha1.AgentSessionResourcePhaseReady
		}
		released = true
		return nil
	}, func() *platformv1alpha1.AgentSessionResource {
		return &platformv1alpha1.AgentSessionResource{}
	}); err != nil {
		return false, err
	}
	return released, nil
}

func (r fakeSessionRepository) UpdateStatus(ctx context.Context, sessionID string, status *agentsessionv1.AgentSessionStatus) (*agentsessionv1.AgentSessionState, error) {
	key := types.NamespacedName{Namespace: r.namespace, Name: strings.TrimSpace(sessionID)}
	if err := resourceops.UpdateStatus(ctx, r.client, key, func(current *platformv1alpha1.AgentSessionResource) error {
		current.Status = resourceStatusFromProto(status)
		return nil
	}, func() *platformv1alpha1.AgentSessionResource {
		return &platformv1alpha1.AgentSessionResource{}
	}); err != nil {
		return nil, err
	}
	return r.load(ctx, sessionID)
}

type fakeSessionActionReader struct {
	client    ctrlclient.Client
	namespace string
}

func (r fakeSessionActionReader) HasNonterminalResetWarmState(ctx context.Context, sessionID string) (bool, error) {
	items, err := r.ListBySession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	for i := range items {
		action := items[i].Spec.Action
		if action == nil || action.GetType() != agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RESET_WARM_STATE {
			continue
		}
		switch items[i].Status.Phase {
		case platformv1alpha1.AgentSessionActionResourcePhaseSucceeded,
			platformv1alpha1.AgentSessionActionResourcePhaseFailed,
			platformv1alpha1.AgentSessionActionResourcePhaseCanceled:
			continue
		default:
			return true, nil
		}
	}
	return false, nil
}

func (r fakeSessionActionReader) ListBySession(ctx context.Context, sessionID string) ([]platformv1alpha1.AgentSessionActionResource, error) {
	list := &platformv1alpha1.AgentSessionActionResourceList{}
	if err := r.client.List(ctx, list, ctrlclient.InNamespace(r.namespace)); err != nil {
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

func (r fakeSessionRepository) load(ctx context.Context, sessionID string) (*agentsessionv1.AgentSessionState, error) {
	resource := &platformv1alpha1.AgentSessionResource{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: r.namespace, Name: strings.TrimSpace(sessionID)}, resource); err != nil {
		return nil, err
	}
	return sessionStateFromResource(resource)
}

func resourceStatusFromProto(status *agentsessionv1.AgentSessionStatus) platformv1alpha1.AgentSessionResourceStatus {
	if status == nil {
		return platformv1alpha1.AgentSessionResourceStatus{}
	}
	out := platformv1alpha1.AgentSessionResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: status.GetObservedGeneration(),
			Conditions:         metaConditions(status.GetConditions()),
		},
		Phase:                    resourcePhase(status.GetPhase()),
		RuntimeConfigGeneration:  status.GetRuntimeConfigGeneration(),
		ResourceConfigGeneration: status.GetResourceConfigGeneration(),
		RealizedRuleRevision:     status.GetRealizedRuleRevision(),
		RealizedSkillRevision:    status.GetRealizedSkillRevision(),
		RealizedMCPRevision:      status.GetRealizedMcpRevision(),
		ObservedHomeStateID:      status.GetObservedHomeStateId(),
		StateGeneration:          status.GetStateGeneration(),
		Message:                  status.GetMessage(),
	}
	if status.GetActiveRun() != nil {
		out.ActiveRunID = strings.TrimSpace(status.GetActiveRun().GetRunId())
	}
	if status.GetUpdatedAt() != nil {
		updatedAt := metav1.NewTime(status.GetUpdatedAt().AsTime())
		out.UpdatedAt = &updatedAt
	}
	return out
}

func metaConditions(items []*conditionv1.Condition) []metav1.Condition {
	if len(items) == 0 {
		return nil
	}
	out := make([]metav1.Condition, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		condition := metav1.Condition{
			Type:               item.GetType(),
			Status:             metaConditionStatus(item.GetStatus()),
			Reason:             item.GetReason(),
			Message:            item.GetMessage(),
			ObservedGeneration: item.GetObservedGeneration(),
		}
		if item.GetLastTransitionTime() != nil {
			condition.LastTransitionTime = metav1.NewTime(item.GetLastTransitionTime().AsTime())
		}
		out = append(out, condition)
	}
	return out
}

func metaConditionStatus(status conditionv1.ConditionStatus) metav1.ConditionStatus {
	switch status {
	case conditionv1.ConditionStatus_CONDITION_STATUS_TRUE:
		return metav1.ConditionTrue
	case conditionv1.ConditionStatus_CONDITION_STATUS_FALSE:
		return metav1.ConditionFalse
	default:
		return metav1.ConditionUnknown
	}
}

func resourcePhase(phase agentsessionv1.AgentSessionPhase) platformv1alpha1.AgentSessionResourcePhase {
	switch phase {
	case agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_PENDING:
		return platformv1alpha1.AgentSessionResourcePhasePending
	case agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_READY:
		return platformv1alpha1.AgentSessionResourcePhaseReady
	case agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_RUNNING:
		return platformv1alpha1.AgentSessionResourcePhaseRunning
	case agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_FAILED:
		return platformv1alpha1.AgentSessionResourcePhaseFailed
	default:
		return ""
	}
}
