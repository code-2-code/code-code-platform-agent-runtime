package agentsessions

import (
	"context"
	"strings"

	domainerror "code-code.internal/go-contract/domainerror"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
)

// ActiveRunManager manages the session-scoped active run slot through the
// product-owned session repository.
type ActiveRunManager struct {
	sessions  SessionRepository
	namespace string
}

// NewActiveRunManager creates one active run slot manager.
func NewActiveRunManager(sessions SessionRepository, namespace string) (*ActiveRunManager, error) {
	if sessions == nil {
		return nil, validation("active run manager session repository is nil")
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return nil, validation("active run manager namespace is empty")
	}
	return &ActiveRunManager{sessions: sessions, namespace: namespace}, nil
}

// Claim reserves the active run slot for one session.
func (m *ActiveRunManager) Claim(ctx context.Context, sessionID string, runID string) (*platformv1alpha1.AgentSessionResource, error) {
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if sessionID == "" {
		return nil, domainerror.NewValidation("platformk8s/agentsessions: session_id is required")
	}
	if runID == "" {
		return nil, domainerror.NewValidation("platformk8s/agentsessions: run_id is required")
	}
	state, err := m.sessions.ClaimActiveRun(ctx, sessionID, runID)
	if err != nil {
		return nil, err
	}
	return ResourceFromState(state, m.namespace)
}

// Release clears the active run slot when it still points to the given run.
func (m *ActiveRunManager) Release(ctx context.Context, sessionID string, runID string) (bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if sessionID == "" || runID == "" {
		return false, nil
	}
	return m.sessions.ReleaseActiveRun(ctx, sessionID, runID)
}

// Load returns the latest session resource.
func (m *ActiveRunManager) Load(ctx context.Context, sessionID string) (*platformv1alpha1.AgentSessionResource, error) {
	state, err := m.sessions.Get(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	return ResourceFromState(state, m.namespace)
}
