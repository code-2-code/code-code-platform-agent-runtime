package agentruns

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"code-code.internal/platform-k8s/internal/platform/httpauth"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type TemporalActivities struct {
	runtimeClient          ctrlclient.Client
	runtimeNamespace       string
	triggerHTTPBaseURL     string
	triggerHTTPActionToken string
}

func (a *TemporalActivities) TriggerRunAction(ctx context.Context, input RunActionInput) error {
	url := strings.TrimRight(a.triggerHTTPBaseURL, "/") + "/" + strings.TrimSpace(input.Action)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(input.Body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	httpauth.SetBearerAuthorization(request, a.triggerHTTPActionToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("platformk8s/agentruns: trigger %q returned %s", input.Action, response.Status)
	}
	return nil
}

func (a *TemporalActivities) ExecuteRunJob(ctx context.Context, input TemporalWorkflowInput) error {
	if input.Run == nil || input.Run.Spec.Run == nil {
		return fmt.Errorf("platformk8s/agentruns: temporal run input is nil")
	}
	name := workflowNameFor(input.Run)
	job := a.executeJob(input, name)
	current := &batchv1.Job{}
	err := a.runtimeClient.Get(ctx, types.NamespacedName{Namespace: input.RuntimeNS, Name: name}, current)
	if apierrors.IsNotFound(err) {
		err = a.runtimeClient.Create(ctx, job)
	}
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return a.waitJob(ctx, input.RuntimeNS, name)
}

func (a *TemporalActivities) waitJob(ctx context.Context, namespace, name string) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		job := &batchv1.Job{}
		if err := a.runtimeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, job); err != nil {
			return err
		}
		if job.Status.Succeeded > 0 {
			return nil
		}
		for _, condition := range job.Status.Conditions {
			if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
				return fmt.Errorf("platformk8s/agentruns: run job %q failed: %s", name, condition.Message)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (a *TemporalActivities) executeJob(input TemporalWorkflowInput, name string) *batchv1.Job {
	run := input.Run.Spec.Run
	binding := providerRunBinding(run.GetAuthRequirement())
	labels := map[string]string{
		"code-code.internal/runtime":                "agent-run",
		"platform.code-code.internal/workflow-kind": agentRunWorkflowKind,
		sessionIDLabelKey:                           run.GetSessionId(),
		runIDLabelKey:                               run.GetRunId(),
		resourceNameLabelKey:                        input.Run.Name,
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: input.RuntimeNS, Labels: labels},
		Spec: batchv1.JobSpec{
			BackoffLimit:            int32Ptr(0),
			TTLSecondsAfterFinished: int32Ptr(86400),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: boolPtr(false),
					RestartPolicy:                corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:        boolPtr(true),
						RunAsUser:           int64Ptr(1000),
						RunAsGroup:          int64Ptr(1000),
						FSGroup:             int64Ptr(1000),
						FSGroupChangePolicy: fsGroupPolicyPtr(corev1.FSGroupChangeOnRootMismatch),
					},
					Volumes: []corev1.Volume{
						{Name: "cli-output-run", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: resourcePtr("1Gi")}}},
						{Name: "workspace", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: agentsessionWorkspacePVCName(run)}}},
						{Name: "home-state", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: agentsessionHomePVCName(run)}}},
						egressTrustCoreVolume(),
					},
					Containers: []corev1.Container{
						executeRunContainer(run, binding),
						cliOutputContainer(run, input.CLIOutputSidecarImage),
					},
				},
			},
		},
	}
}
