package agentsessions

import (
	"context"
	"testing"
	"time"

	capv1 "code-code.internal/go-contract/agent/cap/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentresourceconfig"
	"code-code.internal/platform-k8s/internal/platform/testutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcilerMarksReadySession(t *testing.T) {
	ctx := context.Background()
	resource := newSessionResource("agent-session-1", 7, validSessionSpec())
	markResourceConfigRealized(resource, 7)
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getSessionResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentSessionResourcePhaseReady {
		t.Fatalf("phase = %q, want READY", updated.Status.Phase)
	}
	if updated.Status.ObservedGeneration != 7 {
		t.Fatalf("observed generation = %d, want 7", updated.Status.ObservedGeneration)
	}
	if got, want := updated.Status.StateGeneration, int64(7); got != want {
		t.Fatalf("state generation = %d, want %d", got, want)
	}
	if got, want := updated.Status.ObservedHomeStateID, "home-1"; got != want {
		t.Fatalf("observed home_state_id = %q, want %q", got, want)
	}
	assertCondition(t, updated.Status.Conditions, string(platformcontract.AgentSessionConditionTypeReadyForNextRun), string(metav1.ConditionTrue))
}

func TestReconcilerMarksEmptyResourceConfigReady(t *testing.T) {
	ctx := context.Background()
	spec := validSessionSpec()
	spec.ResourceConfig = &capv1.AgentResources{}
	resource := newSessionResource("agent-session-1", 7, spec)
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getSessionResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentSessionResourcePhaseReady {
		t.Fatalf("phase = %q, want READY", updated.Status.Phase)
	}
	if updated.Status.ResourceConfigGeneration != 7 {
		t.Fatalf("resource config generation = %d, want 7", updated.Status.ResourceConfigGeneration)
	}
	assertCondition(t, updated.Status.Conditions, string(platformcontract.AgentSessionConditionTypeResourceConfigReady), string(metav1.ConditionTrue))
}

func TestReconcilerKeepsActiveSessionRunning(t *testing.T) {
	ctx := context.Background()
	resource := newSessionResource("agent-session-1", 7, validSessionSpec())
	markResourceConfigRealized(resource, 7)
	resource.Status.ActiveRunID = "run-1"
	client := newClient(resource, activeRunResource("run-1"))
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getSessionResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentSessionResourcePhaseRunning {
		t.Fatalf("phase = %q, want RUNNING", updated.Status.Phase)
	}
	assertCondition(t, updated.Status.Conditions, string(platformcontract.AgentSessionConditionTypeReadyForNextRun), string(metav1.ConditionFalse))
	if updated.Status.ActiveRunID != "run-1" {
		t.Fatalf("active run not preserved: %q", updated.Status.ActiveRunID)
	}
}

func TestReconcilerMarksMissingRuntimeConfigPending(t *testing.T) {
	ctx := context.Background()
	spec := validSessionSpec()
	spec.RuntimeConfig.ProviderRuntimeRef = nil
	resource := newSessionResource("agent-session-1", 7, spec)
	markResourceConfigRealized(resource, 7)
	resource.Status.RuntimeConfigGeneration = 3
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getSessionResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentSessionResourcePhasePending {
		t.Fatalf("phase = %q, want PENDING", updated.Status.Phase)
	}
	if updated.Status.RuntimeConfigGeneration != 3 {
		t.Fatalf("runtime config generation = %d, want 3", updated.Status.RuntimeConfigGeneration)
	}
	if updated.Status.ResourceConfigGeneration != 7 {
		t.Fatalf("resource config generation = %d, want 7", updated.Status.ResourceConfigGeneration)
	}
	assertCondition(t, updated.Status.Conditions, string(platformcontract.AgentSessionConditionTypeRuntimeConfigReady), string(metav1.ConditionFalse))
	assertCondition(t, updated.Status.Conditions, string(platformcontract.AgentSessionConditionTypeReadyForNextRun), string(metav1.ConditionFalse))
}

func TestReconcilerRejectsMissingExecutionClass(t *testing.T) {
	ctx := context.Background()
	spec := validSessionSpec()
	spec.ExecutionClass = ""
	resource := newSessionResource("agent-session-1", 7, spec)
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getSessionResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentSessionResourcePhaseFailed {
		t.Fatalf("phase = %q, want FAILED", updated.Status.Phase)
	}
	assertCondition(t, updated.Status.Conditions, string(platformcontract.AgentSessionConditionTypeReadyForNextRun), string(metav1.ConditionFalse))
}

func TestReconcilerAcceptsInlineSessionWithoutProfileProvenance(t *testing.T) {
	ctx := context.Background()
	spec := validSessionSpec()
	spec.ProfileId = ""
	spec.ProfileGeneration = 0
	resource := newSessionResource("agent-session-1", 7, spec)
	markResourceConfigRealized(resource, 7)
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getSessionResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentSessionResourcePhaseReady {
		t.Fatalf("phase = %q, want READY", updated.Status.Phase)
	}
}

func TestReconcilerPreservesStateGenerationAcrossRuntimeDrift(t *testing.T) {
	ctx := context.Background()
	resource := newSessionResource("agent-session-1", 9, validSessionSpec())
	markResourceConfigRealized(resource, 9)
	resource.Status.StateGeneration = 7
	resource.Status.ObservedHomeStateID = "home-1"
	resource.Status.RuntimeConfigGeneration = 8
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getSessionResource(t, ctx, client, resource.Name)
	if got, want := updated.Status.StateGeneration, int64(7); got != want {
		t.Fatalf("state generation = %d, want %d", got, want)
	}
	if got, want := updated.Status.ObservedHomeStateID, "home-1"; got != want {
		t.Fatalf("observed home_state_id = %q, want %q", got, want)
	}
}

func TestReconcilerBumpsStateGenerationWhenHomeStateChanges(t *testing.T) {
	ctx := context.Background()
	spec := validSessionSpec()
	spec.HomeStateRef = &agentsessionv1.AgentSessionHomeStateRef{HomeStateId: "home-2"}
	resource := newSessionResource("agent-session-1", 9, spec)
	markResourceConfigRealized(resource, 9)
	resource.Status.StateGeneration = 7
	resource.Status.ObservedHomeStateID = "home-1"
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getSessionResource(t, ctx, client, resource.Name)
	if got, want := updated.Status.StateGeneration, int64(9); got != want {
		t.Fatalf("state generation = %d, want %d", got, want)
	}
	if got, want := updated.Status.ObservedHomeStateID, "home-2"; got != want {
		t.Fatalf("observed home_state_id = %q, want %q", got, want)
	}
}

func TestReconcilerCreatesCarrierPVCsAndMarksSessionReadyBeforeBinding(t *testing.T) {
	ctx := context.Background()
	resource := newSessionResource("agent-session-1", 7, validSessionSpec())
	markResourceConfigRealized(resource, 7)
	client := newClientWithoutAutoCarriers(resource)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getSessionResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentSessionResourcePhaseReady {
		t.Fatalf("phase = %q, want READY", updated.Status.Phase)
	}
	assertCondition(t, updated.Status.Conditions, string(platformcontract.AgentSessionConditionTypeWorkspaceReady), string(metav1.ConditionTrue))
	assertCondition(t, updated.Status.Conditions, string(platformcontract.AgentSessionConditionTypeWarmStateReady), string(metav1.ConditionTrue))
	if got, want := updated.Status.StateGeneration, int64(7); got != want {
		t.Fatalf("state generation = %d, want %d", got, want)
	}
	workspace := &corev1.PersistentVolumeClaim{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code-runs", Name: WorkspacePVCName("agent-session-1", "workspace-1")}, workspace); err != nil {
		t.Fatalf("get workspace pvc: %v", err)
	}
	homeState := &corev1.PersistentVolumeClaim{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code-runs", Name: HomeStatePVCName("agent-session-1", "home-1")}, homeState); err != nil {
		t.Fatalf("get home-state pvc: %v", err)
	}
}

func TestReconcilerMarksWarmStatePendingWhileResetActionExists(t *testing.T) {
	ctx := context.Background()
	resource := newSessionResource("agent-session-1", 7, validSessionSpec())
	markResourceConfigRealized(resource, 7)
	client := newClient(resource, pendingResetWarmStateAction("reset-1", "agent-session-1", "home-1", "home-2"))
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getSessionResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentSessionResourcePhasePending {
		t.Fatalf("phase = %q, want PENDING", updated.Status.Phase)
	}
	assertCondition(t, updated.Status.Conditions, string(platformcontract.AgentSessionConditionTypeWarmStateReady), string(metav1.ConditionFalse))
	assertCondition(t, updated.Status.Conditions, string(platformcontract.AgentSessionConditionTypeReadyForNextRun), string(metav1.ConditionFalse))
	if got, want := updated.Status.Message, "AgentSession warm state reset is in progress."; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestReconcilerKeepsReferencedStaleCarriers(t *testing.T) {
	ctx := context.Background()
	spec := validSessionSpec()
	spec.WorkspaceRef = &agentsessionv1.AgentSessionWorkspaceRef{WorkspaceId: "workspace-2"}
	spec.HomeStateRef = &agentsessionv1.AgentSessionHomeStateRef{HomeStateId: "home-2"}
	resource := newSessionResource("agent-session-1", 9, spec)
	markResourceConfigRealized(resource, 9)
	action := pendingReferencedCarrierAction("action-1", "agent-session-1", "workspace-1", "home-1")
	oldCarriers := boundCarrierPVCsForSession(&agentsessionv1.AgentSessionSpec{
		SessionId:    "agent-session-1",
		WorkspaceRef: &agentsessionv1.AgentSessionWorkspaceRef{WorkspaceId: "workspace-1"},
		HomeStateRef: &agentsessionv1.AgentSessionHomeStateRef{HomeStateId: "home-1"},
	})
	client := newClientWithoutAutoCarriers(resource, action, oldCarriers[0], oldCarriers[1])
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code-runs", Name: WorkspacePVCName("agent-session-1", "workspace-1")}, &corev1.PersistentVolumeClaim{}); err != nil {
		t.Fatalf("get stale workspace pvc: %v", err)
	}
	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code-runs", Name: HomeStatePVCName("agent-session-1", "home-1")}, &corev1.PersistentVolumeClaim{}); err != nil {
		t.Fatalf("get stale home-state pvc: %v", err)
	}
	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code-runs", Name: WorkspacePVCName("agent-session-1", "workspace-2")}, &corev1.PersistentVolumeClaim{}); err != nil {
		t.Fatalf("get current workspace pvc: %v", err)
	}
	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code-runs", Name: HomeStatePVCName("agent-session-1", "home-2")}, &corev1.PersistentVolumeClaim{}); err != nil {
		t.Fatalf("get current home-state pvc: %v", err)
	}
}

func TestReconcilerSkipsUnchangedStatusUpdate(t *testing.T) {
	ctx := context.Background()
	resource := newSessionResource("agent-session-1", 7, validSessionSpec())
	markResourceConfigRealized(resource, 7)
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	first := getSessionResource(t, ctx, client, resource.Name)
	firstUpdatedAt := first.Status.UpdatedAt
	firstTransitionTime := conditionTransitionTime(t, first.Status.Conditions, string(platformcontract.AgentSessionConditionTypeReadyForNextRun))

	reconciler.now = func() time.Time {
		return time.Date(2026, 4, 14, 2, 3, 4, 0, time.UTC)
	}
	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	second := getSessionResource(t, ctx, client, resource.Name)
	if !second.Status.UpdatedAt.Equal(firstUpdatedAt) {
		t.Fatalf("updatedAt changed: %v -> %v", firstUpdatedAt, second.Status.UpdatedAt)
	}
	secondTransitionTime := conditionTransitionTime(t, second.Status.Conditions, string(platformcontract.AgentSessionConditionTypeReadyForNextRun))
	if !secondTransitionTime.Equal(&firstTransitionTime) {
		t.Fatalf("transition time changed: %v -> %v", firstTransitionTime, secondTransitionTime)
	}
}

func TestReconcilerRecordsCreatedAndReadyTimelineEvents(t *testing.T) {
	ctx := context.Background()
	resource := newSessionResource("agent-session-1", 7, validSessionSpec())
	markResourceConfigRealized(resource, 7)
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)
	sink := &fakeTimelineSink{}
	reconciler.SetTimelineSink(sink)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if len(sink.events) != 2 {
		t.Fatalf("recorded events = %d, want 2", len(sink.events))
	}
	if sink.events[0].EventType != "CREATED" {
		t.Fatalf("first event type = %q, want CREATED", sink.events[0].EventType)
	}
	if sink.events[1].EventType != "READY" {
		t.Fatalf("second event type = %q, want READY", sink.events[1].EventType)
	}
}

func TestReconcilerEnsuresOnlyStaleSkillReloadAction(t *testing.T) {
	ctx := context.Background()
	spec := validSessionSpec()
	spec.ResourceConfig = resourceConfigWithSubjects("resources-1")
	resource := newSessionResource("agent-session-1", 7, spec)
	desired := agentresourceconfig.DesiredRevisions(spec.GetResourceConfig())
	resource.Status.RealizedRuleRevision = desired.Rule
	resource.Status.RealizedMCPRevision = desired.MCP
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getSessionResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentSessionResourcePhasePending {
		t.Fatalf("phase = %q, want PENDING", updated.Status.Phase)
	}
	assertCondition(t, updated.Status.Conditions, string(platformcontract.AgentSessionConditionTypeResourceConfigReady), string(metav1.ConditionFalse))
	action := &platformv1alpha1.AgentSessionActionResource{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code", Name: "agent-session-1-skill-reload-g7"}, action); err != nil {
		t.Fatalf("get action: %v", err)
	}
	if got, want := action.Spec.Action.GetInputSnapshot().GetReloadSubject().GetSubject(), agentsessionactionv1.AgentSessionActionSubject_AGENT_SESSION_ACTION_SUBJECT_SKILL; got != want {
		t.Fatalf("reload subject = %q, want %q", got, want)
	}
	if got, want := action.Spec.Action.GetInputSnapshot().GetReloadSubject().GetSubjectRevision(), desired.Skill; got != want {
		t.Fatalf("reload subject revision = %q, want %q", got, want)
	}
	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code", Name: "agent-session-1-rule-reload-g7"}, &platformv1alpha1.AgentSessionActionResource{}); !apierrors.IsNotFound(err) {
		t.Fatalf("rule reload action err = %v, want not found", err)
	}
	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code", Name: "agent-session-1-mcp-reload-g7"}, &platformv1alpha1.AgentSessionActionResource{}); !apierrors.IsNotFound(err) {
		t.Fatalf("mcp reload action err = %v, want not found", err)
	}
}

func newTestReconciler(t *testing.T, client ctrlclient.Client) *Reconciler {
	t.Helper()
	runtimeReferences := newTestRuntimeReferences(readinessRuntimeReferenceObjects())
	reconciler, err := NewReconciler(ReconcilerConfig{
		Client:           client,
		Namespace:        "code-code",
		RuntimeNamespace: "code-code-runs",
		ProfileSource:    newTestProfileSource(nil),
		RuntimeCatalog:   runtimeReferences,
		ModelRegistry:    testModelRegistry{},
		Sessions:         fakeSessionRepository{client: client, namespace: "code-code"},
		Actions:          fakeSessionActionReader{client: client, namespace: "code-code"},
		Now: func() time.Time {
			return time.Date(2026, 4, 14, 1, 2, 3, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new reconciler: %v", err)
	}
	return reconciler
}

func readinessRuntimeReferenceObjects() []any {
	objects := make([]any, 0, len(readinessDependencyObjects())+1)
	objects = append(objects, newTestCLIReference("codex"))
	for _, object := range readinessDependencyObjects() {
		objects = append(objects, object)
	}
	objects = append(objects, testProviderSurfaceBindingProvider("provider-instance-1"))
	return objects
}

func newClient(objects ...ctrlclient.Object) ctrlclient.Client {
	return newClientWithCarrierMode(true, objects...)
}

func newClientWithoutAutoCarriers(objects ...ctrlclient.Object) ctrlclient.Client {
	return newClientWithCarrierMode(false, objects...)
}

func newClientWithCarrierMode(autoCarriers bool, objects ...ctrlclient.Object) ctrlclient.Client {
	baseObjects := readinessDependencyObjects()
	baseObjects = append(baseObjects, objects...)
	if autoCarriers {
		for _, object := range objects {
			session, ok := object.(*platformv1alpha1.AgentSessionResource)
			if !ok || session.Spec.Session == nil {
				continue
			}
			baseObjects = append(baseObjects, boundCarrierPVCsForSession(session.Spec.Session)...)
		}
	}
	client := ctrlclientfake.NewClientBuilder().
		WithScheme(testutil.NewScheme()).
		WithObjects(baseObjects...).
		WithStatusSubresource(&platformv1alpha1.AgentSessionResource{}, &platformv1alpha1.AgentRunResource{}).
		Build()
	materializer, err := agentresourceconfig.NewMaterializer(client, "code-code")
	if err != nil {
		panic(err)
	}
	for _, object := range baseObjects {
		session, ok := object.(*platformv1alpha1.AgentSessionResource)
		if !ok || session.Spec.Session == nil || session.Status.ResourceConfigGeneration == 0 {
			continue
		}
		if err := materializer.Ensure(context.Background(), session.Spec.Session.GetSessionId(), session.Spec.Session.GetResourceConfig()); err != nil {
			panic(err)
		}
	}
	return client
}

func pendingReferencedCarrierAction(actionID string, sessionID string, workspaceID string, homeStateID string) *platformv1alpha1.AgentSessionActionResource {
	return &platformv1alpha1.AgentSessionActionResource{
		TypeMeta: metav1.TypeMeta{APIVersion: platformv1alpha1.GroupVersion.String(), Kind: platformv1alpha1.KindAgentSessionActionResource},
		ObjectMeta: metav1.ObjectMeta{
			Name:      actionID,
			Namespace: "code-code",
			Labels: map[string]string{
				"agentsessionaction.code-code.internal/session-id": sessionID,
				"agentsessionaction.code-code.internal/type":       agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RUN_TURN.String(),
			},
		},
		Spec: platformv1alpha1.AgentSessionActionResourceSpec{
			Action: &agentsessionactionv1.AgentSessionActionSpec{
				ActionId:  actionID,
				SessionId: sessionID,
				Type:      agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RUN_TURN,
				InputSnapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot{
					Snapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot_RunTurn{
						RunTurn: &agentsessionactionv1.AgentSessionRunTurnSnapshot{
							WorkspaceId: workspaceID,
							HomeStateId: homeStateID,
						},
					},
				},
			},
		},
		Status: platformv1alpha1.AgentSessionActionResourceStatus{
			Phase: platformv1alpha1.AgentSessionActionResourcePhasePending,
		},
	}
}

func pendingResetWarmStateAction(actionID string, sessionID string, sourceHomeStateID string, targetHomeStateID string) *platformv1alpha1.AgentSessionActionResource {
	return &platformv1alpha1.AgentSessionActionResource{
		TypeMeta: metav1.TypeMeta{APIVersion: platformv1alpha1.GroupVersion.String(), Kind: platformv1alpha1.KindAgentSessionActionResource},
		ObjectMeta: metav1.ObjectMeta{
			Name:      actionID,
			Namespace: "code-code",
			Labels: map[string]string{
				"agentsessionaction.code-code.internal/session-id": sessionID,
				"agentsessionaction.code-code.internal/type":       agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RESET_WARM_STATE.String(),
			},
		},
		Spec: platformv1alpha1.AgentSessionActionResourceSpec{
			Action: &agentsessionactionv1.AgentSessionActionSpec{
				ActionId:  actionID,
				SessionId: sessionID,
				Type:      agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RESET_WARM_STATE,
				InputSnapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot{
					Snapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot_ResetWarmState{
						ResetWarmState: &agentsessionactionv1.AgentSessionResetWarmStateSnapshot{
							SessionGeneration: 7,
							SourceHomeStateId: sourceHomeStateID,
							TargetHomeStateId: targetHomeStateID,
						},
					},
				},
			},
		},
		Status: platformv1alpha1.AgentSessionActionResourceStatus{
			Phase: platformv1alpha1.AgentSessionActionResourcePhasePending,
		},
	}
}

func requestFor(resource *platformv1alpha1.AgentSessionResource) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Namespace: resource.Namespace, Name: resource.Name}}
}

func getSessionResource(t *testing.T, ctx context.Context, client ctrlclient.Client, name string) *platformv1alpha1.AgentSessionResource {
	t.Helper()
	resource := &platformv1alpha1.AgentSessionResource{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code", Name: name}, resource); err != nil {
		t.Fatalf("get session: %v", err)
	}
	return resource
}

func newSessionResource(name string, generation int64, spec *agentsessionv1.AgentSessionSpec) *platformv1alpha1.AgentSessionResource {
	return &platformv1alpha1.AgentSessionResource{
		TypeMeta: metav1.TypeMeta{APIVersion: platformv1alpha1.GroupVersion.String(), Kind: platformv1alpha1.KindAgentSessionResource},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "code-code",
			Generation: generation,
		},
		Spec: platformv1alpha1.AgentSessionResourceSpec{Session: spec},
	}
}

func validSessionSpec() *agentsessionv1.AgentSessionSpec {
	return &agentsessionv1.AgentSessionSpec{
		SessionId:      "agent-session-1",
		ProviderId:     "codex",
		ExecutionClass: "default",
		RuntimeConfig: &agentsessionv1.AgentSessionRuntimeConfig{
			ProviderRuntimeRef: &providerv1.ProviderRuntimeRef{
				SurfaceId: "provider-instance-1",
			},
		},
		ResourceConfig: &capv1.AgentResources{
			SnapshotId: "resources-1",
		},
		WorkspaceRef: &agentsessionv1.AgentSessionWorkspaceRef{WorkspaceId: "workspace-1"},
		HomeStateRef: &agentsessionv1.AgentSessionHomeStateRef{HomeStateId: "home-1"},
	}
}

func markResourceConfigRealized(resource *platformv1alpha1.AgentSessionResource, generation int64) {
	desired := agentresourceconfig.DesiredRevisions(resource.Spec.Session.GetResourceConfig())
	resource.Status.ResourceConfigGeneration = generation
	resource.Status.RealizedRuleRevision = desired.Rule
	resource.Status.RealizedSkillRevision = desired.Skill
	resource.Status.RealizedMCPRevision = desired.MCP
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

func assertCondition(t *testing.T, conditions []metav1.Condition, conditionType string, status string) {
	t.Helper()
	for _, condition := range conditions {
		if condition.Type == conditionType {
			if string(condition.Status) != status {
				t.Fatalf("condition %s status = %s, want %s", conditionType, condition.Status, status)
			}
			return
		}
	}
	t.Fatalf("condition %s not found in %+v", conditionType, conditions)
}

func conditionTransitionTime(t *testing.T, conditions []metav1.Condition, conditionType string) metav1.Time {
	t.Helper()
	for _, condition := range conditions {
		if condition.Type == conditionType {
			if condition.LastTransitionTime.IsZero() {
				t.Fatalf("condition %s transition time is nil", conditionType)
			}
			return condition.LastTransitionTime
		}
	}
	t.Fatalf("condition %s not found in %+v", conditionType, conditions)
	return metav1.Time{}
}

type fakeTimelineSink struct {
	events []*platformcontract.TimelineEvent
}

func (*fakeTimelineSink) RecordStageInterval(context.Context, *platformcontract.StageInterval) error {
	return nil
}

func (f *fakeTimelineSink) RecordEvent(_ context.Context, event *platformcontract.TimelineEvent) error {
	f.events = append(f.events, event)
	return nil
}

func (*fakeTimelineSink) Close() {}
