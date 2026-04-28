package sessionapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"code-code.internal/platform-k8s/internal/platform/triggerhttp"
)

const (
	reconcileSessionAction        = "reconcile-session"
	reconcileSessionActionsAction = "reconcile-session-actions"
	reconcileActionAction         = "reconcile-action"
	reconcileRunAction            = "reconcile-run"
	reconcileRunActionsAction     = "reconcile-run-actions"
	prepareAgentRunJobAction      = "prepare-agent-run-job"
	cleanupAgentRunAction         = "cleanup-agent-run"
)

type reconcileSessionTriggerRequest struct {
	SessionID string `json:"sessionId"`
}

type reconcileActionTriggerRequest struct {
	SessionID string `json:"sessionId,omitempty"`
	ActionID  string `json:"actionId"`
}

type reconcileRunTriggerRequest struct {
	SessionID string `json:"sessionId,omitempty"`
	RunID     string `json:"runId"`
}

// NewTriggerHandler exposes stateless HTTP trigger actions for Temporal activities.
func (s *SessionServer) NewTriggerHandler(logger *slog.Logger, actionToken string) (http.Handler, error) {
	return triggerhttp.NewServer(triggerhttp.Config{
		Logger: logger,
		Actions: map[string]triggerhttp.ActionFunc{
			reconcileSessionAction:        s.handleReconcileSessionTrigger,
			reconcileSessionActionsAction: s.handleReconcileSessionActionsTrigger,
			reconcileActionAction:         s.handleReconcileActionTrigger,
			reconcileRunAction:            s.handleReconcileRunTrigger,
			reconcileRunActionsAction:     s.handleReconcileRunActionsTrigger,
			prepareAgentRunJobAction:      s.handlePrepareAgentRunJobTrigger,
			cleanupAgentRunAction:         s.handleCleanupAgentRunTrigger,
		},
		MaxBody:   1 << 20,
		AuthToken: strings.TrimSpace(actionToken),
	})
}

func (s *SessionServer) handleReconcileSessionTrigger(ctx context.Context, request triggerhttp.Request) (any, error) {
	var body reconcileSessionTriggerRequest
	if err := request.DecodeJSON(&body); err != nil {
		return nil, err
	}
	sessionID := strings.TrimSpace(body.SessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	sessions, actions, _, err := s.domainReconcilers()
	if err != nil {
		return nil, err
	}
	if err := s.reconcileSessionDomain(ctx, sessionID, sessions, actions); err != nil {
		return nil, err
	}
	return map[string]string{"sessionId": sessionID}, nil
}

func (s *SessionServer) handleReconcileSessionActionsTrigger(ctx context.Context, request triggerhttp.Request) (any, error) {
	var body reconcileSessionTriggerRequest
	if err := request.DecodeJSON(&body); err != nil {
		return nil, err
	}
	sessionID := strings.TrimSpace(body.SessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	_, actions, _, err := s.domainReconcilers()
	if err != nil {
		return nil, err
	}
	results, err := actions.ReconcileSessionActions(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := s.scheduleSessionActionsReconcile(ctx, results, sessionID); err != nil {
		return nil, err
	}
	return map[string]string{"sessionId": sessionID}, nil
}

func (s *SessionServer) handleReconcileActionTrigger(ctx context.Context, request triggerhttp.Request) (any, error) {
	var body reconcileActionTriggerRequest
	if err := request.DecodeJSON(&body); err != nil {
		return nil, err
	}
	actionID := strings.TrimSpace(body.ActionID)
	if actionID == "" {
		return nil, fmt.Errorf("actionId is required")
	}
	sessions, actions, _, err := s.domainReconcilers()
	if err != nil {
		return nil, err
	}
	if err := s.reconcileActionDomain(ctx, strings.TrimSpace(body.SessionID), actionID, sessions, actions); err != nil {
		return nil, err
	}
	return map[string]string{"actionId": actionID}, nil
}

func (s *SessionServer) handleReconcileRunTrigger(ctx context.Context, request triggerhttp.Request) (any, error) {
	var body reconcileRunTriggerRequest
	if err := request.DecodeJSON(&body); err != nil {
		return nil, err
	}
	runID := strings.TrimSpace(body.RunID)
	if runID == "" {
		return nil, fmt.Errorf("runId is required")
	}
	sessions, actions, runs, err := s.domainReconcilers()
	if err != nil {
		return nil, err
	}
	if err := s.reconcileRunDomain(ctx, strings.TrimSpace(body.SessionID), runID, sessions, actions, runs); err != nil {
		return nil, err
	}
	return map[string]string{"runId": runID}, nil
}

func (s *SessionServer) handleReconcileRunActionsTrigger(ctx context.Context, request triggerhttp.Request) (any, error) {
	var body reconcileRunTriggerRequest
	if err := request.DecodeJSON(&body); err != nil {
		return nil, err
	}
	sessionID := strings.TrimSpace(body.SessionID)
	runID := strings.TrimSpace(body.RunID)
	if sessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	if runID == "" {
		return nil, fmt.Errorf("runId is required")
	}
	_, actions, _, err := s.domainReconcilers()
	if err != nil {
		return nil, err
	}
	results, err := actions.ReconcileRunActions(ctx, sessionID, runID)
	if err != nil {
		return nil, err
	}
	if err := s.scheduleRunActionsReconcile(ctx, results, sessionID, runID); err != nil {
		return nil, err
	}
	return map[string]string{"sessionId": sessionID, "runId": runID}, nil
}
