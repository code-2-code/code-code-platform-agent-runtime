package agentruns

import (
	"context"
	"strings"

	"code-code.internal/go-contract/domainerror"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/platform/resourceops"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (s *Service) Cancel(ctx context.Context, runID string) (*agentrunv1.AgentRunState, error) {
	resource, err := s.getRunResource(ctx, runID)
	if err != nil {
		return nil, err
	}
	if resource.Spec.Run == nil {
		return nil, domainerror.NewValidation("platformk8s/agentruns: run %q is missing payload", resource.Name)
	}
	if isTerminalPhase(resource.Status.Phase) || resource.Spec.Run.GetCancelRequested() {
		return runStateFromResource(resource)
	}
	key := types.NamespacedName{Namespace: s.namespace, Name: resource.Name}
	if err := resourceops.UpdateResource(ctx, s.client, key, func(current *platformv1alpha1.AgentRunResource) error {
		if current.Spec.Run == nil {
			return domainerror.NewValidation("platformk8s/agentruns: run %q is missing payload", resource.Name)
		}
		current.Spec.Run.CancelRequested = true
		return nil
	}, func() *platformv1alpha1.AgentRunResource {
		return &platformv1alpha1.AgentRunResource{}
	}); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, domainerror.NewNotFound("platformk8s/agentruns: run %q not found", resource.Name)
		}
		return nil, err
	}
	updated, err := s.getRunResource(ctx, resource.Name)
	if err != nil {
		return nil, err
	}
	s.recordTimelineEvent(ctx, updated.Spec.Run.GetSessionId(), "STOP_REQUESTED", "run", "cancel", map[string]string{
		"run_id": updated.Spec.Run.GetRunId(),
	})
	return runStateFromResource(updated)
}

func (s *Service) getRunResource(ctx context.Context, runID string) (*platformv1alpha1.AgentRunResource, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, domainerror.NewValidation("platformk8s/agentruns: run_id is required")
	}
	resource := &platformv1alpha1.AgentRunResource{}
	if err := s.reader.Get(ctx, types.NamespacedName{Namespace: s.namespace, Name: runID}, resource); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, domainerror.NewNotFound("platformk8s/agentruns: run %q not found", runID)
		}
		return nil, err
	}
	return resource, nil
}
