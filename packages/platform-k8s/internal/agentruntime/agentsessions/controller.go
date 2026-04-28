package agentsessions

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentexecution"
	"code-code.internal/platform-k8s/internal/agentruntime/agentresourceconfig"
	"code-code.internal/platform-k8s/internal/agentruntime/timeline"
	"code-code.internal/platform-k8s/internal/platform/resourceops"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const agentSessionCleanupFinalizer = "agentsession.code-code.internal/runtime-cleanup"
const agentSessionReadinessRequeueInterval = 5 * time.Second

// Reconciler reconciles AgentSessionResource readiness and status.
type Reconciler struct {
	client    ctrlclient.Client
	namespace string
	logger    *slog.Logger
	now       func() time.Time
	sink      timeline.Sink
	resolver  *agentexecution.Resolver
	projector *ProfileProjector
	resources *agentresourceconfig.Materializer
	carriers  *CarrierManager
	sessions  SessionRepository
	actions   SessionActionReader
}

type SessionActionReader interface {
	HasNonterminalResetWarmState(context.Context, string) (bool, error)
	ListBySession(context.Context, string) ([]platformv1alpha1.AgentSessionActionResource, error)
}

// ReconcilerConfig groups AgentSession reconciler dependencies.
type ReconcilerConfig struct {
	Client           ctrlclient.Client
	ResourceClient   ctrlclient.Client
	Namespace        string
	RuntimeNamespace string
	ProfileSource    ProfileProjectionSource
	RuntimeCatalog   agentexecution.RuntimeCatalog
	ModelRegistry    agentexecution.ModelRegistry
	Sessions         SessionRepository
	Actions          SessionActionReader
	Logger           *slog.Logger
	Now              func() time.Time
}

// NewReconciler creates one AgentSession reconciler.
func NewReconciler(config ReconcilerConfig) (*Reconciler, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("agentsessions: reconciler client is nil")
	}
	if config.Sessions == nil {
		return nil, fmt.Errorf("agentsessions: session repository is nil")
	}
	if strings.TrimSpace(config.Namespace) == "" {
		return nil, fmt.Errorf("agentsessions: reconciler namespace is empty")
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.ResourceClient == nil {
		config.ResourceClient = config.Client
	}
	resolver, err := agentexecution.NewResolver(config.RuntimeCatalog, config.ModelRegistry)
	if err != nil {
		return nil, err
	}
	projector, err := NewProfileProjector(config.ProfileSource)
	if err != nil {
		return nil, err
	}
	resources, err := agentresourceconfig.NewMaterializer(config.ResourceClient, config.Namespace)
	if err != nil {
		return nil, err
	}
	carriers, err := NewCarrierManager(config.ResourceClient, config.Namespace, config.RuntimeNamespace)
	if err != nil {
		return nil, err
	}
	carriers.actions = config.Actions
	return &Reconciler{
		client:    config.Client,
		namespace: strings.TrimSpace(config.Namespace),
		logger:    config.Logger,
		now:       config.Now,
		resolver:  resolver,
		projector: projector,
		resources: resources,
		carriers:  carriers,
		sessions:  config.Sessions,
		actions:   config.Actions,
	}, nil
}

// SetTimelineSink wires one optional timeline sink.
func (r *Reconciler) SetTimelineSink(sink timeline.Sink) {
	if r == nil {
		return
	}
	r.sink = sink
}

// Reconcile updates AgentSessionResource observed readiness state.
func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	if request.Namespace != r.namespace {
		return ctrl.Result{}, nil
	}

	state, err := r.sessions.Get(ctx, request.Name)
	if err != nil {
		if apierrors.IsNotFound(err) || status.Code(err) == codes.NotFound {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	resource, err := ResourceFromState(state, r.namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	if synced, err := r.syncProfileBackedSession(ctx, resource); err != nil {
		status := failedStatus(&resource.Status, resource.Generation, r.now().UTC(), "ProfileProjectionFailed", err.Error())
		if statusSemanticallyEqual(&resource.Status, status) {
			return ctrl.Result{}, nil
		}
		if err := r.updateStatus(ctx, resource, status); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	} else if synced {
		return ctrl.Result{Requeue: true}, nil
	}
	if r.carriers != nil {
		if err := r.carriers.Ensure(ctx, resource); err != nil {
			return ctrl.Result{}, err
		}
	}

	previous := resource.Status.DeepCopy()
	now := r.now().UTC()
	status := r.deriveStatus(ctx, resource, now)
	if err := r.ensureReloadSubjectActions(ctx, resource, r.evaluateReadiness(ctx, resource), metav1.NewTime(now)); err != nil {
		return ctrl.Result{}, err
	}
	if statusSemanticallyEqual(&resource.Status, status) {
		return readinessRequeueFor(status), nil
	}
	if err := r.updateStatus(ctx, resource, status); err != nil {
		r.logger.Error("agentSession status update failed", "name", request.NamespacedName.String(), "error", err)
		return ctrl.Result{}, err
	}
	r.recordTimelineEvents(ctx, sessionTimelineEvents(resource, previous, status, now))
	return readinessRequeueFor(status), nil
}

func readinessRequeueFor(status *platformv1alpha1.AgentSessionResourceStatus) ctrl.Result {
	if status == nil || status.Phase != platformv1alpha1.AgentSessionResourcePhasePending {
		return ctrl.Result{}
	}
	return ctrl.Result{RequeueAfter: agentSessionReadinessRequeueInterval}
}

func (r *Reconciler) reconcileDeletedSession(ctx context.Context, resource *platformv1alpha1.AgentSessionResource) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(resource, agentSessionCleanupFinalizer) {
		return ctrl.Result{}, nil
	}
	if r.carriers != nil {
		if err := r.carriers.Cleanup(ctx, resource); err != nil {
			return ctrl.Result{}, err
		}
	}
	if err := resourceops.UpdateResource(ctx, r.client, ctrlclient.ObjectKeyFromObject(resource), func(current *platformv1alpha1.AgentSessionResource) error {
		controllerutil.RemoveFinalizer(current, agentSessionCleanupFinalizer)
		return nil
	}, func() *platformv1alpha1.AgentSessionResource {
		return &platformv1alpha1.AgentSessionResource{}
	}); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func observedConfigGeneration(previous int64, ready bool, current int64) int64 {
	if ready {
		return current
	}
	return previous
}

func previousConfigGenerations(status *platformv1alpha1.AgentSessionResourceStatus) (int64, int64, int64) {
	if status == nil {
		return 0, 0, 0
	}
	return status.RuntimeConfigGeneration, status.ResourceConfigGeneration, status.StateGeneration
}

func failedStatus(previous *platformv1alpha1.AgentSessionResourceStatus, generation int64, now time.Time, reason string, message string) *platformv1alpha1.AgentSessionResourceStatus {
	conditions := readyForNextRunCondition(previousConditions(previous))
	setStatusCondition(&conditions, platformcontract.AgentSessionConditionTypeReadyForNextRun, metav1.ConditionFalse, reason, message, generation, now)
	return &platformv1alpha1.AgentSessionResourceStatus{
		CommonStatusFields: platformv1alpha1.CommonStatusFields{
			ObservedGeneration: generation,
			Conditions:         conditions,
		},
		Phase:     platformv1alpha1.AgentSessionResourcePhaseFailed,
		Message:   message,
		UpdatedAt: timePtr(now),
	}
}

func previousConditions(status *platformv1alpha1.AgentSessionResourceStatus) []metav1.Condition {
	if status == nil {
		return nil
	}
	return status.Conditions
}

func readyForNextRunCondition(conditions []metav1.Condition) []metav1.Condition {
	out := make([]metav1.Condition, 0, 1)
	for _, condition := range conditions {
		if condition.Type == string(platformcontract.AgentSessionConditionTypeReadyForNextRun) {
			out = append(out, condition)
			return out
		}
	}
	return out
}

func readyForNextRun(conditions []metav1.Condition) bool {
	condition := meta.FindStatusCondition(conditions, string(platformcontract.AgentSessionConditionTypeReadyForNextRun))
	return condition != nil && condition.Status == metav1.ConditionTrue
}

func timePtr(value time.Time) *metav1.Time {
	out := metav1.NewTime(value)
	return &out
}

func (r *Reconciler) recordTimelineEvents(ctx context.Context, events []*platformcontract.TimelineEvent) {
	if r == nil || r.sink == nil {
		return
	}
	for _, event := range events {
		if err := r.sink.RecordEvent(ctx, event); err != nil {
			r.logger.Error("agentSession timeline event record failed", "error", err, "eventType", event.EventType)
		}
	}
}

func sessionTimelineEvents(resource *platformv1alpha1.AgentSessionResource, previous *platformv1alpha1.AgentSessionResourceStatus, next *platformv1alpha1.AgentSessionResourceStatus, now time.Time) []*platformcontract.TimelineEvent {
	if resource == nil || resource.Spec.Session == nil || strings.TrimSpace(resource.Spec.Session.GetSessionId()) == "" || next == nil {
		return nil
	}
	isFirstReconcile := previous == nil || previous.Phase == ""
	scopeRef := platformcontract.TimelineScopeRef{
		Scope:     platformcontract.TimelineScopeSession,
		SessionID: resource.Spec.Session.GetSessionId(),
	}
	events := make([]*platformcontract.TimelineEvent, 0, 3)
	if isFirstReconcile {
		events = append(events, &platformcontract.TimelineEvent{
			ScopeRef:   scopeRef,
			EventType:  "CREATED",
			Subject:    "session",
			Action:     "reconcile",
			OccurredAt: now,
			Attributes: map[string]string{
				"phase": string(next.Phase),
			},
		})
	}
	if next.Phase == platformv1alpha1.AgentSessionResourcePhaseReady && (isFirstReconcile || previous.Phase != platformv1alpha1.AgentSessionResourcePhaseReady) {
		events = append(events, &platformcontract.TimelineEvent{
			ScopeRef:   scopeRef,
			EventType:  "READY",
			Subject:    "session",
			Action:     "reconcile",
			OccurredAt: now,
		})
	}
	if next.Phase == platformv1alpha1.AgentSessionResourcePhaseRunning && (previous == nil || previous.Phase != platformv1alpha1.AgentSessionResourcePhaseRunning) {
		attributes := map[string]string{}
		if runID := strings.TrimSpace(next.ActiveRunID); runID != "" {
			attributes["run_id"] = runID
		}
		events = append(events, &platformcontract.TimelineEvent{
			ScopeRef:   scopeRef,
			EventType:  "STARTED",
			Subject:    "run",
			Action:     "claim",
			OccurredAt: now,
			Attributes: attributes,
		})
	}
	return events
}
