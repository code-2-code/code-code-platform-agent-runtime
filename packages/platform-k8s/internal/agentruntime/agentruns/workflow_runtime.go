package agentruns

import (
	"context"
	"strings"

	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/workflows"
)

const (
	defaultCLIOutputSidecarImage = "code-code/cli-output-sidecar:0.0.0"
	defaultCLIOutputNATSURL      = "nats://nats.code-code-infra.svc.cluster.local:4222"
	defaultTriggerHTTPBaseURL    = "http://platform-agent-runtime-service.code-code.svc.cluster.local:8080/internal/actions"
)

// WorkflowState carries one execution workload observation snapshot.
type WorkflowState = workflows.State

// WorkflowRuntime submits and reads AgentRun execution workloads.
type WorkflowRuntime interface {
	Submit(ctx context.Context, run *platformv1alpha1.AgentRunResource) (string, error)
	Get(ctx context.Context, name string) (*WorkflowState, error)
	Cancel(ctx context.Context, name string) error
	Delete(ctx context.Context, name string) error
	Cleanup(ctx context.Context, run *platformv1alpha1.AgentRunResource) error
}

func workflowNameFor(run *platformv1alpha1.AgentRunResource) string {
	if run == nil {
		return ""
	}
	return strings.TrimSpace(run.Name)
}
