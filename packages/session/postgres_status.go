package session

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func (r *PostgresRepository) UpdateStatus(ctx context.Context, sessionID string, sessionStatus *agentsessionv1.AgentSessionStatus) (*agentsessionv1.AgentSessionState, error) {
	if r.begin != nil {
		var updated *agentsessionv1.AgentSessionState
		if err := r.doTx(ctx, func(txRepo *PostgresRepository) error {
			var err error
			updated, err = txRepo.UpdateStatus(ctx, sessionID, sessionStatus)
			return err
		}); err != nil {
			return nil, err
		}
		return updated, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}
	normalized, err := NormalizeStatus(sessionID, sessionStatus)
	if err != nil {
		return nil, err
	}
	current, generation, err := r.getResource(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if observed := normalized.GetObservedGeneration(); observed != 0 && observed != generation {
		return nil, status.Error(codes.FailedPrecondition, "status.observed_generation does not match current session generation")
	}
	currentState, err := stateFromAgentSessionResource(current, generation)
	if err != nil {
		return nil, err
	}
	if proto.Equal(currentState.GetStatus(), normalized) {
		return currentState, nil
	}
	current.Metadata.Generation = generation
	current.Metadata.ResourceVersion = strconv.FormatInt(generation, 10)
	current.Status = resourceStatusFromProto(normalized)
	payload, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	tag, err := r.db.Exec(ctx, `
update platform_sessions
set payload = $2::jsonb,
	updated_at = now()
where id = $1
`, sessionID, string(payload))
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, status.Error(codes.NotFound, "session not found")
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

func resourceStatusFromProto(status *agentsessionv1.AgentSessionStatus) agentSessionResourceStatus {
	updatedAt := ""
	if status.GetUpdatedAt() != nil {
		updatedAt = status.GetUpdatedAt().AsTime().UTC().Format(time.RFC3339Nano)
	}
	activeRunID := ""
	if status.GetActiveRun() != nil {
		activeRunID = strings.TrimSpace(status.GetActiveRun().GetRunId())
	}
	return agentSessionResourceStatus{
		ObservedGeneration:       status.GetObservedGeneration(),
		Conditions:               conditionsToResource(status.GetConditions()),
		Phase:                    resourcePhase(status.GetPhase()),
		RuntimeConfigGeneration:  status.GetRuntimeConfigGeneration(),
		ResourceConfigGeneration: status.GetResourceConfigGeneration(),
		RealizedRuleRevision:     status.GetRealizedRuleRevision(),
		RealizedSkillRevision:    status.GetRealizedSkillRevision(),
		RealizedMCPRevision:      status.GetRealizedMcpRevision(),
		ObservedHomeStateID:      status.GetObservedHomeStateId(),
		StateGeneration:          status.GetStateGeneration(),
		Message:                  status.GetMessage(),
		ActiveRunID:              activeRunID,
		UpdatedAt:                updatedAt,
	}
}
