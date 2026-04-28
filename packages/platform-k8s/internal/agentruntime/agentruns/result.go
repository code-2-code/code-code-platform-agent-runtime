package agentruns

import (
	"context"
	"strings"

	resultv1 "code-code.internal/go-contract/agent/result/v1"
	"code-code.internal/go-contract/domainerror"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/platform/resourceops"
	"k8s.io/apimachinery/pkg/types"
)

// PublishTerminalResult durably writes one run terminal result into AgentRun status.
func (s *Service) PublishTerminalResult(ctx context.Context, runID string, result *resultv1.RunResult) error {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return domainerror.NewValidation("platformk8s/agentruns: run_id is required")
	}
	if result == nil {
		return domainerror.NewValidation("platformk8s/agentruns: terminal result is required")
	}
	key := types.NamespacedName{Namespace: s.namespace, Name: runID}
	sessionID := ""
	if err := resourceops.UpdateStatus(ctx, s.client, key, func(current *platformv1alpha1.AgentRunResource) error {
		if current.Spec.Run == nil {
			return domainerror.NewValidation("platformk8s/agentruns: run %q is missing payload", runID)
		}
		sessionID = current.Spec.Run.GetSessionId()
		current.Status.ResultSummary = resultSummary(result)
		current.Status.Message = terminalResultMessage(result)
		current.Status.UpdatedAt = timePtr(s.now().UTC())
		return nil
	}, func() *platformv1alpha1.AgentRunResource {
		return &platformv1alpha1.AgentRunResource{}
	}); err != nil {
		return err
	}
	s.recordTimelineEvent(ctx, sessionID, "RESULT_RECORDED", "run", "publish_terminal_result", map[string]string{"run_id": runID})
	return nil
}

func resultSummary(result *resultv1.RunResult) *platformv1alpha1.AgentRunResultSummary {
	if result == nil {
		return nil
	}
	summary := &platformv1alpha1.AgentRunResultSummary{Status: result.GetStatus().String()}
	if result.GetError() != nil {
		summary.ErrorCode = strings.TrimSpace(result.GetError().GetCode())
		summary.ErrorMessage = strings.TrimSpace(result.GetError().GetMessage())
		summary.Retryable = result.GetError().GetRetryable()
	}
	return summary
}

func terminalResultMessage(result *resultv1.RunResult) string {
	if result == nil {
		return ""
	}
	if result.GetError() != nil && strings.TrimSpace(result.GetError().GetMessage()) != "" {
		return strings.TrimSpace(result.GetError().GetMessage())
	}
	switch result.GetStatus() {
	case resultv1.RunStatus_RUN_STATUS_COMPLETED:
		return "AgentRun completed successfully."
	case resultv1.RunStatus_RUN_STATUS_CANCELLED:
		return "AgentRun canceled."
	case resultv1.RunStatus_RUN_STATUS_INTERRUPTED:
		return "AgentRun interrupted."
	case resultv1.RunStatus_RUN_STATUS_FAILED:
		return "AgentRun failed."
	default:
		return "AgentRun terminal result recorded."
	}
}
