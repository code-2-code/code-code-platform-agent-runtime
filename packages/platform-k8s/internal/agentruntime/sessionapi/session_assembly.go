package sessionapi

import (
	"fmt"

	"code-code.internal/platform-k8s/internal/agentruntime/agentexecution"
	"code-code.internal/platform-k8s/internal/agentruntime/agentruns"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessionactions"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessions"
	"code-code.internal/platform-k8s/internal/agentruntime/timeline"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type sessionServices struct {
	agentSessions       agentSessionService
	agentSessionActions agentSessionActionService
	agentRuns           agentRunService
}

func assembleSessionServices(
	client ctrlclient.Client,
	reader ctrlclient.Reader,
	namespace string,
	runtimeNamespace string,
	timelineSink timeline.Sink,
	sessionRepository agentsessions.SessionRepository,
	actionStore agentsessionactions.Store,
	slots agentruns.ActiveRunSlotManager,
	profileSource agentsessions.ProfileProjectionSource,
	runtimeCatalog agentexecution.RuntimeCatalog,
	modelRegistry agentexecution.ModelRegistry,
) (*sessionServices, error) {
	agentSessionMgmt, err := agentsessions.NewService(sessionRepository, namespace)
	if err != nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: create agent session management: %w", err)
	}
	resolver, err := agentexecution.NewResolver(runtimeCatalog, modelRegistry)
	if err != nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: create execution resolver: %w", err)
	}
	agentSessionActionMgmt, err := agentsessionactions.NewService(actionStore, sessionRepository, namespace, timelineSink, resolver)
	if err != nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: create agent session action management: %w", err)
	}
	agentRunMgmt, err := agentruns.NewService(
		client,
		reader,
		namespace,
		timelineSink,
		agentruns.WithRuntimeNamespace(runtimeNamespace),
		agentruns.WithActiveRunSlots(slots),
	)
	if err != nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: create agent run management: %w", err)
	}
	return &sessionServices{
		agentSessions:       agentSessionMgmt,
		agentSessionActions: agentSessionActionMgmt,
		agentRuns:           agentRunMgmt,
	}, nil
}
