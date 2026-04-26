package session

import (
	"strings"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func NormalizeStatus(sessionID string, current *agentsessionv1.AgentSessionStatus) (*agentsessionv1.AgentSessionStatus, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}
	if current == nil {
		return nil, status.Error(codes.InvalidArgument, "session status is required")
	}
	next := proto.Clone(current).(*agentsessionv1.AgentSessionStatus)
	if statusSessionID := strings.TrimSpace(next.GetSessionId()); statusSessionID != "" && statusSessionID != sessionID {
		return nil, status.Error(codes.InvalidArgument, "status.session_id must match session_id")
	}
	next.SessionId = sessionID
	if next.UpdatedAt == nil {
		next.UpdatedAt = timestamppb.Now()
	}
	return next, nil
}
