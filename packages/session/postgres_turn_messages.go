package session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultTurnMessagePageSize = int32(200)
	maxTurnMessagePageSize     = int32(500)
)

func (r *PostgresRepository) UpsertTurnMessage(ctx context.Context, message TurnMessage) error {
	if r.begin != nil {
		return r.doTx(ctx, func(txRepo *PostgresRepository) error {
			return txRepo.UpsertTurnMessage(ctx, message)
		})
	}
	normalized, err := NormalizeTurnMessage(message)
	if err != nil {
		return err
	}
	createdAt := normalized.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err = r.db.Exec(ctx, `
insert into platform_session_turn_messages (
	session_id, turn_id, run_id, message_id, message, sequence, created_at, updated_at
) values ($1, $2, $3, $4, $5::jsonb, $6, $7, now())
on conflict (session_id, message_id) do update set
	turn_id = excluded.turn_id,
	run_id = excluded.run_id,
	message = excluded.message,
	sequence = excluded.sequence,
	updated_at = now()
`, normalized.SessionID, normalized.TurnID, normalized.RunID, normalized.MessageID, string(normalized.Message), normalized.Sequence, createdAt)
	return err
}

func (r *PostgresRepository) ListTurnMessages(ctx context.Context, sessionID string, pageSize int32, pageToken string) ([]TurnMessage, string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, "", status.Error(codes.InvalidArgument, "session_id is required")
	}
	limit := normalizeTurnMessagePageSize(pageSize)
	cursorCreatedAt, cursorSequence, cursorMessageID, hasCursor, err := parseTurnMessagePageToken(pageToken)
	if err != nil {
		return nil, "", err
	}
	rows, err := r.db.Query(ctx, `
select session_id, turn_id, run_id, message_id, message, sequence, created_at
from platform_session_turn_messages
where session_id = $1
	and (not $2::boolean or (created_at, sequence, message_id) > ($3::timestamptz, $4::bigint, $5::text))
order by created_at asc, sequence asc, message_id asc
limit $6
`, sessionID, hasCursor, cursorCreatedAt, cursorSequence, cursorMessageID, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	messages := make([]TurnMessage, 0, limit+1)
	for rows.Next() {
		message, err := scanTurnMessage(rows)
		if err != nil {
			return nil, "", err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if int32(len(messages)) <= limit {
		return messages, "", nil
	}
	last := messages[limit-1]
	return messages[:limit], encodeTurnMessagePageToken(last.CreatedAt, last.Sequence, last.MessageID), nil
}

func scanTurnMessage(row pgx.Row) (TurnMessage, error) {
	var message TurnMessage
	var messageJSON []byte
	if err := row.Scan(
		&message.SessionID,
		&message.TurnID,
		&message.RunID,
		&message.MessageID,
		&messageJSON,
		&message.Sequence,
		&message.CreatedAt,
	); err != nil {
		return TurnMessage{}, err
	}
	message.Message = json.RawMessage(messageJSON)
	return message, nil
}

func normalizeTurnMessagePageSize(pageSize int32) int32 {
	if pageSize <= 0 {
		return defaultTurnMessagePageSize
	}
	if pageSize > maxTurnMessagePageSize {
		return maxTurnMessagePageSize
	}
	return pageSize
}

func parseTurnMessagePageToken(token string) (time.Time, int64, string, bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return time.Time{}, 0, "", false, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return time.Time{}, 0, "", false, status.Error(codes.InvalidArgument, "page_token is invalid")
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 || strings.TrimSpace(parts[2]) == "" {
		return time.Time{}, 0, "", false, status.Error(codes.InvalidArgument, "page_token is invalid")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, 0, "", false, status.Error(codes.InvalidArgument, "page_token is invalid")
	}
	sequence, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return time.Time{}, 0, "", false, status.Error(codes.InvalidArgument, "page_token is invalid")
	}
	return createdAt, sequence, parts[2], true, nil
}

func encodeTurnMessagePageToken(createdAt time.Time, sequence int64, messageID string) string {
	payload := createdAt.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(sequence, 10) + "|" + strings.TrimSpace(messageID)
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}
