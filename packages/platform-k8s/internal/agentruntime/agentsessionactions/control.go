package agentsessionactions

import (
	"context"
	"strings"

	corev1 "code-code.internal/go-contract/agent/core/v1"
	"code-code.internal/go-contract/domainerror"
	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"google.golang.org/protobuf/proto"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func (s *Service) Stop(ctx context.Context, actionID string) (*agentsessionactionv1.AgentSessionActionState, error) {
	resource, err := s.getActionResource(ctx, actionID)
	if err != nil {
		return nil, err
	}
	if resource.Spec.Action == nil {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: action %q is missing payload", resource.Name)
	}
	if isTerminalPhase(resource.Status.Phase) || resource.Spec.Action.GetStopRequested() {
		return actionStateFromResource(resource)
	}
	if _, err := s.store.Update(ctx, resource.Name, func(current *platformv1alpha1.AgentSessionActionResource) error {
		if current.Spec.Action == nil {
			return domainerror.NewValidation("platformk8s/agentsessionactions: action %q is missing payload", resource.Name)
		}
		current.Spec.Action.StopRequested = true
		return nil
	}); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, domainerror.NewNotFound("platformk8s/agentsessionactions: action %q not found", resource.Name)
		}
		return nil, err
	}
	updated, err := s.getActionResource(ctx, resource.Name)
	if err != nil {
		return nil, err
	}
	s.recordTimelineEvent(ctx, updated.Spec.Action.GetSessionId(), "STOP_REQUESTED", "action", "stop", map[string]string{
		"action_id": updated.Spec.Action.GetActionId(),
		"type":      updated.Spec.Action.GetType().String(),
	})
	return actionStateFromResource(updated)
}

func (s *Service) Retry(ctx context.Context, sourceActionID string, request *RetryRequest) (*agentsessionactionv1.AgentSessionActionState, error) {
	resource, err := s.getActionResource(ctx, sourceActionID)
	if err != nil {
		return nil, err
	}
	if err := validateRetrySource(resource); err != nil {
		return nil, err
	}
	next, err := s.Create(ctx, resource.Spec.Action.GetSessionId(), &CreateRequest{
		ActionID:   retryTurnID(request),
		TurnID:     retryTurnID(request),
		RunRequest: cloneRetryRunRequest(resource.Spec.Action.GetInputSnapshot().GetRunTurn().GetRunRequest()),
	})
	if err != nil {
		return nil, err
	}
	s.recordTimelineEvent(ctx, resource.Spec.Action.GetSessionId(), "RETRIED", "action", "retry", map[string]string{
		"source_action_id": resource.Spec.Action.GetActionId(),
		"action_id":        next.GetSpec().GetActionId(),
	})
	return next, nil
}

func (s *Service) getActionResource(ctx context.Context, actionID string) (*platformv1alpha1.AgentSessionActionResource, error) {
	actionID = strings.TrimSpace(actionID)
	if actionID == "" {
		return nil, domainerror.NewValidation("platformk8s/agentsessionactions: action_id is required")
	}
	resource, err := s.store.Get(ctx, actionID)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, domainerror.NewNotFound("platformk8s/agentsessionactions: action %q not found", actionID)
		}
		return nil, err
	}
	return resource, nil
}

func validateRetrySource(resource *platformv1alpha1.AgentSessionActionResource) error {
	if resource == nil || resource.Spec.Action == nil {
		return domainerror.NewValidation("platformk8s/agentsessionactions: source action is invalid")
	}
	if resource.Spec.Action.GetType() != agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RUN_TURN {
		return domainerror.NewValidation("platformk8s/agentsessionactions: action %q is not a run_turn action", resource.Name)
	}
	switch resource.Status.Phase {
	case platformv1alpha1.AgentSessionActionResourcePhaseFailed,
		platformv1alpha1.AgentSessionActionResourcePhaseCanceled:
	default:
		return domainerror.NewValidation("platformk8s/agentsessionactions: action %q is not terminal retryable", resource.Name)
	}
	snapshot := resource.Spec.Action.GetInputSnapshot().GetRunTurn()
	if snapshot == nil || snapshot.GetRunRequest() == nil {
		return domainerror.NewValidation("platformk8s/agentsessionactions: action %q is missing source run_request", resource.Name)
	}
	return nil
}

func retryTurnID(request *RetryRequest) string {
	if request == nil {
		return ""
	}
	return strings.TrimSpace(request.TurnID)
}

func cloneRetryRunRequest(request *corev1.RunRequest) *corev1.RunRequest {
	if request == nil {
		return nil
	}
	return proto.Clone(request).(*corev1.RunRequest)
}
