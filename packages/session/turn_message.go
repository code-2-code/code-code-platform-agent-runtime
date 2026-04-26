package session

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"code-code.internal/go-contract/agui"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TurnMessage is one durable AG-UI message plus platform ownership metadata.
type TurnMessage struct {
	SessionID string
	TurnID    string
	RunID     string
	MessageID string
	Message   json.RawMessage
	Sequence  int64
	CreatedAt time.Time
}

// TurnMessageRepository persists durable AG-UI turn messages.
type TurnMessageRepository interface {
	UpsertTurnMessage(context.Context, TurnMessage) error
	ListTurnMessages(context.Context, string, int32, string) ([]TurnMessage, string, error)
}

// NormalizeTurnMessage validates and trims one transcript message.
func NormalizeTurnMessage(message TurnMessage) (TurnMessage, error) {
	message.SessionID = strings.TrimSpace(message.SessionID)
	message.TurnID = strings.TrimSpace(message.TurnID)
	message.RunID = strings.TrimSpace(message.RunID)
	message.MessageID = strings.TrimSpace(message.MessageID)
	if message.SessionID == "" {
		return TurnMessage{}, status.Error(codes.InvalidArgument, "session_id is required")
	}
	if message.Sequence < 0 {
		return TurnMessage{}, status.Error(codes.InvalidArgument, "sequence is invalid")
	}
	if len(message.Message) == 0 {
		return TurnMessage{}, status.Error(codes.InvalidArgument, "message is required")
	}
	if !json.Valid(message.Message) {
		return TurnMessage{}, status.Error(codes.InvalidArgument, "message is invalid")
	}
	aguiMessage, err := agui.MessageFromRaw(message.Message)
	if err != nil {
		return TurnMessage{}, status.Errorf(codes.InvalidArgument, "message is invalid: %v", err)
	}
	normalized, err := agui.MessageRaw(aguiMessage)
	if err != nil {
		return TurnMessage{}, status.Errorf(codes.InvalidArgument, "message is invalid: %v", err)
	}
	message.Message = normalized
	if message.MessageID == "" {
		message.MessageID = aguiMessage.ID
	}
	return message, nil
}
