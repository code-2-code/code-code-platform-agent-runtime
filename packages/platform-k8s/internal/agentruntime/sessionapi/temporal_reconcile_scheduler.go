package sessionapi

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"code-code.internal/platform-k8s/internal/platform/temporalruntime"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

const (
	TemporalTaskQueue = "platform-agent-runtime-service"

	AgentRuntimeReconcileWorkflowName = "platform.agentRuntime.reconcile"
	triggerReconcileActivityName      = "platform.agentRuntime.triggerReconcile"
)

// ReconcileActionDispatcher dispatches a named reconcile action with a JSON body.
// SessionServer implements this interface, enabling Temporal activities to call
// domain reconcilers directly instead of routing through HTTP self-callbacks.
type ReconcileActionDispatcher interface {
	DispatchReconcileAction(ctx context.Context, action string, body []byte) error
}

type TemporalReconcileSchedulerConfig struct {
	Client            client.Client
	TaskQueue         string
	Namespace         string
	PlatformNamespace string
}

type TemporalReconcileScheduler struct {
	client    client.Client
	taskQueue string

	// activities holds the registered activity instance for late-binding
	// the ReconcileActionDispatcher after SessionServer is created.
	activities *TemporalReconcileActivities
}

func NewTemporalReconcileScheduler(config TemporalReconcileSchedulerConfig) (*TemporalReconcileScheduler, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("platformk8s/sessionapi: temporal client is nil")
	}
	taskQueue := strings.TrimSpace(config.TaskQueue)
	if taskQueue == "" {
		taskQueue = TemporalTaskQueue
	}
	return &TemporalReconcileScheduler{
		client:    config.Client,
		taskQueue: taskQueue,
	}, nil
}

// SetDispatcher injects the ReconcileActionDispatcher (typically *SessionServer)
// into the reconcile activities. Must be called after the SessionServer is created
// and before any reconcile activity is executed.
func (s *TemporalReconcileScheduler) SetDispatcher(dispatcher ReconcileActionDispatcher) {
	if s != nil && s.activities != nil {
		s.activities.dispatcher = dispatcher
	}
}

func (s *TemporalReconcileScheduler) ScheduleReconcile(ctx context.Context, request ReconcileScheduleRequest) error {
	input, err := temporalReconcileInput(request)
	if err != nil {
		return err
	}
	_, err = s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:                       temporalReconcileWorkflowID(request, input.Action),
		TaskQueue:                s.taskQueue,
		WorkflowIDConflictPolicy: enumspb.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
	}, AgentRuntimeReconcileWorkflowName, input)
	return err
}

func RegisterTemporalWorkflows(worker worker.Worker, scheduler *TemporalReconcileScheduler) error {
	if worker == nil {
		return fmt.Errorf("platformk8s/sessionapi: temporal worker is nil")
	}
	if scheduler == nil {
		return fmt.Errorf("platformk8s/sessionapi: temporal reconcile scheduler is nil")
	}
	worker.RegisterWorkflowWithOptions(AgentRuntimeReconcileWorkflow, workflow.RegisterOptions{Name: AgentRuntimeReconcileWorkflowName})
	activities := &TemporalReconcileActivities{}
	scheduler.activities = activities
	worker.RegisterActivityWithOptions(activities.TriggerReconcile, activity.RegisterOptions{Name: triggerReconcileActivityName})
	return nil
}

type TemporalReconcileInput struct {
	Action string
	Body   []byte
	Delay  time.Duration
}

func AgentRuntimeReconcileWorkflow(ctx workflow.Context, input TemporalReconcileInput) error {
	delay := input.Delay
	if delay <= 0 {
		delay = defaultRequeueDelay
	}
	if err := workflow.Sleep(ctx, delay); err != nil {
		return err
	}
	options := workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval: time.Second,
			MaximumInterval: 30 * time.Second,
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, options)
	return workflow.ExecuteActivity(ctx, triggerReconcileActivityName, input).Get(ctx, nil)
}

type TemporalReconcileActivities struct {
	dispatcher ReconcileActionDispatcher
}

func (a *TemporalReconcileActivities) TriggerReconcile(ctx context.Context, input TemporalReconcileInput) error {
	if a == nil || a.dispatcher == nil {
		return fmt.Errorf("platformk8s/sessionapi: reconcile action dispatcher is not initialized")
	}
	return a.dispatcher.DispatchReconcileAction(ctx, input.Action, input.Body)
}

func temporalReconcileInput(request ReconcileScheduleRequest) (TemporalReconcileInput, error) {
	action := strings.TrimSpace(request.Action)
	if action == "" || strings.Contains(action, "/") {
		return TemporalReconcileInput{}, fmt.Errorf("platformk8s/sessionapi: reconcile action is invalid")
	}
	body := bytes.TrimSpace(request.Body)
	if len(body) == 0 {
		body = []byte("{}")
	}
	return TemporalReconcileInput{Action: action, Body: body, Delay: request.Delay}, nil
}

func temporalReconcileWorkflowID(request ReconcileScheduleRequest, action string) string {
	return "agent-runtime-reconcile-" + temporalruntime.IDPart(request.OwnerKind+"-"+request.OwnerID+"-"+action, "runtime")
}
