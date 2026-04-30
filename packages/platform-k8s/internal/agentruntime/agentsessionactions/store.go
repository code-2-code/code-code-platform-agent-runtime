package agentsessionactions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/platform/domainevents"
	"github.com/jackc/pgx/v5/pgxpool"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
