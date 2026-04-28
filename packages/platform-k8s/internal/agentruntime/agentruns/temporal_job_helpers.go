package agentruns

import (
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentsessions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func executeRunContainer(run *agentrunv1.AgentRunSpec, binding providerBindingView) corev1.Container {
	return corev1.Container{
		Name:            "execute",
		Image:           run.GetContainerImage(),
		ImagePullPolicy: corev1.PullIfNotPresent,
		WorkingDir:      run.GetRuntimeEnvironment().GetWorkspaceDir(),
		Command:         []string{"/usr/local/bin/agent-entrypoint.sh"},
		Env: append([]corev1.EnvVar{
			{Name: "AGENT_RUN_PROMPT", Value: requestPrompt(run.GetRequest())},
			{Name: "AGENT_RUN_MODEL", Value: requestModel(run.GetRequest())},
			{Name: "AGENT_RUN_RUNTIME_URL", Value: binding.RuntimeURL()},
			{Name: "AGENT_RUN_AUTH_MATERIALIZATION_KEY", Value: binding.MaterializationKey()},
			{Name: "AGENT_RUN_RUNTIME_CLI_ID", Value: binding.RuntimeCLIID()},
			{Name: "HOME", Value: run.GetRuntimeEnvironment().GetDataDir()},
			{Name: "XDG_DATA_HOME", Value: run.GetRuntimeEnvironment().GetDataDir() + "/.local/share"},
			{Name: "XDG_CONFIG_HOME", Value: run.GetRuntimeEnvironment().GetDataDir() + "/.config"},
			{Name: "XDG_CACHE_HOME", Value: run.GetRuntimeEnvironment().GetDataDir() + "/.cache"},
		}, egressTrustCoreEnv()...),
		SecurityContext: nonRootCoreSecurityContext(false),
		VolumeMounts: []corev1.VolumeMount{
			{Name: "workspace", MountPath: run.GetRuntimeEnvironment().GetWorkspaceDir()},
			{Name: "home-state", MountPath: run.GetRuntimeEnvironment().GetDataDir()},
			{Name: "cli-output-run", MountPath: "/run/cli-output"},
			egressTrustCoreVolumeMount(),
		},
	}
}

func cliOutputContainer(run *agentrunv1.AgentRunSpec, image string) corev1.Container {
	return corev1.Container{
		Name:            "cli-output",
		Image:           firstNonEmpty(image, defaultCLIOutputSidecarImage),
		ImagePullPolicy: corev1.PullIfNotPresent,
		Env: []corev1.EnvVar{
			{Name: "CLI_OUTPUT_RUN_ID", Value: run.GetRunId()},
			{Name: "CLI_OUTPUT_SESSION_ID", Value: run.GetSessionId()},
			{Name: "CLI_OUTPUT_NATS_URL", Value: defaultCLIOutputNATSURL},
			{Name: "CLI_OUTPUT_WORK_DIR", Value: "/run/cli-output"},
			{Name: "CLI_OUTPUT_CLI_ID", Value: run.GetAgentRuntimeId()},
		},
		SecurityContext: nonRootCoreSecurityContext(true),
		VolumeMounts:    []corev1.VolumeMount{{Name: "cli-output-run", MountPath: "/run/cli-output"}},
	}
}

func agentsessionWorkspacePVCName(run *agentrunv1.AgentRunSpec) string {
	return agentsessions.WorkspacePVCName(run.GetSessionId(), run.GetWorkspaceId())
}

func agentsessionHomePVCName(run *agentrunv1.AgentRunSpec) string {
	return agentsessions.HomeStatePVCName(run.GetSessionId(), run.GetHomeStateId())
}

func egressTrustCoreVolume() corev1.Volume {
	return corev1.Volume{
		Name: "egress-trust-bundle",
		VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: "code-code-egress-trust-bundle"},
		}},
	}
}

func egressTrustCoreVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{Name: "egress-trust-bundle", MountPath: "/var/run/code-code-egress-trust", ReadOnly: true}
}

func egressTrustCoreEnv() []corev1.EnvVar {
	path := "/var/run/code-code-egress-trust/ca-certificates.crt"
	return []corev1.EnvVar{
		{Name: "SSL_CERT_FILE", Value: path},
		{Name: "REQUESTS_CA_BUNDLE", Value: path},
		{Name: "CURL_CA_BUNDLE", Value: path},
		{Name: "GIT_SSL_CAINFO", Value: path},
		{Name: "NODE_EXTRA_CA_CERTS", Value: path},
	}
}

func nonRootCoreSecurityContext(readonly bool) *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: boolPtr(false),
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		ReadOnlyRootFilesystem:   boolPtr(readonly),
		RunAsNonRoot:             boolPtr(true),
		RunAsUser:                int64Ptr(1000),
		RunAsGroup:               int64Ptr(1000),
		SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}

func resourcePtr(value string) *resource.Quantity {
	parsed := resource.MustParse(value)
	return &parsed
}

func boolPtr(value bool) *bool    { return &value }
func int32Ptr(value int32) *int32 { return &value }
func int64Ptr(value int64) *int64 { return &value }

func fsGroupPolicyPtr(value corev1.PodFSGroupChangePolicy) *corev1.PodFSGroupChangePolicy {
	return &value
}
