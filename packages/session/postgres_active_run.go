package session

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	platformcontract "code-code.internal/platform-contract"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ClaimActiveRun atomically reserves the session-scoped active run slot.
func (r *PostgresRepository) ClaimActiveRun(ctx context.Context, sessionID string, runID string) (*agentsessionv1.AgentSessionState, error) {
	if r.begin != nil {
		var claimed *agentsessionv1.AgentSessionState
		if err := r.doTx(ctx, func(txRepo *PostgresRepository) error {
			var err error
			claimed, err = txRepo.ClaimActiveRun(ctx, sessionID, runID)
			return err
		}); err != nil {
			return nil, err
		}
		return claimed, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}
	if runID == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id is required")
	}
	current, generation, err := r.getResourceForUpdate(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if current.Spec.Session == nil {
		return nil, status.Error(codes.FailedPrecondition, "session is missing payload")
	}
	activeRunID := strings.TrimSpace(current.Status.ActiveRunID)
	if activeRunID != "" && activeRunID != runID {
		return nil, status.Error(codes.FailedPrecondition, "session already has an active run")
	}
	if activeRunID == runID {
		return stateFromAgentSessionResource(current, generation)
	}
	if !activeRunDispatchReady(current.Status.Conditions) {
		return nil, status.Error(codes.FailedPrecondition, "session is not ready to dispatch")
	}
	now := time.Now().UTC()
	current.Status.ActiveRunID = runID
	current.Status.Phase = resourcePhase(agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_RUNNING)
	current.Status.Message = "AgentSession has an active run."
	current.Status.UpdatedAt = now.Format(time.RFC3339Nano)
	setResourceCondition(
		&current.Status.Conditions,
		string(platformcontract.AgentSessionConditionTypeReadyForNextRun),
		"False",
		string(platformcontract.AgentSessionConditionReasonActiveRunInProgress),
		"AgentSession has an active run.",
		generation,
		now,
	)
	if err := r.writeResource(ctx, sessionID, current, generation); err != nil {
		return nil, err
	}
	state, err := stateFromAgentSessionResource(current, generation)
	if err != nil {
		return nil, err
	}
	if err := r.enqueueSessionEvent(ctx, "status_updated", state); err != nil {
		return nil, err
	}
	return state, nil
}

// ReleaseActiveRun clears the active run slot when it still points to runID.
func (r *PostgresRepository) ReleaseActiveRun(ctx context.Context, sessionID string, runID string) (bool, error) {
	if r.begin != nil {
		var released bool
		if err := r.doTx(ctx, func(txRepo *PostgresRepository) error {
			var err error
			released, err = txRepo.ReleaseActiveRun(ctx, sessionID, runID)
			return err
		}); err != nil {
			return false, err
		}
		return released, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if sessionID == "" || runID == "" {
		return false, nil
	}
	current, generation, err := r.getResourceForUpdate(ctx, sessionID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return false, nil
		}
		return false, err
	}
	if strings.TrimSpace(current.Status.ActiveRunID) != runID {
		return false, nil
	}
	current.Status.ActiveRunID = ""
	current.Status.Phase = resourcePhase(agentsessionv1.AgentSessionPhase_AGENT_SESSION_PHASE_PENDING)
	current.Status.Message = "AgentSession active run completed; waiting for readiness reconcile."
	current.Status.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := r.writeResource(ctx, sessionID, current, generation); err != nil {
		return false, err
	}
	state, err := stateFromAgentSessionResource(current, generation)
	if err != nil {
		return false, err
	}
	if err := r.enqueueSessionEvent(ctx, "status_updated", state); err != nil {
		return false, err
	}
	return true, nil
}

func (r *PostgresRepository) writeResource(ctx context.Context, sessionID string, resource *agentSessionResource, generation int64) error {
	resource.Metadata.Generation = generation
	resource.Metadata.ResourceVersion = strconv.FormatInt(generation, 10)
	payload, err := json.Marshal(resource)
	if err != nil {
		return err
	}
	tag, err := r.db.Exec(ctx, `
update platform_sessions
set payload = $2::jsonb,
	updated_at = now()
where id = $1
`, sessionID, string(payload))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return status.Error(codes.NotFound, "session not found")
	}
	return nil
}

func activeRunDispatchReady(conditions []agentSessionCondition) bool {
	return resourceConditionTrue(conditions, string(platformcontract.AgentSessionConditionTypeWorkspaceReady)) &&
		resourceConditionTrue(conditions, string(platformcontract.AgentSessionConditionTypeWarmStateReady))
}

func resourceConditionTrue(conditions []agentSessionCondition, conditionType string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType && condition.Status == "True" {
			return true
		}
	}
	return false
}

func setResourceCondition(conditions *[]agentSessionCondition, conditionType, conditionStatus, reason, message string, observedGeneration int64, now time.Time) {
	next := agentSessionCondition{
		Type:               conditionType,
		Status:             conditionStatus,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: observedGeneration,
		LastTransitionTime: now.UTC().Format(time.RFC3339Nano),
	}
	for i := range *conditions {
		if (*conditions)[i].Type == conditionType {
			if (*conditions)[i].Status == conditionStatus && (*conditions)[i].Reason == reason && (*conditions)[i].Message == message && (*conditions)[i].ObservedGeneration == observedGeneration {
				return
			}
			(*conditions)[i] = next
			return
		}
	}
	*conditions = append(*conditions, next)
}
