package sessionapi

import (
	"context"
	"fmt"
	"strings"

	domaineventv1 "code-code.internal/go-contract/platform/domain_event/v1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentruns"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessionactions"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessions"
	"code-code.internal/platform-k8s/internal/platform/domainevents"
	"github.com/jackc/pgx/v5/pgxpool"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *SessionServer) StartDomainEventConsumers(ctx context.Context, pool *pgxpool.Pool, natsURL string) error {
	if s == nil {
		return fmt.Errorf("platformk8s/sessionapi: server is nil")
	}
	if strings.TrimSpace(natsURL) == "" {
		return nil
	}
	if s.sessionRepository == nil {
		return fmt.Errorf("platformk8s/sessionapi: session repository is nil")
	}
	sessionReconciler, actionReconciler, _, err := s.domainReconcilers()
	if err != nil {
		return err
	}
	consumer, err := domainevents.NewConsumer(pool, domainevents.ConsumerConfig{
		NATSURL:     natsURL,
		ClientName:  "platform-agent-runtime-service-domain-consumer",
		DurableName: "platform-agent-runtime-service",
		FilterSubjects: []string{
			domainevents.SubjectPrefix + ".agent_session.>",
			domainevents.SubjectPrefix + ".agent_session_action.>",
		},
	}, func(eventCtx context.Context, event *domaineventv1.DomainEvent) error {
		return s.handleDomainEvent(eventCtx, event, sessionReconciler, actionReconciler)
	})
	if err != nil {
		return err
	}
	go func() {
		_ = consumer.Run(ctx)
	}()
	return nil
}

func (s *SessionServer) domainReconcilers() (*agentsessions.Reconciler, *agentsessionactions.Reconciler, *agentruns.Reconciler, error) {
	if s.sessionRepository == nil {
		return nil, nil, nil, fmt.Errorf("platformk8s/sessionapi: session repository is nil")
	}
	sessionReconciler, err := agentsessions.NewReconciler(agentsessions.ReconcilerConfig{
		Client:           s.client,
		ResourceClient:   s.runtimeClient,
		Namespace:        s.namespace,
		RuntimeNamespace: s.runtimeNamespace,
		ProfileSource:    s.profileSource,
		RuntimeCatalog:   s.runtimeCatalog,
		ModelRegistry:    s.modelRegistry,
		Sessions:         s.sessionRepository,
		Actions:          s.actionStore,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	sessionReconciler.SetTimelineSink(s.timelineSink)
	runService, err := agentruns.NewService(s.client, s.reader, s.namespace, s.timelineSink, agentruns.WithRuntimeNamespace(s.runtimeNamespace), agentruns.WithActiveRunSlots(s.activeRunSlots))
	if err != nil {
		return nil, nil, nil, err
	}
	actionReconciler, err := agentsessionactions.NewReconciler(agentsessionactions.ReconcilerConfig{
		Client:      s.client,
		Store:       s.actionStore,
		Sessions:    s.sessionRepository,
		Namespace:   s.namespace,
		Runs:        runService,
		RetryPolicy: s.actionRetryPolicy,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	actionReconciler.SetTimelineSink(s.timelineSink)
	if s.agentRunWorkflow == nil {
		return nil, nil, nil, fmt.Errorf("platformk8s/sessionapi: agent run workflow runtime is nil")
	}
	runReconciler, err := agentruns.NewReconciler(agentruns.ReconcilerConfig{
		Client:          s.client,
		Namespace:       s.namespace,
		WorkflowRuntime: s.agentRunWorkflow,
		Slots:           s.activeRunSlots,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	runReconciler.SetTimelineSink(s.timelineSink)
	return sessionReconciler, actionReconciler, runReconciler, nil
}

func (s *SessionServer) handleDomainEvent(
	ctx context.Context,
	event *domaineventv1.DomainEvent,
	sessionReconciler *agentsessions.Reconciler,
	actionReconciler *agentsessionactions.Reconciler,
) error {
	switch payload := event.GetPayload().(type) {
	case *domaineventv1.DomainEvent_AgentSession:
		sessionID := firstNonEmpty(payload.AgentSession.GetState().GetSpec().GetSessionId(), event.GetAggregateId())
		return s.reconcileSessionDomain(ctx, sessionID, sessionReconciler, actionReconciler)
	case *domaineventv1.DomainEvent_AgentSessionAction:
		actionID := firstNonEmpty(payload.AgentSessionAction.GetState().GetSpec().GetActionId(), event.GetAggregateId())
		sessionID := payload.AgentSessionAction.GetState().GetSpec().GetSessionId()
		return s.reconcileActionDomain(ctx, sessionID, actionID, sessionReconciler, actionReconciler)
	default:
		return nil
	}
}

func (s *SessionServer) reconcileSessionDomain(ctx context.Context, sessionID string, sessions *agentsessions.Reconciler, actions *agentsessionactions.Reconciler) error {
	result, err := sessions.Reconcile(ctx, ctrl.Request{NamespacedName: ctrlclient.ObjectKey{Namespace: s.namespace, Name: strings.TrimSpace(sessionID)}})
	if err != nil {
		return err
	}
	if err := s.scheduleSessionReconcile(ctx, result, sessionID); err != nil {
		return err
	}
	results, err := actions.ReconcileSessionActions(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.scheduleSessionActionsReconcile(ctx, results, sessionID)
}

func (s *SessionServer) reconcileActionDomain(ctx context.Context, sessionID, actionID string, sessions *agentsessions.Reconciler, actions *agentsessionactions.Reconciler) error {
	result, err := actions.Reconcile(ctx, ctrl.Request{NamespacedName: ctrlclient.ObjectKey{Namespace: s.namespace, Name: strings.TrimSpace(actionID)}})
	if err != nil {
		return err
	}
	if err := s.scheduleActionReconcile(ctx, result, sessionID, actionID); err != nil {
		return err
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	sessionResult, err := sessions.Reconcile(ctx, ctrl.Request{NamespacedName: ctrlclient.ObjectKey{Namespace: s.namespace, Name: strings.TrimSpace(sessionID)}})
	if err != nil {
		return err
	}
	return s.scheduleSessionReconcile(ctx, sessionResult, sessionID)
}

func (s *SessionServer) reconcileRunDomain(ctx context.Context, sessionID, runID string, sessions *agentsessions.Reconciler, actions *agentsessionactions.Reconciler, runs *agentruns.Reconciler) error {
	result, err := runs.Reconcile(ctx, ctrl.Request{NamespacedName: ctrlclient.ObjectKey{Namespace: s.namespace, Name: strings.TrimSpace(runID)}})
	if err != nil {
		return err
	}
	if err := s.scheduleRunReconcile(ctx, result, sessionID, runID); err != nil {
		return err
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	actionResults, err := actions.ReconcileRunActions(ctx, sessionID, runID)
	if err != nil {
		return err
	}
	if err := s.scheduleRunActionsReconcile(ctx, actionResults, sessionID, runID); err != nil {
		return err
	}
	sessionResult, err := sessions.Reconcile(ctx, ctrl.Request{NamespacedName: ctrlclient.ObjectKey{Namespace: s.namespace, Name: strings.TrimSpace(sessionID)}})
	if err != nil {
		return err
	}
	return s.scheduleSessionReconcile(ctx, sessionResult, sessionID)
}

func (s *SessionServer) scheduleSessionActionsReconcile(ctx context.Context, results []ctrl.Result, sessionID string) error {
	for _, result := range results {
		if err := s.scheduleReconcile(ctx, result, "reconcile-session-actions", map[string]string{"sessionId": strings.TrimSpace(sessionID)}, "session", sessionID); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionServer) scheduleRunActionsReconcile(ctx context.Context, results []ctrl.Result, sessionID string, runID string) error {
	for _, result := range results {
		if err := s.scheduleReconcile(ctx, result, "reconcile-run-actions", map[string]string{
			"sessionId": strings.TrimSpace(sessionID),
			"runId":     strings.TrimSpace(runID),
		}, "run", runID); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionServer) scheduleSessionReconcile(ctx context.Context, result ctrl.Result, sessionID string) error {
	return s.scheduleReconcile(ctx, result, "reconcile-session", map[string]string{"sessionId": strings.TrimSpace(sessionID)}, "session", sessionID)
}

func (s *SessionServer) scheduleActionReconcile(ctx context.Context, result ctrl.Result, sessionID string, actionID string) error {
	return s.scheduleReconcile(ctx, result, "reconcile-action", map[string]string{
		"sessionId": strings.TrimSpace(sessionID),
		"actionId":  strings.TrimSpace(actionID),
	}, "action", actionID)
}

func (s *SessionServer) scheduleRunReconcile(ctx context.Context, result ctrl.Result, sessionID string, runID string) error {
	return s.scheduleReconcile(ctx, result, "reconcile-run", map[string]string{
		"sessionId": strings.TrimSpace(sessionID),
		"runId":     strings.TrimSpace(runID),
	}, "run", runID)
}

func (s *SessionServer) scheduleReconcile(ctx context.Context, result ctrl.Result, action string, body any, ownerKind string, ownerID string) error {
	delay, ok := requeueDelay(result)
	if !ok || s.reconcileScheduler == nil {
		return nil
	}
	request, err := scheduleRequest(action, body, delay, ownerKind, ownerID)
	if err != nil {
		return err
	}
	return s.reconcileScheduler.ScheduleReconcile(ctx, request)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
