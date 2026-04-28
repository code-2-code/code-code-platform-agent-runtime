package agentsessionactions

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	agentsessionactionv1 "code-code.internal/go-contract/platform/agent_session_action/v1"
	domaineventv1 "code-code.internal/go-contract/platform/domain_event/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/platform/domainevents"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Store interface {
	Get(context.Context, string) (*platformv1alpha1.AgentSessionActionResource, error)
	Create(context.Context, *platformv1alpha1.AgentSessionActionResource) error
	Update(context.Context, string, func(*platformv1alpha1.AgentSessionActionResource) error) (*platformv1alpha1.AgentSessionActionResource, error)
	UpdateStatus(context.Context, string, *platformv1alpha1.AgentSessionActionResourceStatus) (*platformv1alpha1.AgentSessionActionResource, error)
	ListBySession(context.Context, string) ([]platformv1alpha1.AgentSessionActionResource, error)
	HasNonterminalResetWarmState(context.Context, string) (bool, error)
}

type PostgresStore struct {
	pool      *pgxpool.Pool
	outbox    *domainevents.Outbox
	namespace string
}

func NewPostgresStore(ctx context.Context, pool *pgxpool.Pool, outbox *domainevents.Outbox, namespace string) (*PostgresStore, error) {
	if pool == nil {
		return nil, fmt.Errorf("agentsessionactions: postgres pool is nil")
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return nil, fmt.Errorf("agentsessionactions: namespace is empty")
	}
	store := &PostgresStore{pool: pool, outbox: outbox, namespace: namespace}
	if err := store.ensureSchema(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *PostgresStore) ensureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
create table if not exists platform_agent_session_actions (
	id text primary key,
	payload jsonb not null,
	generation bigint not null default 1,
	created_at timestamptz not null default now(),
	updated_at timestamptz not null default now()
);
create index if not exists platform_agent_session_actions_namespace_id_idx on platform_agent_session_actions (
	((payload->'metadata'->>'namespace')),
	id
);
create index if not exists platform_agent_session_actions_session_id_label_idx on platform_agent_session_actions (
	((coalesce(payload->'metadata'->'labels', '{}'::jsonb)->>'agentsessionaction.code-code.internal/session-id')),
	id
);`)
	return err
}

func (s *PostgresStore) Get(ctx context.Context, actionID string) (*platformv1alpha1.AgentSessionActionResource, error) {
	resource, _, err := s.get(ctx, s.pool, strings.TrimSpace(actionID), false)
	return resource, err
}

func (s *PostgresStore) Create(ctx context.Context, resource *platformv1alpha1.AgentSessionActionResource) error {
	if resource == nil || resource.Spec.Action == nil {
		return validation("action resource is invalid")
	}
	normalizeActionResource(resource, s.namespace, 1)
	payload, err := json.Marshal(resource)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
insert into platform_agent_session_actions (id, payload, generation, created_at, updated_at)
values ($1, $2::jsonb, 1, now(), now())
on conflict (id) do nothing`, resource.GetName(), string(payload))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apierrors.NewAlreadyExists(actionGroupResource(), resource.GetName())
	}
	if err := s.enqueue(ctx, tx, resource, "created"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) Update(ctx context.Context, actionID string, mutate func(*platformv1alpha1.AgentSessionActionResource) error) (*platformv1alpha1.AgentSessionActionResource, error) {
	return s.update(ctx, actionID, true, "updated", mutate)
}

func (s *PostgresStore) UpdateStatus(ctx context.Context, actionID string, next *platformv1alpha1.AgentSessionActionResourceStatus) (*platformv1alpha1.AgentSessionActionResource, error) {
	return s.update(ctx, actionID, false, "status_updated", func(current *platformv1alpha1.AgentSessionActionResource) error {
		if next != nil {
			current.Status = *next.DeepCopy()
		}
		return nil
	})
}

func (s *PostgresStore) update(ctx context.Context, actionID string, incrementGeneration bool, mutation string, mutate func(*platformv1alpha1.AgentSessionActionResource) error) (*platformv1alpha1.AgentSessionActionResource, error) {
	actionID = strings.TrimSpace(actionID)
	if actionID == "" {
		return nil, validation("action_id is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	resource, generation, err := s.get(ctx, tx, actionID, true)
	if err != nil {
		return nil, err
	}
	if mutate != nil {
		if err := mutate(resource); err != nil {
			return nil, err
		}
	}
	if incrementGeneration {
		generation++
	}
	normalizeActionResource(resource, s.namespace, generation)
	payload, err := json.Marshal(resource)
	if err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx, `
update platform_agent_session_actions
set payload = $2::jsonb,
	generation = $3,
	updated_at = now()
where id = $1`, actionID, string(payload), generation)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, apierrors.NewNotFound(actionGroupResource(), actionID)
	}
	if err := s.enqueue(ctx, tx, resource, mutation); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return resource, nil
}

func (s *PostgresStore) ListBySession(ctx context.Context, sessionID string) ([]platformv1alpha1.AgentSessionActionResource, error) {
	rows, err := s.pool.Query(ctx, `
select payload, generation
from platform_agent_session_actions
where payload->'metadata'->>'namespace' = $1
	and coalesce(payload->'metadata'->'labels', '{}'::jsonb)->>'agentsessionaction.code-code.internal/session-id' = $2
order by created_at, id`, s.namespace, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []platformv1alpha1.AgentSessionActionResource{}
	for rows.Next() {
		resource, err := scanActionResource(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *resource)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		left := actionCreatedAt(&items[i])
		right := actionCreatedAt(&items[j])
		if left.Equal(right) {
			return items[i].Name < items[j].Name
		}
		return left.Before(right)
	})
	return items, nil
}

func (s *PostgresStore) HasNonterminalResetWarmState(ctx context.Context, sessionID string) (bool, error) {
	items, err := s.ListBySession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	for i := range items {
		action := items[i].Spec.Action
		if action == nil || action.GetType() != agentsessionactionv1.AgentSessionActionType_AGENT_SESSION_ACTION_TYPE_RESET_WARM_STATE {
			continue
		}
		if !isTerminalPhase(items[i].Status.Phase) {
			return true, nil
		}
	}
	return false, nil
}

func (s *PostgresStore) get(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, actionID string, lock bool) (*platformv1alpha1.AgentSessionActionResource, int64, error) {
	if actionID == "" {
		return nil, 0, validation("action_id is required")
	}
	query := `
select payload, generation
from platform_agent_session_actions
where id = $1
`
	if lock {
		query += "for update"
	}
	var payload []byte
	var generation int64
	if err := db.QueryRow(ctx, query, actionID).Scan(&payload, &generation); err != nil {
		return nil, 0, apierrors.NewNotFound(actionGroupResource(), actionID)
	}
	resource := &platformv1alpha1.AgentSessionActionResource{}
	if err := json.Unmarshal(payload, resource); err != nil {
		return nil, 0, err
	}
	normalizeActionResource(resource, s.namespace, generation)
	return resource, generation, nil
}

type actionRow interface {
	Scan(...any) error
}

func scanActionResource(row actionRow) (*platformv1alpha1.AgentSessionActionResource, error) {
	var payload []byte
	var generation int64
	if err := row.Scan(&payload, &generation); err != nil {
		return nil, err
	}
	resource := &platformv1alpha1.AgentSessionActionResource{}
	if err := json.Unmarshal(payload, resource); err != nil {
		return nil, err
	}
	resource.SetGeneration(generation)
	resource.SetResourceVersion(strconv.FormatInt(generation, 10))
	return resource, nil
}

func normalizeActionResource(resource *platformv1alpha1.AgentSessionActionResource, namespace string, generation int64) {
	if resource.TypeMeta.APIVersion == "" {
		resource.TypeMeta.APIVersion = platformv1alpha1.GroupVersion.String()
	}
	if resource.TypeMeta.Kind == "" {
		resource.TypeMeta.Kind = platformv1alpha1.KindAgentSessionActionResource
	}
	if strings.TrimSpace(resource.Namespace) == "" {
		resource.Namespace = strings.TrimSpace(namespace)
	}
	if resource.CreationTimestamp.IsZero() {
		resource.CreationTimestamp = metav1.NewTime(time.Now().UTC())
	}
	if resource.Status.CreatedAt == nil {
		created := metav1.NewTime(resource.CreationTimestamp.UTC())
		resource.Status.CreatedAt = &created
	}
	resource.SetGeneration(generation)
	resource.SetResourceVersion(strconv.FormatInt(generation, 10))
}

func (s *PostgresStore) enqueue(ctx context.Context, tx pgx.Tx, resource *platformv1alpha1.AgentSessionActionResource, mutation string) error {
	if s.outbox == nil {
		return nil
	}
	state, err := actionStateFromResource(resource)
	if err != nil {
		return err
	}
	return s.outbox.EnqueueTx(ctx, tx, &domaineventv1.DomainEvent{
		EventType:        mutation,
		AggregateType:    "agent_session_action",
		AggregateId:      resource.GetName(),
		AggregateVersion: resource.GetGeneration(),
		Payload: &domaineventv1.DomainEvent_AgentSessionAction{AgentSessionAction: &domaineventv1.AgentSessionActionEvent{
			Mutation: actionMutation(mutation),
			State:    state,
		}},
	})
}

func actionMutation(value string) domaineventv1.DomainMutation {
	switch strings.TrimSpace(value) {
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

func actionGroupResource() schema.GroupResource {
	return schema.GroupResource{Group: platformv1alpha1.GroupName, Resource: "agentsessionactions"}
}
