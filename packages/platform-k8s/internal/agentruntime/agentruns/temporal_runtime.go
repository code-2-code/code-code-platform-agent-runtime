package agentruns

import (
	"context"
	"errors"
	"fmt"
	"strings"

	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	TemporalTaskQueue        = "platform-agent-runtime-service"
	AgentRunWorkflowName     = "platform.agentRun.execute"
	triggerRunActionActivity = "platform.agentRun.triggerAction"
	executeRunJobActivity    = "platform.agentRun.executeJob"
)

type TemporalWorkflowRuntimeConfig struct {
	TemporalClient         client.Client
	RuntimeClient          ctrlclient.Client
	ControlNamespace       string
	RuntimeNamespace       string
	TaskQueue              string
	CLIOutputSidecarImage  string
	TriggerHTTPBaseURL     string
	TriggerHTTPActionToken string
}

type TemporalWorkflowRuntime struct {
	temporalClient         client.Client
	runtimeClient          ctrlclient.Client
	controlNamespace       string
	runtimeNamespace       string
	taskQueue              string
	cliOutputSidecarImage  string
	triggerHTTPBaseURL     string
	triggerHTTPActionToken string
}

func NewTemporalWorkflowRuntime(config TemporalWorkflowRuntimeConfig) (*TemporalWorkflowRuntime, error) {
	if config.TemporalClient == nil {
		return nil, fmt.Errorf("platformk8s/agentruns: temporal client is nil")
	}
	if config.RuntimeClient == nil {
		return nil, fmt.Errorf("platformk8s/agentruns: runtime client is nil")
	}
	controlNamespace := firstNonEmpty(config.ControlNamespace, "code-code")
	runtimeNamespace := firstNonEmpty(config.RuntimeNamespace, controlNamespace)
	taskQueue := firstNonEmpty(config.TaskQueue, TemporalTaskQueue)
	baseURL := strings.TrimRight(strings.TrimSpace(config.TriggerHTTPBaseURL), "/")
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://platform-agent-runtime-service.%s.svc.cluster.local:8080/internal/actions", controlNamespace)
	}
	sidecarImage := firstNonEmpty(config.CLIOutputSidecarImage, defaultCLIOutputSidecarImage)
	return &TemporalWorkflowRuntime{
		temporalClient:         config.TemporalClient,
		runtimeClient:          config.RuntimeClient,
		controlNamespace:       controlNamespace,
		runtimeNamespace:       runtimeNamespace,
		taskQueue:              taskQueue,
		cliOutputSidecarImage:  sidecarImage,
		triggerHTTPBaseURL:     baseURL,
		triggerHTTPActionToken: strings.TrimSpace(config.TriggerHTTPActionToken),
	}, nil
}

func (r *TemporalWorkflowRuntime) Submit(ctx context.Context, run *platformv1alpha1.AgentRunResource) (string, error) {
	input, err := r.workflowInput(run)
	if err != nil {
		return "", err
	}
	workflowID := workflowNameFor(run)
	_, err = r.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:                       workflowID,
		TaskQueue:                r.taskQueue,
		WorkflowIDConflictPolicy: enumspb.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
	}, AgentRunWorkflowName, input)
	if err != nil {
		return "", err
	}
	return workflowID, nil
}

func (r *TemporalWorkflowRuntime) Get(ctx context.Context, name string) (*WorkflowState, error) {
	description, err := r.temporalClient.DescribeWorkflowExecution(ctx, strings.TrimSpace(name), "")
	if err != nil {
		if temporalNotFound(err) {
			return nil, workflowExecutionNotFound(name)
		}
		return nil, err
	}
	info := description.GetWorkflowExecutionInfo()
	return &WorkflowState{
		Phase:      temporalWorkflowPhase(info.GetStatus()),
		StartedAt:  timePtrFromProto(info.GetStartTime()),
		FinishedAt: timePtrFromProto(info.GetCloseTime()),
	}, nil
}

func (r *TemporalWorkflowRuntime) Cancel(ctx context.Context, name string) error {
	if err := r.temporalClient.CancelWorkflow(ctx, strings.TrimSpace(name), ""); err != nil {
		if temporalNotFound(err) {
			return workflowExecutionNotFound(name)
		}
		return err
	}
	return nil
}

func (r *TemporalWorkflowRuntime) Delete(context.Context, string) error { return nil }

func (r *TemporalWorkflowRuntime) Cleanup(context.Context, *platformv1alpha1.AgentRunResource) error {
	return nil
}

func (r *TemporalWorkflowRuntime) Register(worker worker.Worker) error {
	if worker == nil {
		return fmt.Errorf("platformk8s/agentruns: temporal worker is nil")
	}
	worker.RegisterWorkflowWithOptions(AgentRunWorkflow, workflow.RegisterOptions{Name: AgentRunWorkflowName})
	activities := &TemporalActivities{
		runtimeClient:          r.runtimeClient,
		runtimeNamespace:       r.runtimeNamespace,
		triggerHTTPBaseURL:     r.triggerHTTPBaseURL,
		triggerHTTPActionToken: r.triggerHTTPActionToken,
	}
	worker.RegisterActivityWithOptions(activities.TriggerRunAction, activity.RegisterOptions{Name: triggerRunActionActivity})
	worker.RegisterActivityWithOptions(activities.ExecuteRunJob, activity.RegisterOptions{Name: executeRunJobActivity})
	return nil
}

func (r *TemporalWorkflowRuntime) workflowInput(run *platformv1alpha1.AgentRunResource) (TemporalWorkflowInput, error) {
	if run == nil || run.Spec.Run == nil {
		return TemporalWorkflowInput{}, fmt.Errorf("platformk8s/agentruns: workflow run is nil")
	}
	if strings.TrimSpace(run.Spec.Run.GetContainerImage()) == "" {
		return TemporalWorkflowInput{}, fmt.Errorf("platformk8s/agentruns: workflow run container image is empty")
	}
	if run.Spec.Run.GetAuthRequirement() == nil {
		return TemporalWorkflowInput{}, fmt.Errorf("platformk8s/agentruns: workflow run auth requirement is empty")
	}
	config := agentRunWorkflowConfig{
		ExecutionNamespace:    r.runtimeNamespace,
		TriggerHTTPBaseURL:    r.triggerHTTPBaseURL,
		CLIOutputSidecarImage: r.cliOutputSidecarImage,
	}
	prepareBodies := make([][]byte, 0, len(run.Spec.Run.GetPrepareJobs()))
	for _, job := range run.Spec.Run.GetPrepareJobs() {
		body, err := prepareRequestBody(run, job, config)
		if err != nil {
			return TemporalWorkflowInput{}, err
		}
		prepareBodies = append(prepareBodies, []byte(body))
	}
	cleanupBody, err := cleanupRequestBody(run, config)
	if err != nil {
		return TemporalWorkflowInput{}, err
	}
	return TemporalWorkflowInput{
		Run:                   run.DeepCopy(),
		RuntimeNS:             r.runtimeNamespace,
		CLIOutputSidecarImage: r.cliOutputSidecarImage,
		PrepareBodies:         prepareBodies,
		CleanupBody:           []byte(cleanupBody),
	}, nil
}

type TemporalWorkflowInput struct {
	Run                   *platformv1alpha1.AgentRunResource
	RuntimeNS             string
	CLIOutputSidecarImage string
	PrepareBodies         [][]byte
	CleanupBody           []byte
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func temporalNotFound(err error) bool {
	var notFound *serviceerror.NotFound
	return errors.As(err, &notFound)
}

func workflowExecutionNotFound(name string) error {
	return apierrors.NewNotFound(schema.GroupResource{Group: "temporal.io", Resource: "workflows"}, strings.TrimSpace(name))
}
