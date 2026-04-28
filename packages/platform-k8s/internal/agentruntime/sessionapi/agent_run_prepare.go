package sessionapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"code-code.internal/platform-k8s/internal/platform/triggerhttp"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	agentPrepareRuntimeLabel = "agent-run-prepare"
	agentPrepareCommand      = "/usr/local/bin/agent-prepare.sh"
	agentPreparePollInterval = 2 * time.Second
)

type prepareAgentRunJobTriggerRequest struct {
	RuntimeNamespace         string                    `json:"runtimeNamespace"`
	SessionID                string                    `json:"sessionId"`
	RunID                    string                    `json:"runId"`
	AgentRunResourceName     string                    `json:"agentRunResourceName"`
	WorkspacePVCName         string                    `json:"workspacePVCName"`
	HomeStatePVCName         string                    `json:"homeStatePVCName"`
	RuntimeWorkspaceDir      string                    `json:"runtimeWorkspaceDir"`
	RuntimeDataDir           string                    `json:"runtimeDataDir"`
	ContainerImage           string                    `json:"containerImage"`
	ProviderID               string                    `json:"providerId"`
	ProviderSurfaceBindingID string                    `json:"providerSurfaceBindingId"`
	RuntimeURL               string                    `json:"runtimeUrl"`
	AuthMaterializationKey   string                    `json:"authMaterializationKey"`
	Job                      prepareAgentRunJobPayload `json:"job"`
}

type prepareAgentRunJobPayload struct {
	JobID          string `json:"jobId"`
	CLIID          string `json:"cliId"`
	JobType        string `json:"jobType"`
	RunType        string `json:"runType"`
	ChangeKey      string `json:"changeKey"`
	Cleanup        bool   `json:"cleanup"`
	ParametersYAML string `json:"parametersYaml"`
}

type cleanupAgentRunTriggerRequest struct {
	RuntimeNamespace     string                      `json:"runtimeNamespace"`
	SessionID            string                      `json:"sessionId"`
	RunID                string                      `json:"runId"`
	AgentRunResourceName string                      `json:"agentRunResourceName"`
	Jobs                 []prepareAgentRunJobPayload `json:"jobs"`
}

func (s *SessionServer) handlePrepareAgentRunJobTrigger(ctx context.Context, request triggerhttp.Request) (any, error) {
	var body prepareAgentRunJobTriggerRequest
	if err := request.DecodeJSON(&body); err != nil {
		return nil, err
	}
	if err := validatePrepareAgentRunJob(body); err != nil {
		return nil, err
	}
	namespace := firstNonEmpty(body.RuntimeNamespace, s.runtimeNamespace)
	name := prepareJobName(body.RunID, body.Job.JobID)
	if err := s.validateAgentRunAuthProjection(ctx, body); err != nil {
		return nil, err
	}
	if err := s.runtimeClient.Create(ctx, buildPrepareJob(namespace, name, body)); err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("create prepare job %q: %w", name, err)
	}
	if err := s.waitPrepareJob(ctx, namespace, name); err != nil {
		return nil, err
	}
	return map[string]string{"job": name, "phase": "succeeded"}, nil
}

func (s *SessionServer) handleCleanupAgentRunTrigger(ctx context.Context, request triggerhttp.Request) (any, error) {
	var body cleanupAgentRunTriggerRequest
	if err := request.DecodeJSON(&body); err != nil {
		return nil, err
	}
	namespace := firstNonEmpty(body.RuntimeNamespace, s.runtimeNamespace)
	deleted := 0
	for _, item := range body.Jobs {
		current := &batchv1.Job{}
		name := prepareJobName(body.RunID, item.JobID)
		if err := s.runtimeClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, current); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		if err := s.runtimeClient.Delete(ctx, current); err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
		deleted++
	}
	return map[string]any{"deleted": deleted}, nil
}

func validatePrepareAgentRunJob(body prepareAgentRunJobTriggerRequest) error {
	switch {
	case strings.TrimSpace(body.RunID) == "":
		return fmt.Errorf("runId is required")
	case strings.TrimSpace(body.SessionID) == "":
		return fmt.Errorf("sessionId is required")
	case strings.TrimSpace(body.ContainerImage) == "":
		return fmt.Errorf("containerImage is required")
	case strings.TrimSpace(body.WorkspacePVCName) == "":
		return fmt.Errorf("workspacePVCName is required")
	case strings.TrimSpace(body.HomeStatePVCName) == "":
		return fmt.Errorf("homeStatePVCName is required")
	case strings.TrimSpace(body.Job.JobID) == "":
		return fmt.Errorf("job.jobId is required")
	case strings.TrimSpace(body.Job.JobType) == "":
		return fmt.Errorf("job.jobType is required")
	default:
		return nil
	}
}

func (s *SessionServer) waitPrepareJob(ctx context.Context, namespace, name string) error {
	ticker := time.NewTicker(agentPreparePollInterval)
	defer ticker.Stop()
	for {
		job := &batchv1.Job{}
		if err := s.runtimeClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, job); err != nil {
			return err
		}
		for _, condition := range job.Status.Conditions {
			if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
				return nil
			}
			if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
				return fmt.Errorf("prepare job %q failed: %s", name, condition.Message)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
