package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	domaineventv1 "code-code.internal/go-contract/platform/domain_event/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	domainSubjectPrefix         = "platform.domain"
	domainAggregateAgentSession = "agent_session"
)

func (r *PostgresRepository) enqueueSessionEvent(ctx context.Context, eventType string, state *agentsessionv1.AgentSessionState) error {
	sessionID := strings.TrimSpace(state.GetSpec().GetSessionId())
	if sessionID == "" {
		return fmt.Errorf("code-code/session: session event missing session_id")
	}
	event := &domaineventv1.DomainEvent{
		EventId:          newDomainEventID(),
		EventType:        strings.TrimSpace(eventType),
		AggregateType:    domainAggregateAgentSession,
		AggregateId:      sessionID,
		AggregateVersion: state.GetGeneration(),
		OccurredAt:       timestamppb.Now(),
		Producer:         r.producer,
		Payload: &domaineventv1.DomainEvent_AgentSession{AgentSession: &domaineventv1.AgentSessionEvent{
			Mutation: sessionMutation(eventType),
			State:    state,
		}},
	}
	payload, err := proto.Marshal(event)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
insert into platform_domain_outbox (
	event_id, subject, payload, aggregate_type, aggregate_id, aggregate_version
) values ($1, $2, $3, $4, $5, $6)
on conflict (event_id) do nothing`,
		event.GetEventId(),
		domainSubject(event),
		payload,
		event.GetAggregateType(),
		event.GetAggregateId(),
		event.GetAggregateVersion(),
	)
	return err
}

func sessionMutation(eventType string) domaineventv1.DomainMutation {
	switch strings.TrimSpace(eventType) {
	case "created":
		return domaineventv1.DomainMutation_DOMAIN_MUTATION_CREATED
	case "status_updated":
		return domaineventv1.DomainMutation_DOMAIN_MUTATION_STATUS_UPDATED
	case "deleted":
		return domaineventv1.DomainMutation_DOMAIN_MUTATION_DELETED
	default:
		return domaineventv1.DomainMutation_DOMAIN_MUTATION_UPDATED
	}
}

func domainSubject(event *domaineventv1.DomainEvent) string {
	aggregate := domainSubjectToken(event.GetAggregateType())
	mutation := domainSubjectToken(strings.ToLower(event.GetEventType()))
	if mutation == "" || mutation == "_" {
		mutation = "changed"
	}
	return fmt.Sprintf("%s.%s.%s", domainSubjectPrefix, aggregate, mutation)
}

func domainSubjectToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, ".", "_")
	value = strings.ReplaceAll(value, "*", "_")
	value = strings.ReplaceAll(value, ">", "_")
	if value == "" {
		return "_"
	}
	return value
}

func newDomainEventID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
}
