package agentruns

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "code-code.internal/go-contract/agent/core/v1"
	inputv1 "code-code.internal/go-contract/agent/input/v1"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/workflows"
	"code-code.internal/platform-k8s/internal/platform/testutil"
	"google.golang.org/protobuf/types/known/structpb"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcilerAcceptsValidRun(t *testing.T) {
	ctx := context.Background()
	resource := newRunResource("agent-run-1", 5, validRunSpec())
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getRunResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentRunResourcePhasePending {
		t.Fatalf("phase = %q, want PENDING", updated.Status.Phase)
	}
	assertCondition(t, updated.Status.Conditions, string(platformcontract.AgentRunConditionTypeAccepted), string(metav1.ConditionTrue))
}

func TestReconcilerSchedulesAcceptedRun(t *testing.T) {
	ctx := context.Background()
	resource := newRunResource("agent-run-1", 5, validRunSpec())
	client := newClient(resource)
	runtime := &fakeWorkflowRuntime{}
	reconciler := newTestReconciler(t, client)
	reconciler.workflowRuntime = runtime
	sink := &fakeTimelineSink{}
	reconciler.SetTimelineSink(sink)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	result, err := reconciler.Reconcile(ctx, requestFor(resource))
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if result.RequeueAfter != workflowPollInterval {
		t.Fatalf("requeue_after = %s, want %s", result.RequeueAfter, workflowPollInterval)
	}

	updated := getRunResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentRunResourcePhaseScheduled {
		t.Fatalf("phase = %q, want SCHEDULED", updated.Status.Phase)
	}
	if got := strings.TrimSpace(updated.Status.WorkloadID); got != "agent-run-1" {
		t.Fatalf("workload id = %q, want agent-run-1", got)
	}
	if len(runtime.submitted) != 1 {
		t.Fatalf("submitted = %d, want 1", len(runtime.submitted))
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "SCHEDULED" {
		t.Fatalf("events = %+v, want one SCHEDULED event", sink.events)
	}
	assertCondition(t, updated.Status.Conditions, string(platformcontract.AgentRunConditionTypeWorkloadReady), string(metav1.ConditionTrue))
}

func TestReconcilerPollsScheduledRunWhenWorkflowStateIsUnchanged(t *testing.T) {
	ctx := context.Background()
	resource := newRunResource("agent-run-1", 5, validRunSpec())
	resource.Status = platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: 5,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), "AgentRun is accepted.", 5, time.Date(2026, 4, 14, 1, 2, 3, 0, time.UTC)),
				newCondition(platformcontract.AgentRunConditionTypeWorkloadReady, true, string(platformcontract.AgentRunConditionReasonWorkloadCreated), "AgentRun workflow submitted.", 5, time.Date(2026, 4, 14, 1, 2, 4, 0, time.UTC)),
			},
		},
		Phase:      platformv1alpha1.AgentRunResourcePhaseScheduled,
		Message:    "AgentRun workflow submitted.",
		WorkloadID: "agent-run-1",
		UpdatedAt:  timePtr(time.Date(2026, 4, 14, 1, 2, 4, 0, time.UTC)),
	}
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)
	reconciler.workflowRuntime = &fakeWorkflowRuntime{states: []*WorkflowState{{Phase: "Pending", Message: ""}}}

	result, err := reconciler.Reconcile(ctx, requestFor(resource))
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if result.RequeueAfter != workflowPollInterval {
		t.Fatalf("requeue_after = %s, want %s", result.RequeueAfter, workflowPollInterval)
	}
}

func TestReconcilerProjectsPrepareJobStatuses(t *testing.T) {
	ctx := context.Background()
	spec := validRunSpec()
	spec.RunId = "agent-run-1"
	spec.PrepareJobs = []*agentrunv1.AgentRunPrepareJob{
		{JobId: "auth", JobType: "auth"},
		{JobId: "rules", JobType: "rules"},
	}
	resource := newRunResource("agent-run-1", 5, spec)
	resource.Status = platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: 5,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), "AgentRun is accepted.", 5, time.Date(2026, 4, 14, 1, 2, 3, 0, time.UTC)),
				newCondition(platformcontract.AgentRunConditionTypeWorkloadReady, true, string(platformcontract.AgentRunConditionReasonWorkloadCreated), "AgentRun workflow submitted.", 5, time.Date(2026, 4, 14, 1, 2, 4, 0, time.UTC)),
			},
		},
		Phase:      platformv1alpha1.AgentRunResourcePhaseScheduled,
		Message:    "AgentRun workflow submitted.",
		WorkloadID: "agent-run-1",
		UpdatedAt:  timePtr(time.Date(2026, 4, 14, 1, 2, 4, 0, time.UTC)),
	}
	client := newClient(resource)
	startedAt := time.Date(2026, 4, 14, 1, 2, 5, 0, time.UTC)
	finishedAt := time.Date(2026, 4, 14, 1, 2, 6, 0, time.UTC)
	reconciler := newTestReconciler(t, client)
	reconciler.workflowRuntime = &fakeWorkflowRuntime{states: []*WorkflowState{{
		Phase: "Running",
		Nodes: []workflows.NodeState{
			{DisplayName: "prepare-01-auth", Phase: "Succeeded", Message: "auth ready", StartedAt: &startedAt, FinishedAt: &finishedAt},
			{DisplayName: "prepare-02-rules", Phase: "Running", Message: "rules loading", StartedAt: &startedAt},
		},
	}}}

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getRunResource(t, ctx, client, resource.Name)
	if got, want := len(updated.Status.PrepareJobs), 2; got != want {
		t.Fatalf("prepare jobs = %d, want %d", got, want)
	}
	if got := updated.Status.PrepareJobs[0].GetPhase(); got != agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_SUCCEEDED {
		t.Fatalf("prepare job 0 phase = %s", got)
	}
	if got := updated.Status.PrepareJobs[1].GetPhase(); got != agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_RUNNING {
		t.Fatalf("prepare job 1 phase = %s", got)
	}
	state, err := runStateFromResource(updated)
	if err != nil {
		t.Fatalf("runStateFromResource: %v", err)
	}
	if got := state.GetStatus().GetPrepareJobs()[0].GetMessage(); got != "auth ready" {
		t.Fatalf("proto prepare job message = %q, want auth ready", got)
	}
}

func TestReconcilerRejectsInvalidRun(t *testing.T) {
	ctx := context.Background()
	spec := validRunSpec()
	spec.SessionId = ""
	resource := newRunResource("agent-run-1", 5, spec)
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getRunResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentRunResourcePhaseFailed {
		t.Fatalf("phase = %q, want FAILED", updated.Status.Phase)
	}
	assertCondition(t, updated.Status.Conditions, string(platformcontract.AgentRunConditionTypeAccepted), string(metav1.ConditionFalse))
}

func TestReconcilerRecordsStartedAndFinishedTimelineTransitions(t *testing.T) {
	ctx := context.Background()
	resource := newRunResource("agent-run-1", 5, validRunSpec())
	now := time.Date(2026, 4, 14, 1, 2, 3, 0, time.UTC)
	resource.Status = platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: 5,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), "AgentRun is accepted.", 5, now),
				newCondition(platformcontract.AgentRunConditionTypeWorkloadReady, true, string(platformcontract.AgentRunConditionReasonWorkloadCreated), "AgentRun workflow submitted.", 5, now.Add(time.Second)),
			},
		},
		Phase:      platformv1alpha1.AgentRunResourcePhaseScheduled,
		Message:    "AgentRun workflow submitted.",
		WorkloadID: "agent-run-1",
		UpdatedAt:  timePtr(now.Add(time.Second)),
	}
	client := newClient(resource)
	runtime := &fakeWorkflowRuntime{
		states: []*WorkflowState{
			{Phase: "Running", Message: "running"},
			{Phase: "Succeeded", Message: "done"},
		},
	}
	reconciler := newTestReconciler(t, client)
	reconciler.workflowRuntime = runtime
	sink := &fakeTimelineSink{}
	reconciler.SetTimelineSink(sink)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("running reconcile: %v", err)
	}
	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("terminal reconcile: %v", err)
	}

	updated := getRunResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentRunResourcePhaseSucceeded {
		t.Fatalf("phase = %q, want SUCCEEDED", updated.Status.Phase)
	}
	if len(sink.events) != 2 {
		t.Fatalf("events = %d, want 2", len(sink.events))
	}
	if sink.events[0].EventType != "STARTED" {
		t.Fatalf("first event = %q, want STARTED", sink.events[0].EventType)
	}
	if sink.events[1].EventType != "FINISHED" {
		t.Fatalf("second event = %q, want FINISHED", sink.events[1].EventType)
	}
	if len(sink.intervals) != 1 {
		t.Fatalf("intervals = %d, want 1", len(sink.intervals))
	}
	if sink.intervals[0].Stage != "EXECUTE" {
		t.Fatalf("interval stage = %q, want EXECUTE", sink.intervals[0].Stage)
	}
	if sink.intervals[0].Status != platformcontract.TimelineStageStatusSucceeded {
		t.Fatalf("interval status = %q, want SUCCEEDED", sink.intervals[0].Status)
	}
}

func TestReconcilerPreservesTerminalResult(t *testing.T) {
	ctx := context.Background()
	resource := newRunResource("run-1", 5, validRunSpec())
	resource.Status = platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: 5,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), "AgentRun is accepted.", 5, time.Date(2026, 4, 14, 1, 2, 3, 0, time.UTC)),
				newCondition(platformcontract.AgentRunConditionTypeWorkloadReady, true, string(platformcontract.AgentRunConditionReasonWorkloadCreated), "AgentRun workflow submitted.", 5, time.Date(2026, 4, 14, 1, 2, 4, 0, time.UTC)),
			},
		},
		Phase:      platformv1alpha1.AgentRunResourcePhaseScheduled,
		Message:    "AgentRun workflow submitted.",
		WorkloadID: "run-1",
		ResultSummary: &platformv1alpha1.AgentRunResultSummary{
			Status: "RUN_STATUS_COMPLETED",
		},
		UpdatedAt: timePtr(time.Date(2026, 4, 14, 1, 2, 4, 0, time.UTC)),
	}
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)
	reconciler.workflowRuntime = &fakeWorkflowRuntime{
		states: []*WorkflowState{{Phase: "Succeeded", Message: "done"}},
	}

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getRunResource(t, ctx, client, resource.Name)
	if updated.Status.ResultSummary == nil || updated.Status.ResultSummary.Status != "RUN_STATUS_COMPLETED" {
		t.Fatalf("result_summary = %+v, want completed", updated.Status.ResultSummary)
	}
}

func TestReconcilerReleasesSessionSlotOnTerminal(t *testing.T) {
	ctx := context.Background()
	run := newRunResource("run-1", 5, validRunSpec())
	run.Status = platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: 5,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), "AgentRun is accepted.", 5, time.Date(2026, 4, 14, 1, 2, 3, 0, time.UTC)),
				newCondition(platformcontract.AgentRunConditionTypeWorkloadReady, true, string(platformcontract.AgentRunConditionReasonWorkloadCreated), "AgentRun workflow submitted.", 5, time.Date(2026, 4, 14, 1, 2, 4, 0, time.UTC)),
			},
		},
		Phase:      platformv1alpha1.AgentRunResourcePhaseScheduled,
		Message:    "AgentRun workflow submitted.",
		WorkloadID: "run-1",
		UpdatedAt:  timePtr(time.Date(2026, 4, 14, 1, 2, 4, 0, time.UTC)),
	}
	session := &platformv1alpha1.AgentSessionResource{
		ObjectMeta: metav1.ObjectMeta{Name: "session-1", Namespace: "code-code"},
		Spec: platformv1alpha1.AgentSessionResourceSpec{
			Session: &agentsessionv1.AgentSessionSpec{SessionId: "session-1"},
		},
		Status: platformv1alpha1.AgentSessionResourceStatus{
			ActiveRunID: "run-1",
			Phase:       platformv1alpha1.AgentSessionResourcePhaseRunning,
		},
	}
	client := newClient(run, session)
	reconciler := newTestReconciler(t, client)
	reconciler.workflowRuntime = &fakeWorkflowRuntime{
		states: []*WorkflowState{{Phase: "Succeeded", Message: "done"}},
	}

	if _, err := reconciler.Reconcile(ctx, requestFor(run)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updatedSession := &platformv1alpha1.AgentSessionResource{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code", Name: "session-1"}, updatedSession); err != nil {
		t.Fatalf("get session: %v", err)
	}
	if updatedSession.Status.ActiveRunID != "" {
		t.Fatalf("active_run_id = %q, want empty", updatedSession.Status.ActiveRunID)
	}
}

func TestReconcilerCancelsRequestedRunWithoutWorkload(t *testing.T) {
	ctx := context.Background()
	resource := newRunResource("agent-run-1", 5, validRunSpec())
	resource.Spec.Run.CancelRequested = true
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getRunResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentRunResourcePhaseCanceled {
		t.Fatalf("phase = %q, want CANCELED", updated.Status.Phase)
	}
}

func TestReconcilerRequestsWorkflowCancel(t *testing.T) {
	ctx := context.Background()
	resource := newRunResource("agent-run-1", 5, validRunSpec())
	resource.Spec.Run.CancelRequested = true
	resource.Status = platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: 5,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), "AgentRun is accepted.", 5, time.Date(2026, 4, 14, 1, 2, 3, 0, time.UTC)),
				newCondition(platformcontract.AgentRunConditionTypeWorkloadReady, true, string(platformcontract.AgentRunConditionReasonWorkloadCreated), "AgentRun workflow submitted.", 5, time.Date(2026, 4, 14, 1, 2, 4, 0, time.UTC)),
			},
		},
		Phase:      platformv1alpha1.AgentRunResourcePhaseRunning,
		Message:    "running",
		WorkloadID: "agent-run-1",
		UpdatedAt:  timePtr(time.Date(2026, 4, 14, 1, 2, 4, 0, time.UTC)),
	}
	client := newClient(resource)
	runtime := &fakeWorkflowRuntime{
		states: []*WorkflowState{{Phase: "Running", Message: "still stopping"}},
	}
	reconciler := newTestReconciler(t, client)
	reconciler.workflowRuntime = runtime

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getRunResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentRunResourcePhaseRunning {
		t.Fatalf("phase = %q, want RUNNING", updated.Status.Phase)
	}
	if len(runtime.canceled) != 1 || runtime.canceled[0] != "agent-run-1" {
		t.Fatalf("canceled = %+v, want agent-run-1", runtime.canceled)
	}
}

func TestReconcilerDoesNotResubmitMissingWorkflowAfterCancel(t *testing.T) {
	ctx := context.Background()
	resource := newRunResource("agent-run-1", 5, validRunSpec())
	resource.Spec.Run.CancelRequested = true
	resource.Status = platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: 5,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), "AgentRun is accepted.", 5, time.Date(2026, 4, 14, 1, 2, 3, 0, time.UTC)),
				newCondition(platformcontract.AgentRunConditionTypeWorkloadReady, true, string(platformcontract.AgentRunConditionReasonWorkloadCreated), "AgentRun workflow submitted.", 5, time.Date(2026, 4, 14, 1, 2, 4, 0, time.UTC)),
			},
		},
		Phase:      platformv1alpha1.AgentRunResourcePhaseScheduled,
		Message:    "submitted",
		WorkloadID: "agent-run-1",
		UpdatedAt:  timePtr(time.Date(2026, 4, 14, 1, 2, 4, 0, time.UTC)),
	}
	client := newClient(resource)
	runtime := &fakeWorkflowRuntime{
		cancelErr: apierrors.NewNotFound(schema.GroupResource{Group: "temporal.io", Resource: "workflows"}, "agent-run-1"),
	}
	reconciler := newTestReconciler(t, client)
	reconciler.workflowRuntime = runtime

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	updated := getRunResource(t, ctx, client, resource.Name)
	if updated.Status.Phase != platformv1alpha1.AgentRunResourcePhaseCanceled {
		t.Fatalf("phase = %q, want CANCELED", updated.Status.Phase)
	}
	if len(runtime.submitted) != 0 {
		t.Fatalf("submitted = %d, want 0", len(runtime.submitted))
	}
}

func TestReconcilerSkipsUnchangedStatusUpdate(t *testing.T) {
	ctx := context.Background()
	resource := newRunResource("agent-run-1", 5, validRunSpec())
	client := newClient(resource)
	reconciler := newTestReconciler(t, client)

	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	second := getRunResource(t, ctx, client, resource.Name)
	secondUpdatedAt := second.Status.UpdatedAt

	reconciler.now = func() time.Time {
		return time.Date(2026, 4, 14, 2, 3, 4, 0, time.UTC)
	}
	if _, err := reconciler.Reconcile(ctx, requestFor(resource)); err != nil {
		t.Fatalf("third reconcile: %v", err)
	}

	third := getRunResource(t, ctx, client, resource.Name)
	if !third.Status.UpdatedAt.Equal(secondUpdatedAt) {
		t.Fatalf("updatedAt changed: %v -> %v", secondUpdatedAt, third.Status.UpdatedAt)
	}
}

func newTestReconciler(t *testing.T, client ctrlclient.Client) *Reconciler {
	t.Helper()
	reconciler, err := NewReconciler(ReconcilerConfig{
		Client:          client,
		Namespace:       "code-code",
		WorkflowRuntime: &fakeWorkflowRuntime{},
		Slots:           newFakeActiveRunSlots(client),
		Now: func() time.Time {
			return time.Date(2026, 4, 14, 1, 2, 3, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new reconciler: %v", err)
	}
	return reconciler
}

func newClient(objects ...ctrlclient.Object) ctrlclient.Client {
	return ctrlclientfake.NewClientBuilder().
		WithScheme(testutil.NewScheme()).
		WithObjects(objects...).
		WithStatusSubresource(&platformv1alpha1.AgentRunResource{}, &platformv1alpha1.AgentSessionResource{}).
		Build()
}

func requestFor(resource *platformv1alpha1.AgentRunResource) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Namespace: resource.Namespace, Name: resource.Name}}
}

func getRunResource(t *testing.T, ctx context.Context, client ctrlclient.Client, name string) *platformv1alpha1.AgentRunResource {
	t.Helper()
	resource := &platformv1alpha1.AgentRunResource{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: "code-code", Name: name}, resource); err != nil {
		t.Fatalf("get run: %v", err)
	}
	return resource
}

func newRunResource(name string, generation int64, spec *agentrunv1.AgentRunSpec) *platformv1alpha1.AgentRunResource {
	return &platformv1alpha1.AgentRunResource{
		TypeMeta: metav1.TypeMeta{APIVersion: platformv1alpha1.GroupVersion.String(), Kind: platformv1alpha1.KindAgentRunResource},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "code-code",
			Generation: generation,
		},
		Spec: platformv1alpha1.AgentRunResourceSpec{Run: spec},
	}
}

func validRunSpec() *agentrunv1.AgentRunSpec {
	return &agentrunv1.AgentRunSpec{
		RunId:                    "run-1",
		SessionId:                "session-1",
		SessionGeneration:        1,
		RuntimeConfigGeneration:  1,
		ResourceConfigGeneration: 1,
		StateGeneration:          1,
		ProviderId:               "codex",
		ExecutionClass:           "default",
		ContainerImage:           "ghcr.io/openai/codex:latest",
		CpuRequest:               "1000m",
		MemoryRequest:            "2Gi",
		AuthRequirement: &agentrunv1.AgentRunAuthRequirement{
			ProviderId:               "codex",
			ProviderSurfaceBindingId: "openai-default",
			AuthStatus:               "bound",
			RuntimeUrl:               "https://api.openai.com/v1",
			MaterializationKey:       "codex.openai-api-key",
		},
		RuntimeEnvironment: &corev1.RuntimeEnvironment{
			WorkspaceDir: "/workspace",
			DataDir:      "/home/agent",
		},
		WorkspaceId: "workspace-1",
		HomeStateId: "home-1",
		Request: &corev1.RunRequest{
			RunId: "run-1",
			Input: &inputv1.RunInput{
				Text:       "hello",
				Parameters: mustStruct(map[string]any{"model": "gpt-5"}),
			},
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

type fakeWorkflowRuntime struct {
	submitted []*platformv1alpha1.AgentRunResource
	canceled  []string
	states    []*WorkflowState
	getErr    error
	cancelErr error
	deleted   []string
	cleaned   []string
}

func (f *fakeWorkflowRuntime) Submit(_ context.Context, run *platformv1alpha1.AgentRunResource) (string, error) {
	f.submitted = append(f.submitted, run.DeepCopy())
	return workflowNameFor(run), nil
}

func (f *fakeWorkflowRuntime) Get(context.Context, string) (*WorkflowState, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if len(f.states) == 0 {
		return &WorkflowState{Phase: "Pending"}, nil
	}
	state := f.states[0]
	if len(f.states) > 1 {
		f.states = f.states[1:]
	}
	if state.StartedAt == nil {
		startedAt := time.Date(2026, 4, 14, 1, 2, 5, 0, time.UTC)
		state.StartedAt = &startedAt
	}
	if strings.EqualFold(state.Phase, "Succeeded") && state.FinishedAt == nil {
		finishedAt := time.Date(2026, 4, 14, 1, 2, 8, 0, time.UTC)
		state.FinishedAt = &finishedAt
	}
	return state, nil
}

func (f *fakeWorkflowRuntime) Cancel(_ context.Context, name string) error {
	if f.cancelErr != nil {
		return f.cancelErr
	}
	f.canceled = append(f.canceled, name)
	return nil
}

func (f *fakeWorkflowRuntime) Delete(_ context.Context, name string) error {
	f.deleted = append(f.deleted, name)
	return nil
}

func (f *fakeWorkflowRuntime) Cleanup(_ context.Context, run *platformv1alpha1.AgentRunResource) error {
	f.cleaned = append(f.cleaned, run.Spec.Run.GetRunId())
	return nil
}

var _ WorkflowRuntime = (*fakeWorkflowRuntime)(nil)
