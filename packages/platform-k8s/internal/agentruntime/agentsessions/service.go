package agentsessions

import (
	"context"
	"fmt"
	"strings"

	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
)

type Service struct {
	sessions  SessionRepository
	namespace string
}

func NewService(sessions SessionRepository, namespace string) (*Service, error) {
	if sessions == nil {
		return nil, fmt.Errorf("platformk8s/agentsessions: session repository is nil")
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, fmt.Errorf("platformk8s/agentsessions: namespace is empty")
	}
	return &Service{
		sessions:  sessions,
		namespace: strings.TrimSpace(namespace),
	}, nil
}

func (s *Service) Get(ctx context.Context, sessionID string) (*agentsessionv1.AgentSessionState, error) {
	return s.sessions.Get(ctx, strings.TrimSpace(sessionID))
}
