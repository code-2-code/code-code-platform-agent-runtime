package agentruns

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/timeline"
	"code-code.internal/platform-k8s/internal/platform/resourceops"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	workflowPollInterval     = 3 * time.Second
	agentRunCleanupFinalizer = "agentrun.code-code.internal/runtime-cleanup"
)

// Reconciler reconciles AgentRunResource execution summary status.
type Reconciler struct {
	client          ctrlclient.Client
	namespace       string
	workflowRuntime WorkflowRuntime
	logger          *slog.Logger
	now             func() time.Time
	sink            timeline.Sink
	slots           activeRunSlotManager
}

// ReconcilerConfig groups AgentRun reconciler dependencies.
type ReconcilerConfig struct {
	Client          ctrlclient.Client
	Namespace       string
	WorkflowRuntime WorkflowRuntime
	Slots           activeRunSlotManager
	Logger          *slog.Logger
	Now             func() time.Time
}

// NewReconciler creates one AgentRun reconciler.
func NewReconciler(config ReconcilerConfig) (*Reconciler, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("agentruns: reconciler client is nil")
	}
	if strings.TrimSpace(config.Namespace) == "" {
		return nil, fmt.Errorf("agentruns: reconciler namespace is empty")
	}
	if config.WorkflowRuntime == nil {
		return nil, fmt.Errorf("agentruns: reconciler workflow runtime is nil")
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Slots == nil {
		return nil, fmt.Errorf("agentruns: active run slot manager is nil")
	}
	return &Reconciler{
		client:          config.Client,
		namespace:       strings.TrimSpace(config.Namespace),
		workflowRuntime: config.WorkflowRuntime,
		logger:          config.Logger,
		now:             config.Now,
		slots:           config.Slots,
	}, nil
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.AgentRunResource{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

// SetTimelineSink wires one optional timeline sink.
func (r *Reconciler) SetTimelineSink(sink timeline.Sink) {
	if r == nil {
		return
	}
	r.sink = sink
}

// Reconcile updates AgentRunResource observed summary state.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	if request.Namespace != r.namespace {
		return ctrl.Result{}, nil
	}

	resource := &platformv1alpha1.AgentRunResource{}
	if err := r.client.Get(ctx, request.NamespacedName, resource); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if resource.DeletionTimestamp != nil {
		return r.reconcileDeletedRun(ctx, resource)
	}
	if !controllerutil.ContainsFinalizer(resource, agentRunCleanupFinalizer) {
		if err := resourceops.UpdateResource(ctx, r.client, request.NamespacedName, func(current *platformv1alpha1.AgentRunResource) error {
			controllerutil.AddFinalizer(current, agentRunCleanupFinalizer)
			return nil
		}, func() *platformv1alpha1.AgentRunResource {
			return &platformv1alpha1.AgentRunResource{}
		}); err != nil {
			return ctrl.Result{}, err
		}
		controllerutil.AddFinalizer(resource, agentRunCleanupFinalizer)
	}

	previous := resource.Status.DeepCopy()
	now := r.now().UTC()
	if invalidStatus, ok := deriveInvalidStatus(resource, now); ok {
		return r.updateObservedStatus(ctx, request, resource, previous, invalidStatus, nil, ctrl.Result{})
	}
	if previous.Phase == "" {
		if resource.Spec.Run.GetCancelRequested() {
			return r.reconcileCanceledRun(ctx, request, resource, previous, now)
		}
		return r.updateObservedStatus(ctx, request, resource, previous, pendingStatus(resource, now), nil, ctrl.Result{Requeue: true})
	}
	if isTerminalPhase(previous.Phase) && previous.ObservedGeneration == resource.Generation {
		if err := r.workflowRuntime.Cleanup(ctx, resource); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if resource.Spec.Run.GetCancelRequested() {
		return r.reconcileCanceledRun(ctx, request, resource, previous, now)
	}
	if strings.TrimSpace(previous.WorkloadID) == "" {
		workloadID, err := r.workflowRuntime.Submit(ctx, resource)
		if err != nil {
			return ctrl.Result{}, err
		}
		next := scheduledStatus(resource, workloadID, now)
		return r.updateObservedStatus(ctx, request, resource, previous, next, nil, pollResultForPhase(next.Phase))
	}
	workflowState, err := r.workflowRuntime.Get(ctx, strings.TrimSpace(previous.WorkloadID))
	if err != nil {
		if apierrors.IsNotFound(err) {
			workloadID, submitErr := r.workflowRuntime.Submit(ctx, resource)
			if submitErr != nil {
				return ctrl.Result{}, submitErr
			}
			next := scheduledStatus(resource, workloadID, now)
			return r.updateObservedStatus(ctx, request, resource, previous, next, nil, pollResultForPhase(next.Phase))
		}
		return ctrl.Result{}, err
	}
	next := observedWorkflowStatus(resource, strings.TrimSpace(previous.WorkloadID), workflowState, now)
	return r.updateObservedStatus(ctx, request, resource, previous, next, workflowState, pollResultForPhase(next.Phase))
}

func (r *Reconciler) updateObservedStatus(ctx context.Context, request ctrl.Request, resource *platformv1alpha1.AgentRunResource, previous *platformv1alpha1.AgentRunResourceStatus, next *platformv1alpha1.AgentRunResourceStatus, workflowState *WorkflowState, result ctrl.Result) (ctrl.Result, error) {
	if statusSemanticallyEqual(previous, next) {
		return result, nil
	}
	if next != nil && isTerminalPhase(next.Phase) && (previous == nil || !isTerminalPhase(previous.Phase)) {
		if err := r.workflowRuntime.Cleanup(ctx, resource); err != nil {
			return ctrl.Result{}, err
		}
	}
	if err := updateStatus(ctx, r.client, request.NamespacedName, next); err != nil {
		r.logger.Error("agentRun status update failed", "name", request.NamespacedName.String(), "error", err)
		return ctrl.Result{}, err
	}
	if err := r.releaseSessionSlot(ctx, resource, previous, next); err != nil {
		r.logger.Error("agentRun session slot release failed", "name", request.NamespacedName.String(), "error", err)
		return ctrl.Result{}, err
	}
	r.recordTimelineTransitions(ctx, runTimelineTransitions(resource, previous, next, workflowState))
	return result, nil
}

func (r *Reconciler) reconcileDeletedRun(ctx context.Context, resource *platformv1alpha1.AgentRunResource) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(resource, agentRunCleanupFinalizer) {
		return ctrl.Result{}, nil
	}
	workloadID := strings.TrimSpace(resource.Status.WorkloadID)
	if workloadID != "" {
		workflowState, err := r.workflowRuntime.Get(ctx, workloadID)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		} else if !isTerminalWorkflowState(workflowState) {
			if err := r.workflowRuntime.Cancel(ctx, workloadID); err != nil {
				if !apierrors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{RequeueAfter: workflowPollInterval}, nil
		} else if err := r.workflowRuntime.Delete(ctx, workloadID); err != nil {
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		}
	}
	if err := r.workflowRuntime.Cleanup(ctx, resource); err != nil {
		return ctrl.Result{}, err
	}
	if err := resourceops.UpdateResource(ctx, r.client, ctrlclient.ObjectKeyFromObject(resource), func(current *platformv1alpha1.AgentRunResource) error {
		controllerutil.RemoveFinalizer(current, agentRunCleanupFinalizer)
		return nil
	}, func() *platformv1alpha1.AgentRunResource {
		return &platformv1alpha1.AgentRunResource{}
	}); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func pollResultForPhase(phase platformv1alpha1.AgentRunResourcePhase) ctrl.Result {
	switch phase {
	case platformv1alpha1.AgentRunResourcePhaseScheduled, platformv1alpha1.AgentRunResourcePhaseRunning:
		return ctrl.Result{RequeueAfter: workflowPollInterval}
	default:
		return ctrl.Result{}
	}
}

func (r *Reconciler) releaseSessionSlot(ctx context.Context, resource *platformv1alpha1.AgentRunResource, previous *platformv1alpha1.AgentRunResourceStatus, next *platformv1alpha1.AgentRunResourceStatus) error {
	if r == nil || r.slots == nil || resource == nil || resource.Spec.Run == nil || next == nil {
		return nil
	}
	if !isTerminalPhase(next.Phase) {
		return nil
	}
	if previous != nil && isTerminalPhase(previous.Phase) {
		return nil
	}
	_, err := r.slots.Release(ctx, resource.Spec.Run.GetSessionId(), resource.Spec.Run.GetRunId())
	return err
}

func timePtr(value time.Time) *metav1.Time {
	out := metav1.NewTime(value)
	return &out
}

func (r *Reconciler) recordTimelineTransitions(ctx context.Context, transitions timelineTransitions) {
	if r == nil || r.sink == nil {
		return
	}
	for _, interval := range transitions.intervals {
		if err := r.sink.RecordStageInterval(ctx, interval); err != nil {
			r.logger.Error("agentRun timeline interval record failed", "error", err, "stage", interval.Stage)
		}
	}
	for _, event := range transitions.events {
		if err := r.sink.RecordEvent(ctx, event); err != nil {
			r.logger.Error("agentRun timeline event record failed", "error", err, "eventType", event.EventType)
		}
	}
}

func deriveInvalidStatus(resource *platformv1alpha1.AgentRunResource, now time.Time) (*platformv1alpha1.AgentRunResourceStatus, bool) {
	if resource == nil {
		return failedStatus(0, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun resource is required."), true
	}
	if resource.Spec.Run == nil {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun spec.run is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetRunId()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun runId is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetSessionId()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun sessionId is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetExecutionClass()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun executionClass is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetProviderId()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun providerId is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetContainerImage()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun containerImage is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetCpuRequest()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun cpuRequest is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetMemoryRequest()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun memoryRequest is required."), true
	}
	if resource.Spec.Run.GetAuthRequirement() == nil {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun authRequirement is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetAuthRequirement().GetProviderSurfaceBindingId()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun authRequirement.providerSurfaceBindingId is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetAuthRequirement().GetProviderId()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun authRequirement.providerId is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetAuthRequirement().GetRuntimeUrl()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun authRequirement.runtimeUrl is required."), true
	}
	if strings.TrimSpace(resource.Spec.Run.GetAuthRequirement().GetMaterializationKey()) == "" {
		return failedStatus(resource.Generation, now, string(platformcontract.AgentRunConditionReasonInvalidSpec), "AgentRun authRequirement.materializationKey is required."), true
	}
	return nil, false
}

func pendingStatus(resource *platformv1alpha1.AgentRunResource, now time.Time) *platformv1alpha1.AgentRunResourceStatus {
	generation := resource.Generation
	message := "AgentRun is accepted."
	return &platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: generation,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), message, generation, now),
			},
		},
		Phase:       platformv1alpha1.AgentRunResourcePhasePending,
		Message:     message,
		PrepareJobs: prepareJobStatuses(resource, nil, agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_PENDING),
		UpdatedAt:   timePtr(now),
	}
}

func scheduledStatus(resource *platformv1alpha1.AgentRunResource, workloadID string, now time.Time) *platformv1alpha1.AgentRunResourceStatus {
	generation := resource.Generation
	message := "AgentRun workflow submitted."
	return &platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: generation,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), "AgentRun is accepted.", generation, now),
				newCondition(platformcontract.AgentRunConditionTypeWorkloadReady, true, string(platformcontract.AgentRunConditionReasonWorkloadCreated), message, generation, now),
			},
		},
		Phase:       platformv1alpha1.AgentRunResourcePhaseScheduled,
		Message:     message,
		WorkloadID:  workloadID,
		PrepareJobs: prepareJobStatuses(resource, nil, agentrunv1.AgentRunPrepareJobPhase_AGENT_RUN_PREPARE_JOB_PHASE_PENDING),
		UpdatedAt:   timePtr(now),
	}
}

func observedWorkflowStatus(resource *platformv1alpha1.AgentRunResource, workloadID string, workflowState *WorkflowState, now time.Time) *platformv1alpha1.AgentRunResourceStatus {
	generation := resource.Generation
	if workflowState == nil {
		return scheduledStatus(resource, workloadID, now)
	}
	message := strings.TrimSpace(workflowState.Message)
	prepareJobs := prepareJobStatuses(resource, workflowState, prepareFallbackPhase(workflowState.Phase))
	switch strings.ToLower(strings.TrimSpace(workflowState.Phase)) {
	case "running":
		if message == "" {
			message = "AgentRun is running."
		}
		return &platformv1alpha1.AgentRunResourceStatus{
			CommonStatusFields: platformv1alpha1.CommonStatusFields{
				ObservedGeneration: generation,
				Conditions: []metav1.Condition{
					newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), "AgentRun is accepted.", generation, now),
					newCondition(platformcontract.AgentRunConditionTypeWorkloadReady, true, string(platformcontract.AgentRunConditionReasonRunStarted), message, generation, now),
				},
			},
			Phase:       platformv1alpha1.AgentRunResourcePhaseRunning,
			Message:     message,
			WorkloadID:  workloadID,
			PrepareJobs: prepareJobs,
			UpdatedAt:   timePtr(now),
		}
	case "succeeded":
		if message == "" {
			message = "AgentRun completed successfully."
		}
		next := terminalWorkflowStatus(platformv1alpha1.AgentRunResourcePhaseSucceeded, generation, workloadID, string(platformcontract.AgentRunConditionReasonRunSucceeded), message, now)
		next.PrepareJobs = prepareJobs
		return next
	case "failed", "error":
		if message == "" {
			message = "AgentRun workflow failed."
		}
		next := terminalWorkflowStatus(platformv1alpha1.AgentRunResourcePhaseFailed, generation, workloadID, string(platformcontract.AgentRunConditionReasonRunFailed), message, now)
		next.PrepareJobs = prepareJobs
		return next
	case "cancelled", "canceled":
		if message == "" {
			message = "AgentRun workflow canceled."
		}
		next := terminalWorkflowStatus(platformv1alpha1.AgentRunResourcePhaseCanceled, generation, workloadID, string(platformcontract.AgentRunConditionReasonRunCanceled), message, now)
		next.PrepareJobs = prepareJobs
		return next
	default:
		return scheduledStatus(resource, workloadID, now)
	}
}

func terminalWorkflowStatus(phase platformv1alpha1.AgentRunResourcePhase, generation int64, workloadID string, reason string, message string, now time.Time) *platformv1alpha1.AgentRunResourceStatus {
	return &platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: generation,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, true, string(platformcontract.AgentRunConditionReasonAccepted), "AgentRun is accepted.", generation, now),
				newCondition(platformcontract.AgentRunConditionTypeCompleted, true, reason, message, generation, now),
			},
		},
		Phase:      phase,
		Message:    message,
		WorkloadID: workloadID,
		UpdatedAt:  timePtr(now),
	}
}

func failedStatus(generation int64, now time.Time, reason string, message string) *platformv1alpha1.AgentRunResourceStatus {
	return &platformv1alpha1.AgentRunResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: generation,
			Conditions: []metav1.Condition{
				newCondition(platformcontract.AgentRunConditionTypeAccepted, false, reason, message, generation, now),
			},
		},
		Phase:     platformv1alpha1.AgentRunResourcePhaseFailed,
		Message:   message,
		UpdatedAt: timePtr(now),
	}
}

func newCondition(conditionType platformcontract.AgentRunConditionType, accepted bool, reason string, message string, generation int64, now time.Time) metav1.Condition {
	status := metav1.ConditionFalse
	if accepted {
		status = metav1.ConditionTrue
	}
	return metav1.Condition{
		Type:               string(conditionType),
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.NewTime(now),
	}
}

func isTerminalPhase(phase platformv1alpha1.AgentRunResourcePhase) bool {
	switch phase {
	case platformv1alpha1.AgentRunResourcePhaseSucceeded,
		platformv1alpha1.AgentRunResourcePhaseFailed,
		platformv1alpha1.AgentRunResourcePhaseCanceled:
		return true
	default:
		return false
	}
}

func isTerminalWorkflowState(state *WorkflowState) bool {
	if state == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(state.Phase)) {
	case "succeeded", "failed", "error", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

type timelineTransitions struct {
	intervals []*platformcontract.StageInterval
	events    []*platformcontract.TimelineEvent
}

func runTimelineTransitions(resource *platformv1alpha1.AgentRunResource, previous *platformv1alpha1.AgentRunResourceStatus, next *platformv1alpha1.AgentRunResourceStatus, workflowState *WorkflowState) timelineTransitions {
	if resource == nil || resource.Spec.Run == nil || strings.TrimSpace(resource.Spec.Run.GetSessionId()) == "" || strings.TrimSpace(resource.Spec.Run.GetRunId()) == "" || next == nil {
		return timelineTransitions{}
	}
	scope := platformcontract.TimelineScopeRef{
		Scope:     platformcontract.TimelineScopeSession,
		SessionID: resource.Spec.Run.GetSessionId(),
	}
	attributes := map[string]string{
		"run_id": resource.Spec.Run.GetRunId(),
		"phase":  string(next.Phase),
	}
	if workloadID := strings.TrimSpace(next.WorkloadID); workloadID != "" {
		attributes["workload_id"] = workloadID
	}
	transitions := timelineTransitions{}
	if next.Phase == platformv1alpha1.AgentRunResourcePhaseScheduled && (previous == nil || previous.Phase != platformv1alpha1.AgentRunResourcePhaseScheduled) {
		transitions.events = append(transitions.events, &platformcontract.TimelineEvent{
			ScopeRef:   scope,
			EventType:  "SCHEDULED",
			Subject:    "run",
			Action:     "workflow",
			OccurredAt: eventTime(nil, next.UpdatedAt),
			Attributes: cloneAttributes(attributes),
		})
	}
	if next.Phase == platformv1alpha1.AgentRunResourcePhaseRunning && (previous == nil || previous.Phase != platformv1alpha1.AgentRunResourcePhaseRunning) {
		transitions.events = append(transitions.events, &platformcontract.TimelineEvent{
			ScopeRef:   scope,
			EventType:  "STARTED",
			Subject:    "run",
			Action:     "workflow",
			OccurredAt: eventTime(timeFromWorkflow(workflowState, true), next.UpdatedAt),
			Attributes: cloneAttributes(attributes),
		})
	}
	if isTerminalPhase(next.Phase) && (previous == nil || !isTerminalPhase(previous.Phase)) {
		transitions.events = append(transitions.events, &platformcontract.TimelineEvent{
			ScopeRef:   scope,
			EventType:  "FINISHED",
			Subject:    "run",
			Action:     "workflow",
			OccurredAt: eventTime(timeFromWorkflow(workflowState, false), next.UpdatedAt),
			Attributes: cloneAttributes(attributes),
		})
		if interval := executeStageInterval(scope, previous, next, workflowState, attributes); interval != nil {
			transitions.intervals = append(transitions.intervals, interval)
		}
	}
	return transitions
}

func executeStageInterval(scope platformcontract.TimelineScopeRef, previous *platformv1alpha1.AgentRunResourceStatus, next *platformv1alpha1.AgentRunResourceStatus, workflowState *WorkflowState, attributes map[string]string) *platformcontract.StageInterval {
	startedAt := timeFromWorkflow(workflowState, true)
	if startedAt == nil {
		startedAt = conditionTransitionTime(next, string(platformcontract.AgentRunConditionTypeWorkloadReady), string(platformcontract.AgentRunConditionReasonRunStarted))
	}
	if startedAt == nil {
		startedAt = conditionTransitionTime(previous, string(platformcontract.AgentRunConditionTypeWorkloadReady), string(platformcontract.AgentRunConditionReasonRunStarted))
	}
	endedAt := timeFromWorkflow(workflowState, false)
	if endedAt == nil {
		endedAt = timePtrValue(next.UpdatedAt)
	}
	if startedAt == nil || endedAt == nil || endedAt.Before(*startedAt) {
		return nil
	}
	return &platformcontract.StageInterval{
		ScopeRef:   scope,
		Stage:      "EXECUTE",
		Subject:    "run",
		Action:     "workflow",
		Status:     timelineStageStatusFor(next.Phase),
		StartedAt:  *startedAt,
		EndedAt:    endedAt,
		Attributes: cloneAttributes(attributes),
	}
}

func conditionTransitionTime(status *platformv1alpha1.AgentRunResourceStatus, conditionType string, reason string) *time.Time {
	if status == nil {
		return nil
	}
	for _, c := range status.Conditions {
		if c.Type != conditionType || c.Reason != reason {
			continue
		}
		if c.LastTransitionTime.IsZero() {
			continue
		}
		value := c.LastTransitionTime.UTC()
		return &value
	}
	return nil
}

func eventTime(workflowTime *time.Time, updatedAt *metav1.Time) time.Time {
	if workflowTime != nil {
		return workflowTime.UTC()
	}
	if updatedAt == nil {
		return time.Time{}
	}
	return updatedAt.UTC()
}

func timePtrValue(value *metav1.Time) *time.Time {
	if value == nil {
		return nil
	}
	out := value.UTC()
	return &out
}

func timeFromWorkflow(state *WorkflowState, started bool) *time.Time {
	if state == nil {
		return nil
	}
	if started {
		if state.StartedAt == nil {
			return nil
		}
		value := state.StartedAt.UTC()
		return &value
	}
	if state.FinishedAt == nil {
		return nil
	}
	value := state.FinishedAt.UTC()
	return &value
}

func timelineStageStatusFor(phase platformv1alpha1.AgentRunResourcePhase) platformcontract.TimelineStageStatus {
	switch phase {
	case platformv1alpha1.AgentRunResourcePhaseSucceeded:
		return platformcontract.TimelineStageStatusSucceeded
	case platformv1alpha1.AgentRunResourcePhaseCanceled:
		return platformcontract.TimelineStageStatusCanceled
	default:
		return platformcontract.TimelineStageStatusFailed
	}
}

func cloneAttributes(attributes map[string]string) map[string]string {
	if len(attributes) == 0 {
		return nil
	}
	out := make(map[string]string, len(attributes))
	for key, value := range attributes {
		out[key] = value
	}
	return out
}
