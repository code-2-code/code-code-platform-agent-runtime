package session

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	defaultSessionNamespace = "code-code"
)

type PostgresRepositoryConfig struct {
	Namespace string
	Producer  string
}

type PostgresRepository struct {
	db        postgresSessionDB
	begin     func(context.Context) (pgx.Tx, error)
	namespace string
	producer  string
}

type postgresSessionDB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func NewPostgresRepository(ctx context.Context, pool *pgxpool.Pool, config PostgresRepositoryConfig) (*PostgresRepository, error) {
	if pool == nil {
		return nil, status.Error(codes.InvalidArgument, "postgres pool is required")
	}
	repo := newPostgresRepository(pool, config)
	repo.begin = pool.Begin
	if err := repo.ensureSchema(ctx); err != nil {
		return nil, err
	}
	return repo, nil
}

func newPostgresRepository(db postgresSessionDB, config PostgresRepositoryConfig) *PostgresRepository {
	namespace := strings.TrimSpace(config.Namespace)
	if namespace == "" {
		namespace = defaultSessionNamespace
	}
	producer := strings.TrimSpace(config.Producer)
	if producer == "" {
		producer = "platform-chat-service"
	}
	return &PostgresRepository{db: db, namespace: namespace, producer: producer}
}

func (r *PostgresRepository) WithTx(tx pgx.Tx) *PostgresRepository {
	return newPostgresRepository(tx, PostgresRepositoryConfig{Namespace: r.namespace, Producer: r.producer})
}

func (r *PostgresRepository) Get(ctx context.Context, sessionID string) (*agentsessionv1.AgentSessionState, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}
	resource, generation, err := r.getResource(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return stateFromAgentSessionResource(resource, generation)
}

func (r *PostgresRepository) Create(ctx context.Context, session *agentsessionv1.AgentSessionSpec) (*agentsessionv1.AgentSessionState, error) {
	if r.begin != nil {
		var created *agentsessionv1.AgentSessionState
		if err := r.doTx(ctx, func(txRepo *PostgresRepository) error {
			var err error
			created, err = txRepo.Create(ctx, session)
			return err
		}); err != nil {
			return nil, err
		}
		return created, nil
	}
	if session == nil {
		return nil, status.Error(codes.InvalidArgument, "session is required")
	}
	sessionID := strings.TrimSpace(session.GetSessionId())
	normalized, err := NormalizeSpec(sessionID, session)
	if err != nil {
		return nil, err
	}
	resource, err := newAgentSessionResource(r.namespace, normalized, 1)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(resource)
	if err != nil {
		return nil, err
	}
	if _, err := r.db.Exec(ctx, `
insert into platform_sessions (id, payload, generation, created_at, updated_at)
values ($1, $2::jsonb, 1, now(), now())
`, normalized.GetSessionId(), string(payload)); err != nil {
		if isSessionUniqueViolation(err) {
			return nil, status.Error(codes.AlreadyExists, "session already exists")
		}
		return nil, err
	}
	state, err := stateFromAgentSessionResource(resource, 1)
	if err != nil {
		return nil, err
	}
	if err := r.enqueueSessionEvent(ctx, "created", state); err != nil {
		return nil, err
	}
	return state, nil
}

func (r *PostgresRepository) Update(ctx context.Context, sessionID string, session *agentsessionv1.AgentSessionSpec) (*agentsessionv1.AgentSessionState, error) {
	if r.begin != nil {
		var updated *agentsessionv1.AgentSessionState
		if err := r.doTx(ctx, func(txRepo *PostgresRepository) error {
			var err error
			updated, err = txRepo.Update(ctx, sessionID, session)
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
	normalized, err := NormalizeSpec(sessionID, session)
	if err != nil {
		return nil, err
	}
	current, generation, err := r.getResource(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	currentSpec, err := specFromAgentSessionResource(current)
	if err != nil {
		return nil, err
	}
	if proto.Equal(currentSpec, normalized) {
		return stateFromAgentSessionResource(current, generation)
	}
	nextGeneration := generation + 1
	current.Metadata.Generation = nextGeneration
	current.Metadata.ResourceVersion = strconv.FormatInt(nextGeneration, 10)
	current.Spec.Session, err = marshalSessionSpec(normalized)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	tag, err := r.db.Exec(ctx, `
update platform_sessions
set payload = $2::jsonb,
	generation = $3,
	updated_at = now()
where id = $1
`, sessionID, string(payload), nextGeneration)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, status.Error(codes.NotFound, "session not found")
	}
	state, err := stateFromAgentSessionResource(current, nextGeneration)
	if err != nil {
		return nil, err
	}
	if err := r.enqueueSessionEvent(ctx, "updated", state); err != nil {
		return nil, err
	}
	return state, nil
}
