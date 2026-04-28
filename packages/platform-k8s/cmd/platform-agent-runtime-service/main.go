package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	managementv1 "code-code.internal/go-contract/platform/management/v1"
	platformk8s "code-code.internal/platform-k8s"
	"code-code.internal/platform-k8s/internal/agentruntime/agentruns"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessionactions"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessions"
	"code-code.internal/platform-k8s/internal/agentruntime/sessionapi"
	"code-code.internal/platform-k8s/internal/agentruntime/timeline"
	"code-code.internal/platform-k8s/internal/platform/domainevents"
	"code-code.internal/platform-k8s/internal/platform/runevents"
	"code-code.internal/platform-k8s/internal/platform/state"
	"code-code.internal/platform-k8s/internal/platform/telemetry"
	"code-code.internal/platform-k8s/internal/platform/temporalruntime"
	sessiondomain "code-code.internal/session"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func main() {
	addr := envOrDefault("PLATFORM_AGENT_RUNTIME_SERVICE_GRPC_ADDR", ":8081")
	httpAddr := envOrDefault("PLATFORM_AGENT_RUNTIME_SERVICE_HTTP_ADDR", ":8080")
	namespace := envOrDefault("PLATFORM_AGENT_RUNTIME_SERVICE_NAMESPACE", "code-code")
	runtimeNamespace := envOrDefault("PLATFORM_AGENT_RUNTIME_SERVICE_RUNTIME_NAMESPACE", namespace)
	profileAddr := envOrDefault("PLATFORM_AGENT_RUNTIME_SERVICE_PROFILE_GRPC_ADDR", "platform-profile-service:8081")
	providerAddr := envOrDefault("PLATFORM_AGENT_RUNTIME_SERVICE_PROVIDER_GRPC_ADDR", "platform-provider-service:8081")
	cliRuntimeAddr := envOrDefault("PLATFORM_AGENT_RUNTIME_SERVICE_CLI_RUNTIME_GRPC_ADDR", "platform-cli-runtime-service:8081")
	modelAddr := envOrDefault("PLATFORM_AGENT_RUNTIME_SERVICE_MODEL_GRPC_ADDR", "platform-model-service:8081")
	supportAddr := envOrDefault("PLATFORM_AGENT_RUNTIME_SERVICE_SUPPORT_GRPC_ADDR", "platform-support-service:8081")
	egressAddr := envOrDefault("PLATFORM_AGENT_RUNTIME_SERVICE_EGRESS_GRPC_ADDR", "platform-egress-service.code-code-net.svc.cluster.local:8081")
	authAddr := envOrDefault("PLATFORM_AGENT_RUNTIME_SERVICE_AUTH_GRPC_ADDR", "platform-auth-service:8081")
	internalActionToken := strings.TrimSpace(os.Getenv("PLATFORM_AGENT_RUNTIME_SERVICE_INTERNAL_ACTION_TOKEN"))
	cliOutputSidecarImage := os.Getenv("PLATFORM_AGENT_RUNTIME_SERVICE_CLI_OUTPUT_SIDECAR_IMAGE")
	timelineNATSURL := os.Getenv("PLATFORM_AGENT_RUNTIME_SERVICE_TIMELINE_NATS_URL")
	timelineNATSSubjectPrefix := envOrDefault("PLATFORM_AGENT_RUNTIME_SERVICE_TIMELINE_NATS_SUBJECT_PREFIX", "platform.timeline")
	domainEventsNATSURL := firstEnv("PLATFORM_AGENT_RUNTIME_SERVICE_DOMAIN_EVENTS_NATS_URL", "PLATFORM_AGENT_RUNTIME_SERVICE_TIMELINE_NATS_URL")
	databaseURL := firstEnv("PLATFORM_DATABASE_URL", "PLATFORM_AGENT_RUNTIME_SERVICE_DATABASE_URL")
	actionRetryPolicy, err := actionRetryPolicyFromEnv()
	must(err)

	telemetryShutdown, err := telemetry.Setup(context.Background(), envOrDefault("OTEL_SERVICE_NAME", "platform-agent-runtime-service"))
	must(err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = telemetryShutdown(ctx)
	}()

	scheme := runtime.NewScheme()
	must(corev1.AddToScheme(scheme))
	must(batchv1.AddToScheme(scheme))
	must(platformk8s.AddToScheme(scheme))
	ctrl.SetLogger(ctrlzap.New(ctrlzap.UseDevMode(true)))

	config := ctrl.GetConfigOrDie()
	directClient, err := ctrlclient.New(config, ctrlclient.Options{Scheme: scheme})
	must(err)
	statePool, err := state.OpenPostgres(context.Background(), databaseURL, "platform-agent-runtime-service")
	must(err)
	defer statePool.Close()
	outbox, err := domainevents.NewOutbox(statePool, "platform-agent-runtime-service")
	must(err)
	sessionRepo, err := sessiondomain.NewPostgresRepository(context.Background(), statePool, sessiondomain.PostgresRepositoryConfig{
		Namespace: namespace,
		Producer:  "platform-agent-runtime-service",
	})
	must(err)
	actionStore, err := agentsessionactions.NewPostgresStore(context.Background(), statePool, outbox, namespace)
	must(err)
	activeRunSlots, err := agentsessions.NewActiveRunManager(sessionRepo, namespace)
	must(err)
	temporalConfig := temporalruntime.ConfigFromEnv(sessionapi.TemporalTaskQueue)
	temporalClient, err := temporalruntime.Dial(context.Background(), temporalConfig)
	must(err)
	defer temporalClient.Close()
	reconcileScheduler, err := sessionapi.NewTemporalReconcileScheduler(sessionapi.TemporalReconcileSchedulerConfig{
		Client:            temporalClient,
		TaskQueue:         temporalConfig.TaskQueue,
		Namespace:         runtimeNamespace,
		PlatformNamespace: namespace,
	})
	must(err)
	agentRunWorkflow, err := agentruns.NewTemporalWorkflowRuntime(agentruns.TemporalWorkflowRuntimeConfig{
		TemporalClient:         temporalClient,
		RuntimeClient:          directClient,
		ControlNamespace:       namespace,
		RuntimeNamespace:       runtimeNamespace,
		TaskQueue:              temporalConfig.TaskQueue,
		CLIOutputSidecarImage:  cliOutputSidecarImage,
		TriggerHTTPActionToken: internalActionToken,
	})
	must(err)
	temporalWorker := temporalruntime.NewWorker(temporalClient, temporalConfig.TaskQueue)
	must(agentRunWorkflow.Register(temporalWorker))
	must(sessionapi.RegisterTemporalWorkflows(temporalWorker, reconcileScheduler))
	must(temporalWorker.Start())
	defer temporalWorker.Stop()

	var timelineSink timeline.Sink
	if timelineNATSURL != "" {
		timelineSink, err = timeline.NewSink(timeline.SinkConfig{
			NATSURL:           timelineNATSURL,
			NATSSubjectPrefix: timelineNATSSubjectPrefix,
			ApplicationName:   "platform-agent-runtime-service",
		})
		must(err)
		defer timelineSink.Close()
		log.Printf("timeline sink enabled (nats+otel)")
	}
	var runOutputs runevents.Reader
	if timelineNATSURL != "" {
		runOutputs, err = runevents.NewJetStreamReader(runevents.Config{
			ClientName: "platform-agent-runtime-service-run-output",
			NATSURL:    timelineNATSURL,
		})
		must(err)
		defer runOutputs.Close()
		log.Printf("run output reader enabled (nats)")
	}

	manager, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                     scheme,
		Metrics:                    metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress:     "0",
		LeaderElection:             true,
		LeaderElectionID:           "platform-agent-runtime-service.code-code.internal",
		LeaderElectionNamespace:    namespace,
		LeaderElectionResourceLock: "leases",
	})
	must(err)
	runReconciler, err := agentruns.NewReconciler(agentruns.ReconcilerConfig{
		Client:          manager.GetClient(),
		Namespace:       namespace,
		WorkflowRuntime: agentRunWorkflow,
		Slots:           activeRunSlots,
	})
	must(err)
	runReconciler.SetTimelineSink(timelineSink)
	must(runReconciler.SetupWithManager(manager))

	profileConn, err := grpc.NewClient(profileAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	must(err)
	defer func() { _ = profileConn.Close() }()
	providerConn, err := grpc.NewClient(providerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	must(err)
	defer func() { _ = providerConn.Close() }()
	cliRuntimeConn, err := grpc.NewClient(cliRuntimeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	must(err)
	defer func() { _ = cliRuntimeConn.Close() }()
	modelConn, err := grpc.NewClient(modelAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	must(err)
	defer func() { _ = modelConn.Close() }()
	supportConn, err := grpc.NewClient(supportAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	must(err)
	defer func() { _ = supportConn.Close() }()
	egressConn, err := grpc.NewClient(egressAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	must(err)
	defer func() { _ = egressConn.Close() }()
	authConn, err := grpc.NewClient(authAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	must(err)
	defer func() { _ = authConn.Close() }()

	sessionServer, err := sessionapi.NewSessionServer(sessionapi.SessionConfig{
		Client:                directClient,
		APIReader:             directClient,
		RuntimeClient:         directClient,
		Namespace:             namespace,
		RuntimeNamespace:      runtimeNamespace,
		ProfileConn:           profileConn,
		ProviderConn:          providerConn,
		CLIRuntimeConn:        cliRuntimeConn,
		ModelConn:             modelConn,
		SupportConn:           supportConn,
		EgressConn:            egressConn,
		AuthConn:              authConn,
		Timeline:              timelineSink,
		RunOutputs:            runOutputs,
		ActionRetryPolicy:     actionRetryPolicy,
		SessionRepository:     sessionRepo,
		ActionStore:           actionStore,
		ActiveRunSlots:        activeRunSlots,
		ReconcileScheduler:    reconcileScheduler,
		AgentRunWorkflow:      agentRunWorkflow,
		CLIOutputSidecarImage: cliOutputSidecarImage,
	})
	must(err)
	reconcileScheduler.SetDispatcher(sessionServer)

	listener, err := net.Listen("tcp", addr)
	must(err)
	grpcServer := grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	managementv1.RegisterAgentSessionManagementServiceServer(grpcServer, sessionServer)

	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthv1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus(managementv1.AgentSessionManagementService_ServiceDesc.ServiceName, healthv1.HealthCheckResponse_SERVING)
	healthv1.RegisterHealthServer(grpcServer, healthServer)

	triggerHandler, err := sessionServer.NewTriggerHandler(slog.Default(), internalActionToken)
	must(err)
	if internalActionToken == "" {
		log.Printf("internal action endpoints are disabled (env PLATFORM_AGENT_RUNTIME_SERVICE_INTERNAL_ACTION_TOKEN is empty)")
	}
	httpServer := &http.Server{
		Addr:              httpAddr,
		Handler:           triggerHandler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := manager.Start(ctx); err != nil && ctx.Err() == nil {
			log.Printf("agent run controller manager stopped: %v", err)
			cancel()
		}
	}()
	if domainEventsNATSURL != "" {
		publisher, err := domainevents.NewPublisher(outbox, domainevents.PublisherConfig{
			NATSURL:    domainEventsNATSURL,
			ClientName: "platform-agent-runtime-service-domain-publisher",
		})
		must(err)
		go func() { _ = publisher.Run(ctx) }()
		must(sessionServer.StartDomainEventConsumers(ctx, statePool, domainEventsNATSURL))
		log.Printf("domain event bus enabled (nats=%s)", domainEventsNATSURL)
	}
	if timelineNATSURL != "" {
		go func() {
			if err := sessionServer.RunTerminalResultProjector(ctx, timelineNATSURL); err != nil && ctx.Err() == nil {
				log.Printf("run terminal result projector stopped: %v", err)
			}
		}()
		log.Printf("run terminal result projector enabled (nats)")
		go func() {
			if err := sessionServer.RunOutputMessageProjector(ctx, timelineNATSURL); err != nil && ctx.Err() == nil {
				log.Printf("run output message projector stopped: %v", err)
			}
		}()
		log.Printf("run output message projector enabled (nats)")
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down platform-agent-runtime-service...")
		healthServer.SetServingStatus("", healthv1.HealthCheckResponse_NOT_SERVING)
		healthServer.SetServingStatus(managementv1.AgentSessionManagementService_ServiceDesc.ServiceName, healthv1.HealthCheckResponse_NOT_SERVING)
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = httpServer.Shutdown(shutdownCtx)
		grpcServer.GracefulStop()
	}()

	go func() {
		log.Printf("platform-agent-runtime-service HTTP trigger listening on %s", httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			must(err)
		}
	}()

	log.Printf("platform-agent-runtime-service listening on %s (namespace=%s runtime_namespace=%s profile=%s provider=%s cli_runtime=%s model=%s)", addr, namespace, runtimeNamespace, profileAddr, providerAddr, cliRuntimeAddr, modelAddr)
	if actionRetryPolicy != nil {
		log.Printf(
			"platform-agent-runtime-service action retry policy override: max_retries=%d base_backoff=%s max_backoff=%s",
			actionRetryPolicy.MaxRetries,
			actionRetryPolicy.BaseBackoff,
			actionRetryPolicy.MaxBackoff,
		)
	}
	if err := grpcServer.Serve(listener); err != nil {
		must(err)
	}
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func actionRetryPolicyFromEnv() (*agentsessionactions.RetryPolicy, error) {
	maxRetriesRaw := strings.TrimSpace(os.Getenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_MAX_RETRIES"))
	baseBackoffRaw := strings.TrimSpace(os.Getenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_BASE_BACKOFF"))
	maxBackoffRaw := strings.TrimSpace(os.Getenv("PLATFORM_AGENT_RUNTIME_SERVICE_ACTION_RETRY_MAX_BACKOFF"))
	if maxRetriesRaw == "" && baseBackoffRaw == "" && maxBackoffRaw == "" {
		return nil, nil
	}
	policy := agentsessionactions.RetryPolicy{}
	if maxRetriesRaw != "" {
		value, err := strconv.ParseInt(maxRetriesRaw, 10, 32)
		if err != nil {
			return nil, err
		}
		policy.MaxRetries = int32(value)
	}
	if baseBackoffRaw != "" {
		value, err := time.ParseDuration(baseBackoffRaw)
		if err != nil {
			return nil, err
		}
		policy.BaseBackoff = value
	}
	if maxBackoffRaw != "" {
		value, err := time.ParseDuration(maxBackoffRaw)
		if err != nil {
			return nil, err
		}
		policy.MaxBackoff = value
	}
	return &policy, nil
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
