package sessionapi

import (
	"context"
	"fmt"

	authv1 "code-code.internal/go-contract/platform/auth/v1"
	cliruntimev1 "code-code.internal/go-contract/platform/cli_runtime/v1"
	egressservicev1 "code-code.internal/go-contract/platform/egress/v1"
	managementv1 "code-code.internal/go-contract/platform/management/v1"
	modelservicev1 "code-code.internal/go-contract/platform/model/v1"
	profileservicev1 "code-code.internal/go-contract/platform/profile/v1"
	providerservicev1 "code-code.internal/go-contract/platform/provider/v1"
	supportv1 "code-code.internal/go-contract/platform/support/v1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentexecution"
	"code-code.internal/platform-k8s/internal/agentruntime/agentruns"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessionactions"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessions"
	"code-code.internal/platform-k8s/internal/agentruntime/timeline"
	"code-code.internal/platform-k8s/internal/platform/runevents"
	sessiondomain "code-code.internal/session"
	"google.golang.org/grpc"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// SessionConfig groups dependencies used to expose the AgentSession service.
type SessionConfig struct {
	Client                ctrlclient.Client
	APIReader             ctrlclient.Reader
	RuntimeClient         ctrlclient.Client
	Namespace             string
	RuntimeNamespace      string
	ProfileConn           grpc.ClientConnInterface
	ProviderConn          grpc.ClientConnInterface
	CLIRuntimeConn        grpc.ClientConnInterface
	ModelConn             grpc.ClientConnInterface
	SupportConn           grpc.ClientConnInterface
	EgressConn            grpc.ClientConnInterface
	AuthConn              grpc.ClientConnInterface
	Timeline              timeline.Sink
	RunOutputs            runevents.Reader
	ActionRetryPolicy     *agentsessionactions.RetryPolicy
	SessionRepository     agentsessions.SessionRepository
	ActionStore           agentsessionactions.Store
	ActiveRunSlots        agentruns.ActiveRunSlotManager
	TurnMessages          sessiondomain.TurnMessageRepository
	ReconcileScheduler    ReconcileScheduler
	AgentRunWorkflow      agentruns.WorkflowRuntime
	CLIOutputSidecarImage string
}

// SessionServer implements platform.management.v1.AgentSessionManagementService.
type SessionServer struct {
	managementv1.UnimplementedAgentSessionManagementServiceServer

	client                ctrlclient.Client
	reader                ctrlclient.Reader
	namespace             string
	runtimeNamespace      string
	runtimeClient         ctrlclient.Client
	agentSessions         agentSessionService
	agentSessionActions   agentSessionActionService
	agentRuns             agentRunService
	timelineSink          timeline.Sink
	runOutputs            runevents.Reader
	actionRetryPolicy     *agentsessionactions.RetryPolicy
	sessionRepository     agentsessions.SessionRepository
	actionStore           agentsessionactions.Store
	activeRunSlots        agentruns.ActiveRunSlotManager
	turnMessages          sessiondomain.TurnMessageRepository
	turnOutputMessages    *aguiTurnMessageProjector
	reconcileScheduler    ReconcileScheduler
	agentRunWorkflow      agentruns.WorkflowRuntime
	cliOutputSidecarImage string
	profileSource         agentsessions.ProfileProjectionSource
	runtimeCatalog        agentexecution.RuntimeCatalog
	modelRegistry         agentexecution.ModelRegistry
	support               runtimeSupportClient
	egress                runtimeEgressClient
	auth                  runtimeAuthClient
}

type runtimeSupportClient interface {
	ResolveProviderCapabilities(ctx context.Context, in *supportv1.ResolveProviderCapabilitiesRequest, opts ...grpc.CallOption) (*supportv1.ResolveProviderCapabilitiesResponse, error)
}

type runtimeEgressClient interface {
	GetEgressRuntimePolicy(ctx context.Context, in *egressservicev1.GetEgressRuntimePolicyRequest, opts ...grpc.CallOption) (*egressservicev1.GetEgressRuntimePolicyResponse, error)
}

type runtimeAuthClient interface {
	GetCredentialRuntimeProjection(ctx context.Context, in *authv1.GetCredentialRuntimeProjectionRequest, opts ...grpc.CallOption) (*authv1.GetCredentialRuntimeProjectionResponse, error)
	GetEgressAuthPolicy(ctx context.Context, in *authv1.GetEgressAuthPolicyRequest, opts ...grpc.CallOption) (*authv1.GetEgressAuthPolicyResponse, error)
}

// NewSessionServer creates one AgentSession gRPC server.
func NewSessionServer(config SessionConfig) (*SessionServer, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: session client is nil")
	}
	if config.APIReader == nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: session api reader is nil")
	}
	if config.Namespace == "" {
		return nil, fmt.Errorf("platformk8s/sessionapi: session namespace is empty")
	}
	if config.RuntimeNamespace == "" {
		config.RuntimeNamespace = config.Namespace
	}
	if config.RuntimeClient == nil {
		config.RuntimeClient = config.Client
	}
	if config.ProfileConn == nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: profile service connection is nil")
	}
	if config.ProviderConn == nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: provider service connection is nil")
	}
	if config.CLIRuntimeConn == nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: cli runtime service connection is nil")
	}
	if config.ModelConn == nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: model service connection is nil")
	}
	if config.SupportConn == nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: support service connection is nil")
	}
	if config.EgressConn == nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: egress service connection is nil")
	}
	if config.AuthConn == nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: auth service connection is nil")
	}
	if config.AgentRunWorkflow == nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: agent run workflow runtime is nil")
	}
	if config.SessionRepository == nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: session repository is nil")
	}
	if config.ActionStore == nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: action store is nil")
	}
	if config.ActiveRunSlots == nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: active run slots are nil")
	}
	profileSource, err := agentsessions.NewRemoteProfileProjectionSource(profileservicev1.NewProfileServiceClient(config.ProfileConn))
	if err != nil {
		return nil, err
	}
	runtimeCatalog, err := agentexecution.NewRemoteRuntimeCatalog(
		providerservicev1.NewProviderServiceClient(config.ProviderConn),
		cliruntimev1.NewCLIRuntimeServiceClient(config.CLIRuntimeConn),
		supportv1.NewSupportServiceClient(config.SupportConn),
	)
	if err != nil {
		return nil, err
	}
	modelRegistry, err := agentexecution.NewRemoteModelRegistry(modelservicev1.NewModelServiceClient(config.ModelConn))
	if err != nil {
		return nil, err
	}
	services, err := assembleSessionServices(config.Client, config.APIReader, config.Namespace, config.RuntimeNamespace, config.Timeline, config.SessionRepository, config.ActionStore, config.ActiveRunSlots, profileSource, runtimeCatalog, modelRegistry)
	if err != nil {
		return nil, err
	}
	turnMessages := config.TurnMessages
	if turnMessages == nil {
		if repository, ok := config.SessionRepository.(sessiondomain.TurnMessageRepository); ok {
			turnMessages = repository
		}
	}
	return &SessionServer{
		client:                config.Client,
		reader:                config.APIReader,
		namespace:             config.Namespace,
		runtimeNamespace:      config.RuntimeNamespace,
		runtimeClient:         config.RuntimeClient,
		agentSessions:         services.agentSessions,
		agentSessionActions:   services.agentSessionActions,
		agentRuns:             services.agentRuns,
		timelineSink:          config.Timeline,
		runOutputs:            config.RunOutputs,
		actionRetryPolicy:     config.ActionRetryPolicy,
		sessionRepository:     config.SessionRepository,
		actionStore:           config.ActionStore,
		activeRunSlots:        config.ActiveRunSlots,
		turnMessages:          turnMessages,
		turnOutputMessages:    newAGUITurnMessageProjector(),
		reconcileScheduler:    config.ReconcileScheduler,
		agentRunWorkflow:      config.AgentRunWorkflow,
		cliOutputSidecarImage: config.CLIOutputSidecarImage,
		profileSource:         profileSource,
		runtimeCatalog:        runtimeCatalog,
		modelRegistry:         modelRegistry,
		support:               supportv1.NewSupportServiceClient(config.SupportConn),
		egress:                egressservicev1.NewEgressServiceClient(config.EgressConn),
		auth:                  authv1.NewAuthServiceClient(config.AuthConn),
	}, nil
}
