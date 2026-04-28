package sessionapi

import (
	"crypto/sha1"
	"fmt"
	"strings"

	"code-code.internal/platform-k8s/internal/agentruntime/workflows"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func buildPrepareJob(namespace, name string, body prepareAgentRunJobTriggerRequest) *batchv1.Job {
	labels := prepareJobLabels(body)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec: batchv1.JobSpec{
			BackoffLimit:            int32Ptr(0),
			ActiveDeadlineSeconds:   int64Ptr(3600),
			TTLSecondsAfterFinished: int32Ptr(300),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyNever,
					AutomountServiceAccountToken: boolPtr(false),
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup:             int64Ptr(1000),
						FSGroupChangePolicy: fsGroupChangePolicyPtr(corev1.FSGroupChangeOnRootMismatch),
					},
					Containers:                    []corev1.Container{prepareContainer(body)},
					Volumes:                       prepareVolumes(body),
					TerminationGracePeriodSeconds: int64Ptr(10),
				},
			},
		},
	}
}

func prepareContainer(body prepareAgentRunJobTriggerRequest) corev1.Container {
	return corev1.Container{
		Name:            "prepare",
		Image:           body.ContainerImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		WorkingDir:      body.RuntimeWorkspaceDir,
		Command:         []string{agentPrepareCommand},
		Env: []corev1.EnvVar{
			{Name: "AGENT_PREPARE_JOB_ID", Value: body.Job.JobID},
			{Name: "AGENT_PREPARE_CLI_ID", Value: body.Job.CLIID},
			{Name: "AGENT_PREPARE_JOB_TYPE", Value: body.Job.JobType},
			{Name: "AGENT_PREPARE_RUN_TYPE", Value: body.Job.RunType},
			{Name: "AGENT_PREPARE_CHANGE_KEY", Value: body.Job.ChangeKey},
			{Name: "AGENT_PREPARE_PARAMETERS_YAML", Value: body.Job.ParametersYAML},
			{Name: "AGENT_RUN_SESSION_ID", Value: body.SessionID},
			{Name: "AGENT_RUN_ID", Value: body.RunID},
			{Name: "AGENT_RUN_RUNTIME_URL", Value: body.RuntimeURL},
			{Name: "AGENT_RUN_AUTH_MATERIALIZATION_KEY", Value: body.AuthMaterializationKey},
			{Name: "AGENT_RUN_PROVIDER_SURFACE_BINDING_ID", Value: body.ProviderSurfaceBindingID},
			{Name: "HOME", Value: body.RuntimeDataDir},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "workspace", MountPath: body.RuntimeWorkspaceDir},
			{Name: "home-state", MountPath: body.RuntimeDataDir},
		},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: boolPtr(false),
			RunAsNonRoot:             boolPtr(true),
			RunAsUser:                int64Ptr(1000),
			RunAsGroup:               int64Ptr(1000),
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
	}
}

func prepareVolumes(body prepareAgentRunJobTriggerRequest) []corev1.Volume {
	return []corev1.Volume{
		{Name: "workspace", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: body.WorkspacePVCName}}},
		{Name: "home-state", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: body.HomeStatePVCName}}},
	}
}

func prepareJobLabels(body prepareAgentRunJobTriggerRequest) map[string]string {
	return map[string]string{
		"code-code.internal/runtime":             agentPrepareRuntimeLabel,
		"agentrun.code-code.internal/session-id": workflows.DNSLabelPart(body.SessionID, "session"),
		"agentrun.code-code.internal/run-id":     workflows.DNSLabelPart(body.RunID, "run"),
		"agentrun.code-code.internal/job-id":     workflows.DNSLabelPart(body.Job.JobID, "job"),
	}
}

func prepareJobName(runID, jobID string) string {
	base := workflows.DNSLabelPart(runID+"-"+jobID, "prepare")
	prefix := "agent-prepare-"
	if len(prefix)+len(base) <= 63 {
		return prefix + base
	}
	sum := sha1.Sum([]byte(runID + "/" + jobID))
	suffix := fmt.Sprintf("%x", sum[:5])
	limit := 63 - len(prefix) - len(suffix) - 1
	if limit < 1 {
		return prefix + suffix
	}
	return prefix + strings.Trim(base[:limit], "-") + "-" + suffix
}

func int32Ptr(value int32) *int32 { return &value }
func int64Ptr(value int64) *int64 { return &value }
func boolPtr(value bool) *bool    { return &value }

func fsGroupChangePolicyPtr(value corev1.PodFSGroupChangePolicy) *corev1.PodFSGroupChangePolicy {
	return &value
}
