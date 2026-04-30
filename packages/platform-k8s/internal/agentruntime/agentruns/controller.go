package agentruns

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/timeline"
	"code-code.internal/platform-k8s/internal/platform/resourceops"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
