package session

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (r *PostgresRepository) ensureSchema(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `
create table if not exists platform_sessions (
	id text primary key,
	payload jsonb not null,
	generation bigint not null default 1,
	created_at timestamptz not null default now(),
	updated_at timestamptz not null default now()
);

create table if not exists platform_session_turn_messages (
	session_id text not null,
	turn_id text not null default '',
	run_id text not null default '',
	message_id text not null,
	message jsonb not null,
	sequence bigint not null default 0,
	created_at timestamptz not null default now(),
	updated_at timestamptz not null default now(),
	primary key (session_id, message_id)
);
create index if not exists platform_session_turn_messages_session_order_idx on platform_session_turn_messages (session_id, created_at, sequence, message_id);

create table if not exists platform_domain_outbox (
	event_id text primary key,
	subject text not null,
	payload bytea not null,
	aggregate_type text not null,
	aggregate_id text not null,
	aggregate_version bigint not null,
	created_at timestamptz not null default now(),
	published_at timestamptz,
	attempts integer not null default 0,
	last_error text not null default ''
);
create index if not exists platform_domain_outbox_unpublished_idx on platform_domain_outbox (created_at, event_id) where published_at is null;
`)
	return err
}

func (r *PostgresRepository) doTx(ctx context.Context, fn func(*PostgresRepository) error) error {
	tx, err := r.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(r.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) getResource(ctx context.Context, sessionID string) (*agentSessionResource, int64, error) {
	return r.getResourceWithLock(ctx, sessionID, false)
}

func (r *PostgresRepository) getResourceForUpdate(ctx context.Context, sessionID string) (*agentSessionResource, int64, error) {
	return r.getResourceWithLock(ctx, sessionID, true)
}

func (r *PostgresRepository) getResourceWithLock(ctx context.Context, sessionID string, lock bool) (*agentSessionResource, int64, error) {
	var payload []byte
	var generation int64
	query := `
select payload, generation
from platform_sessions
where id = $1
`
	if lock {
		query += "for update"
	}
	if err := r.db.QueryRow(ctx, query, strings.TrimSpace(sessionID)).Scan(&payload, &generation); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, status.Error(codes.NotFound, "session not found")
		}
		return nil, 0, err
	}
	resource := &agentSessionResource{}
	if err := json.Unmarshal(payload, resource); err != nil {
		return nil, 0, err
	}
	if resource.Metadata.Name == "" {
		resource.Metadata.Name = strings.TrimSpace(sessionID)
	}
	if resource.Metadata.Namespace == "" {
		resource.Metadata.Namespace = r.namespace
	}
	return resource, generation, nil
}

func isSessionUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
