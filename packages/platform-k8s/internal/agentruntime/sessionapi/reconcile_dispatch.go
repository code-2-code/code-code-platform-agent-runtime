package sessionapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// DispatchReconcileAction implements ReconcileActionDispatcher by routing
// named actions directly to domain reconcilers, eliminating the HTTP
// self-callback roundtrip previously used by Temporal activities.
func (s *SessionServer) DispatchReconcileAction(ctx context.Context, action string, body []byte) error {
	switch strings.TrimSpace(action) {
	case reconcileSessionAction:
		return s.dispatchReconcileSession(ctx, body)
	case reconcileSessionActionsAction:
		return s.dispatchReconcileSessionActions(ctx, body)
	case reconcileActionAction:
		return s.dispatchReconcileActionAction(ctx, body)
	case reconcileRunAction:
		return s.dispatchReconcileRun(ctx, body)
	case reconcileRunActionsAction:
		return s.dispatchReconcileRunActions(ctx, body)
	default:
		return fmt.Errorf("platformk8s/sessionapi: unknown reconcile action %q", action)
	}
}

func (s *SessionServer) dispatchReconcileSession(ctx context.Context, body []byte) error {
	var req reconcileSessionTriggerRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return fmt.Errorf("sessionId is required")
	}
	sessions, actions, _, err := s.domainReconcilers()
	if err != nil {
		return err
	}
	return s.reconcileSessionDomain(ctx, sessionID, sessions, actions)
}

func (s *SessionServer) dispatchReconcileSessionActions(ctx context.Context, body []byte) error {
	var req reconcileSessionTriggerRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return fmt.Errorf("sessionId is required")
	}
	_, actions, _, err := s.domainReconcilers()
	if err != nil {
		return err
	}
	results, err := actions.ReconcileSessionActions(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.scheduleSessionActionsReconcile(ctx, results, sessionID)
}

func (s *SessionServer) dispatchReconcileActionAction(ctx context.Context, body []byte) error {
	var req reconcileActionTriggerRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return err
	}
	actionID := strings.TrimSpace(req.ActionID)
	if actionID == "" {
		return fmt.Errorf("actionId is required")
	}
	sessions, actions, _, err := s.domainReconcilers()
	if err != nil {
		return err
	}
	return s.reconcileActionDomain(ctx, strings.TrimSpace(req.SessionID), actionID, sessions, actions)
}

func (s *SessionServer) dispatchReconcileRun(ctx context.Context, body []byte) error {
	var req reconcileRunTriggerRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return err
	}
	runID := strings.TrimSpace(req.RunID)
	if runID == "" {
		return fmt.Errorf("runId is required")
	}
	sessions, actions, runs, err := s.domainReconcilers()
	if err != nil {
		return err
	}
	return s.reconcileRunDomain(ctx, strings.TrimSpace(req.SessionID), runID, sessions, actions, runs)
}

func (s *SessionServer) dispatchReconcileRunActions(ctx context.Context, body []byte) error {
	var req reconcileRunTriggerRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(req.SessionID)
	runID := strings.TrimSpace(req.RunID)
	if sessionID == "" {
		return fmt.Errorf("sessionId is required")
	}
	if runID == "" {
		return fmt.Errorf("runId is required")
	}
	_, actions, _, err := s.domainReconcilers()
	if err != nil {
		return err
	}
	results, err := actions.ReconcileRunActions(ctx, sessionID, runID)
	if err != nil {
		return err
	}
	return s.scheduleRunActionsReconcile(ctx, results, sessionID, runID)
}
