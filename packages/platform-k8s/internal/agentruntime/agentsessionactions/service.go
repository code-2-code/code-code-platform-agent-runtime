package agentsessionactions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"code-code.internal/go-contract/domainerror"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentexecution"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessions"
	"code-code.internal/platform-k8s/internal/agentruntime/timeline"
	"code-code.internal/platform-k8s/internal/platform/resourcemeta"
	"google.golang.org/protobuf/proto"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Service struct {
	store     Store
	sessions  agentsessions.SessionRepository
	namespace string
	resolver  *agentexecution.Resolver
	sink      timeline.Sink
	now       func() time.Time
}

func NewService(store Store, sessions agentsessions.SessionRepository, namespace string, sink timeline.Sink, resolver *agentexecution.Resolver) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("platformk8s/agentsessionactions: store is nil")
	}
	if sessions == nil {
		return nil, fmt.Errorf("platformk8s/agentsessionactions: session repository is nil")
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return nil, fmt.Errorf("platformk8s/agentsessionactions: namespace is empty")
	}
	if resolver == nil {
		return nil, fmt.Errorf("platformk8s/agentsessionactions: execution resolver is nil")
	}
	return &Service{
		store:     store,
		sessions:  sessions,
		namespace: namespace,
		resolver:  resolver,
		sink:      sink,
		now:       time.Now,
	}, nil
}

func (s *Service) Get(ctx context.Context, actionID string) (*agentsessionactionv1.AgentSessionActionState, error) {
	resource, err := s.getActionResource(ctx, actionID)
	if err != nil {
		return nil, err
	}
	return actionStateFromResource(resource)
}

func (s *Service) Create(ctx context.Context, sessionID string, request *CreateRequest) (*agentsessionactionv1.AgentSessionActionState, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: session_id is required")
	}
	session, err := s.loadReadySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	snapshot, err := buildRunTurnSnapshot(ctx, s.resolver, session, request)
	if err != nil {
		return nil, err
	}
	actionID, err := resourcemeta.EnsureResourceID(strings.TrimSpace(request.ActionID), sessionID, "action")
	if err != nil {
		return nil, err
	}
	resource := &platformv1alpha1.AgentSessionActionResource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: platformv1alpha1.GroupVersion.String(),
			Kind:       platformv1alpha1.KindAgentSessionActionResource,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      actionID,
			Namespace: s.namespace,
			Labels:    actionLabels(sessionID, agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RUN_TURN),
		},
		Spec: platformv1alpha1.AgentSessionActionResourceSpec{
			Action: &agentsessionactionv1.AgentSessionActionSpec{
				ActionId:  actionID,
				SessionId: sessionID,
				Type:      agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RUN_TURN,
				TurnId:    normalizeTurnID(request.TurnID),
				InputSnapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot{
					Snapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot_RunTurn{
						RunTurn: snapshot,
					},
				},
			},
		},
	}
	if err := s.store.Create(ctx, resource); err != nil {
		if apierrors.IsAlreadyExists(err) || apierrors.IsConflict(err) {
			return nil, domainerror.NewAlreadyExists("platformk8s/agentsessionactions: action %q already exists", resource.Name)
		}
		return nil, err
	}
	s.recordTimelineEvent(ctx, sessionID, "SUBMITTED", "action", "create", map[string]string{
		"action_id": actionID,
		"type":      agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RUN_TURN.String(),
	})
	return actionStateFromResource(resource)
}

func (s *Service) loadReadySession(ctx context.Context, sessionID string) (*platformv1alpha1.AgentSessionResource, error) {
	state, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	resource, err := agentsessions.ResourceFromState(state, s.namespace)
	if err != nil {
		return nil, err
	}
	if resource.Spec.Session == nil {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: session %q is missing payload", sessionID)
	}
	condition := meta.FindStatusCondition(resource.Status.Conditions, string(platformcontract.AgentSessionConditionTypeReadyForNextRun))
	if condition == nil || condition.Status != metav1.ConditionTrue {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: session %q is not ready for next run", sessionID)
	}
	if condition.ObservedGeneration != resource.Generation {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: session %q readiness is stale", sessionID)
	}
	if resource.Status.RuntimeConfigGeneration != resource.Generation {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: session %q runtime config generation is stale", sessionID)
	}
	if resource.Status.ResourceConfigGeneration != resource.Generation {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: session %q resource config generation is stale", sessionID)
	}
	resetPending, err := s.store.HasNonterminalResetWarmState(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if resetPending {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: session %q warm state reset is in progress", sessionID)
	}
	return resource, nil
}

func (s *Service) recordTimelineEvent(ctx context.Context, sessionID string, eventType string, subject string, action string, attributes map[string]string) {
	if s == nil || s.sink == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	_ = s.sink.RecordEvent(ctx, &platformcontract.TimelineEvent{
		ScopeRef: platformcontract.TimelineScopeRef{
			Scope:     platformcontract.TimelineScopeSession,
			SessionID: strings.TrimSpace(sessionID),
		},
		EventType:  eventType,
		Subject:    subject,
		Action:     action,
		OccurredAt: s.now().UTC(),
		Attributes: attributes,
	})
}

func cloneActionSpec(spec *agentsessionactionv1.AgentSessionActionSpec) *agentsessionactionv1.AgentSessionActionSpec {
	if spec == nil {
		return nil
	}
	return proto.Clone(spec).(*agentsessionactionv1.AgentSessionActionSpec)
}
