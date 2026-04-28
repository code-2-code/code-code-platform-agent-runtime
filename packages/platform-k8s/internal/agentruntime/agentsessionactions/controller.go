package agentsessionactions

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"code-code.internal/platform-k8s/internal/agentruntime/agentresourceconfig"
	"code-code.internal/platform-k8s/internal/agentruntime/agentruns"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessions"
	"code-code.internal/platform-k8s/internal/agentruntime/timeline"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Reconciler struct {
	client      ctrlclient.Client
	store       Store
	sessions    agentsessions.SessionRepository
	namespace   string
	logger      *slog.Logger
	now         func() time.Time
	runs        *agentruns.Service
	sink        timeline.Sink
	resources   *agentresourceconfig.Materializer
	retryPolicy RetryPolicy
}

type ReconcilerConfig struct {
	Client      ctrlclient.Client
	Store       Store
	Sessions    agentsessions.SessionRepository
	Namespace   string
	Logger      *slog.Logger
	Now         func() time.Time
	Runs        *agentruns.Service
	RetryPolicy *RetryPolicy
}

func NewReconciler(config ReconcilerConfig) (*Reconciler, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("agentsessionactions: reconciler client is nil")
	}
	if config.Store == nil {
		return nil, fmt.Errorf("agentsessionactions: reconciler store is nil")
	}
	if config.Sessions == nil {
		return nil, fmt.Errorf("agentsessionactions: session repository is nil")
	}
	if strings.TrimSpace(config.Namespace) == "" {
		return nil, fmt.Errorf("agentsessionactions: reconciler namespace is empty")
	}
	if config.Runs == nil {
		return nil, fmt.Errorf("agentsessionactions: reconciler run service is nil")
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	retryPolicy := DefaultRetryPolicy()
	if config.RetryPolicy != nil {
		retryPolicy = normalizeRetryPolicy(*config.RetryPolicy)
	}
	resources, err := agentresourceconfig.NewMaterializer(config.Client, config.Namespace)
	if err != nil {
		return nil, err
	}
	return &Reconciler{
		client:      config.Client,
		store:       config.Store,
		sessions:    config.Sessions,
		namespace:   strings.TrimSpace(config.Namespace),
		logger:      config.Logger,
		now:         config.Now,
		runs:        config.Runs,
		resources:   resources,
		retryPolicy: retryPolicy,
	}, nil
}

func (r *Reconciler) SetTimelineSink(sink timeline.Sink) {
	if r == nil {
		return
	}
	r.sink = sink
}

func (r *Reconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	if request.Namespace != r.namespace {
		return ctrl.Result{}, nil
	}
	resource, err := r.store.Get(ctx, request.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if resource.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}
	previous := resource.Status.DeepCopy()
	next, result, err := r.deriveStatus(ctx, resource)
	if err != nil {
		return result, err
	}
	if statusSemanticallyEqual(previous, next) {
		return result, nil
	}
	if _, err := r.store.UpdateStatus(ctx, request.Name, next); err != nil {
		r.logger.Error("agentSessionAction status update failed", "name", request.NamespacedName.String(), "error", err)
		return ctrl.Result{}, err
	}
	r.recordTimelineTransitions(ctx, actionTimelineTransitions(resource, previous, next))
	return result, nil
}

// ReconcileSessionActions reconciles all non-filtered actions in one session.
func (r *Reconciler) ReconcileSessionActions(ctx context.Context, sessionID string) ([]ctrl.Result, error) {
	items, err := listSessionActions(ctx, r.store, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	results := make([]ctrl.Result, 0, len(items))
	for i := range items {
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: ctrlclient.ObjectKey{Namespace: items[i].Namespace, Name: items[i].Name}})
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

// ReconcileRunActions reconciles actions that point to one run.
func (r *Reconciler) ReconcileRunActions(ctx context.Context, sessionID string, runID string) ([]ctrl.Result, error) {
	items, err := listSessionActions(ctx, r.store, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	results := []ctrl.Result{}
	for i := range items {
		if items[i].Name != runID && strings.TrimSpace(items[i].Status.RunID) != strings.TrimSpace(runID) {
			continue
		}
		result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: ctrlclient.ObjectKey{Namespace: items[i].Namespace, Name: items[i].Name}})
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}
