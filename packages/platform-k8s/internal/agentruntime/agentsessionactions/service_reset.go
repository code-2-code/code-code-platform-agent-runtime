package agentsessionactions

import (
	"context"
	"strings"

	"code-code.internal/go-contract/domainerror"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessions"
	"code-code.internal/platform-k8s/internal/platform/resourcemeta"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ResetWarmStateRequest struct {
	ActionID string
}

func (s *Service) ResetWarmState(ctx context.Context, sessionID string, request *ResetWarmStateRequest) (*agentsessionactionv1.AgentSessionActionState, error) {
	session, err := s.loadSessionForWarmStateReset(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	hasPendingReset, err := s.store.HasNonterminalResetWarmState(ctx, session.Spec.Session.GetSessionId())
	if err != nil {
		return nil, err
	}
	if hasPendingReset {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: session %q already has a warm state reset in progress", session.Spec.Session.GetSessionId())
	}
	snapshot, err := buildResetWarmStateSnapshot(session)
	if err != nil {
		return nil, err
	}
	actionID, err := resourcemeta.EnsureResourceID(strings.TrimSpace(requestActionID(request)), session.Spec.Session.GetSessionId(), "reset-warm-state")
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
			Labels:    actionLabels(session.Spec.Session.GetSessionId(), agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RESET_WARM_STATE),
		},
		Spec: platformv1alpha1.AgentSessionActionResourceSpec{
			Action: &agentsessionactionv1.AgentSessionActionSpec{
				ActionId:  actionID,
				SessionId: session.Spec.Session.GetSessionId(),
				Type:      agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RESET_WARM_STATE,
				InputSnapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot{
					Snapshot: &agentsessionactionv1.AgentSessionActionInputSnapshot_ResetWarmState{
						ResetWarmState: snapshot,
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
	s.recordTimelineEvent(ctx, session.Spec.Session.GetSessionId(), "SUBMITTED", "action", "create", map[string]string{
		"action_id": actionID,
		"type":      agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RESET_WARM_STATE.String(),
	})
	return actionStateFromResource(resource)
}

func (s *Service) loadSessionForWarmStateReset(ctx context.Context, sessionID string) (*platformv1alpha1.AgentSessionResource, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: session_id is required")
	}
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
	if strings.TrimSpace(resource.Spec.Session.GetHomeStateRef().GetHomeStateId()) == "" {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: session %q home_state_ref.home_state_id is required", sessionID)
	}
	return resource, nil
}

func buildResetWarmStateSnapshot(session *platformv1alpha1.AgentSessionResource) (*agentsessionactionv1.AgentSessionResetWarmStateSnapshot, error) {
	if session == nil || session.Spec.Session == nil {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: session is invalid")
	}
	source := strings.TrimSpace(session.Spec.Session.GetHomeStateRef().GetHomeStateId())
	target, err := resourcemeta.EnsureResourceID("", source, "home-state")
	if err != nil {
		return nil, err
	}
	if target == source {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: target home_state_id must differ from source")
	}
	return &agentsessionactionv1.AgentSessionResetWarmStateSnapshot{
		SessionGeneration: session.Generation,
		SourceHomeStateId: source,
		TargetHomeStateId: target,
	}, nil
}

func requestActionID(request *ResetWarmStateRequest) string {
	if request == nil {
		return ""
	}
	return request.ActionID
}

func warmStateReady(session *platformv1alpha1.AgentSessionResource) bool {
	if session == nil {
		return false
	}
	for _, condition := range session.Status.Conditions {
		if condition.Type == string(platformcontract.AgentSessionConditionTypeWarmStateReady) && condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}
