package sessionapi

import (
	"context"
	"strings"

	"code-code.internal/go-contract/domainerror"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	managementv1 "code-code.internal/go-contract/platform/management/v1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentrunauth"
	"code-code.internal/platform-k8s/internal/agentruntime/agentruns"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *SessionServer) ResolveAgentRunRuntimeContext(ctx context.Context, request *managementv1.ResolveAgentRunRuntimeContextRequest) (*managementv1.ResolveAgentRunRuntimeContextResponse, error) {
	if request == nil {
		return nil, grpcError(domainerror.NewValidation("platformk8s/sessionapi: runtime context request is required"))
	}
	switch source := request.GetSource().(type) {
	case *managementv1.ResolveAgentRunRuntimeContextRequest_RunId:
		return s.resolveAgentRunRuntimeContextByRunID(ctx, source.RunId)
	case *managementv1.ResolveAgentRunRuntimeContextRequest_WorkloadId:
		return s.resolveAgentRunRuntimeContextByWorkloadID(ctx, source.WorkloadId)
	case *managementv1.ResolveAgentRunRuntimeContextRequest_Pod:
		return s.resolveAgentRunRuntimeContextByPod(ctx, source.Pod)
	default:
		return nil, grpcError(domainerror.NewValidation("platformk8s/sessionapi: runtime context source is required"))
	}
}

func (s *SessionServer) resolveAgentRunRuntimeContextByRunID(ctx context.Context, runID string) (*managementv1.ResolveAgentRunRuntimeContextResponse, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, grpcError(domainerror.NewValidation("platformk8s/sessionapi: run_id is required"))
	}
	run, err := s.agentRuns.Get(ctx, runID)
	if err != nil {
		return nil, grpcError(err)
	}
	pod, err := s.podForRun(ctx, runID)
	if err != nil {
		return nil, grpcError(err)
	}
	response, err := s.runtimeContextResponse(ctx, run, pod)
	if err != nil {
		return nil, grpcError(err)
	}
	return response, nil
}

func (s *SessionServer) resolveAgentRunRuntimeContextByWorkloadID(ctx context.Context, workloadID string) (*managementv1.ResolveAgentRunRuntimeContextResponse, error) {
	workloadID = strings.TrimSpace(workloadID)
	if workloadID == "" {
		return nil, grpcError(domainerror.NewValidation("platformk8s/sessionapi: workload_id is required"))
	}
	run, err := s.agentRuns.Get(ctx, workloadID)
	if err != nil {
		return nil, grpcError(err)
	}
	pod, err := s.podForRun(ctx, workloadID)
	if err != nil {
		return nil, grpcError(err)
	}
	response, err := s.runtimeContextResponse(ctx, run, pod)
	if err != nil {
		return nil, grpcError(err)
	}
	return response, nil
}

func (s *SessionServer) resolveAgentRunRuntimeContextByPod(ctx context.Context, ref *managementv1.AgentRunPodRef) (*managementv1.ResolveAgentRunRuntimeContextResponse, error) {
	pod, err := s.resolveRuntimePod(ctx, ref)
	if err != nil {
		return nil, grpcError(err)
	}
	runID := strings.TrimSpace(pod.GetLabels()[agentruns.RunIDLabelKey])
	if runID == "" {
		return nil, grpcError(domainerror.NewNotFound("platformk8s/sessionapi: runtime context not found for pod %s/%s", pod.Namespace, pod.Name))
	}
	run, err := s.agentRuns.Get(ctx, runID)
	if err != nil {
		return nil, grpcError(err)
	}
	response, err := s.runtimeContextResponse(ctx, run, pod)
	if err != nil {
		return nil, grpcError(err)
	}
	return response, nil
}

func (s *SessionServer) resolveRuntimePod(ctx context.Context, ref *managementv1.AgentRunPodRef) (*corev1.Pod, error) {
	if ref == nil {
		return nil, domainerror.NewValidation("platformk8s/sessionapi: pod ref is required")
	}
	namespace := firstNonEmptyString(ref.GetNamespace(), s.runtimeNamespace, s.namespace)
	name := strings.TrimSpace(ref.GetName())
	uid := strings.TrimSpace(ref.GetUid())
	ip := strings.TrimSpace(ref.GetIp())
	if namespace == "" {
		return nil, domainerror.NewValidation("platformk8s/sessionapi: pod namespace is required")
	}
	if s.runtimeClient == nil {
		return nil, domainerror.NewValidation("platformk8s/sessionapi: runtime client is unavailable")
	}
	if name != "" {
		pod := &corev1.Pod{}
		if err := s.runtimeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, pod); err != nil {
			return nil, err
		}
		if err := validatePodRef(pod, uid, ip); err != nil {
			return nil, err
		}
		return pod, nil
	}
	if uid == "" && ip == "" {
		return nil, domainerror.NewValidation("platformk8s/sessionapi: pod name, uid, or ip is required")
	}
	pods := &corev1.PodList{}
	if err := s.runtimeClient.List(ctx, pods, ctrlclient.InNamespace(namespace)); err != nil {
		return nil, err
	}
	var match *corev1.Pod
	for index := range pods.Items {
		pod := &pods.Items[index]
		if uid != "" && string(pod.UID) != uid {
			continue
		}
		if ip != "" && !podHasIP(pod, ip) {
			continue
		}
		if match != nil {
			return nil, domainerror.NewValidation("platformk8s/sessionapi: pod ref matched multiple pods in namespace %s", namespace)
		}
		match = pod
	}
	if match == nil {
		return nil, domainerror.NewNotFound("platformk8s/sessionapi: runtime pod not found in namespace %s", namespace)
	}
	return match, nil
}

func (s *SessionServer) podForRun(ctx context.Context, runID string) (*corev1.Pod, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" || s.runtimeClient == nil {
		return nil, nil
	}
	namespace := firstNonEmptyString(s.runtimeNamespace, s.namespace)
	if namespace == "" {
		return nil, nil
	}
	pods := &corev1.PodList{}
	err := s.runtimeClient.List(ctx, pods,
		ctrlclient.InNamespace(namespace),
		ctrlclient.MatchingLabels{agentruns.RunIDLabelKey: runID},
	)
	if err != nil {
		return nil, err
	}
	if len(pods.Items) == 0 {
		return nil, nil
	}
	selected := &pods.Items[0]
	for index := 1; index < len(pods.Items); index++ {
		candidate := &pods.Items[index]
		if candidate.CreationTimestamp.After(selected.CreationTimestamp.Time) {
			selected = candidate
		}
	}
	return selected, nil
}

func validatePodRef(pod *corev1.Pod, uid string, ip string) error {
	if pod == nil {
		return domainerror.NewValidation("platformk8s/sessionapi: pod is required")
	}
	if uid != "" && string(pod.UID) != uid {
		return domainerror.NewValidation("platformk8s/sessionapi: pod uid does not match")
	}
	if ip != "" && !podHasIP(pod, ip) {
		return domainerror.NewValidation("platformk8s/sessionapi: pod ip does not match")
	}
	return nil
}

func podHasIP(pod *corev1.Pod, ip string) bool {
	ip = strings.TrimSpace(ip)
	if pod == nil || ip == "" {
		return false
	}
	if strings.TrimSpace(pod.Status.PodIP) == ip {
		return true
	}
	for _, value := range pod.Status.PodIPs {
		if strings.TrimSpace(value.IP) == ip {
			return true
		}
	}
	return false
}

func podRefFromPod(pod *corev1.Pod) *managementv1.AgentRunPodRef {
	if pod == nil {
		return nil
	}
	return &managementv1.AgentRunPodRef{
		Namespace: strings.TrimSpace(pod.Namespace),
		Name:      strings.TrimSpace(pod.Name),
		Uid:       strings.TrimSpace(string(pod.UID)),
		Ip:        strings.TrimSpace(pod.Status.PodIP),
	}
}

func (s *SessionServer) runtimeContextResponse(ctx context.Context, run *agentrunv1.AgentRunState, pod *corev1.Pod) (*managementv1.ResolveAgentRunRuntimeContextResponse, error) {
	metadata, err := s.runtimeMetadataFromRun(ctx, run)
	if err != nil {
		return nil, err
	}
	return &managementv1.ResolveAgentRunRuntimeContextResponse{
		Run:      run,
		Pod:      podRefFromPod(pod),
		Metadata: metadata,
	}, nil
}

func (s *SessionServer) runtimeMetadataFromRun(ctx context.Context, run *agentrunv1.AgentRunState) (*managementv1.AgentRunRuntimeMetadata, error) {
	if run == nil || run.GetSpec() == nil {
		return nil, nil
	}
	metadata := &managementv1.AgentRunRuntimeMetadata{
		ProviderId: "",
		CliId:      strings.TrimSpace(run.GetSpec().GetAgentRuntimeId()),
		ImageId:    strings.TrimSpace(run.GetSpec().GetContainerImage()),
	}
	if binding := run.GetSpec().GetAuthRequirement().GetProviderRunBinding(); binding != nil {
		metadata.CliId = firstNonEmptyString(binding.GetRuntimeCliId(), run.GetSpec().GetAgentRuntimeId())
		metadata.ProviderId = firstNonEmptyString(binding.GetProviderId(), metadata.GetProviderId())
		metadata.CredentialId = strings.TrimSpace(binding.GetCredentialGrantRef().GetGrantId())
		metadata.RuntimeUrl = strings.TrimSpace(binding.GetRuntimeUrl())
		metadata.AuthMaterializationKey = strings.TrimSpace(binding.GetMaterializationKey())
		if api := binding.GetApi(); api != nil {
			metadata.Protocol = api.GetProtocol()
		}
		metadata.ModelId = firstNonEmptyString(binding.GetProviderModelId(), binding.GetCanonicalModelId(), binding.GetSourceModelId())
	}
	if err := s.addRuntimeAuthProjectionMetadata(ctx, run, metadata); err != nil {
		return nil, err
	}
	return metadata, nil
}

func (s *SessionServer) addRuntimeAuthProjectionMetadata(ctx context.Context, run *agentrunv1.AgentRunState, metadata *managementv1.AgentRunRuntimeMetadata) error {
	if run == nil || run.GetSpec() == nil || metadata == nil {
		return nil
	}
	authRequirement := run.GetSpec().GetAuthRequirement()
	surfaceID := strings.TrimSpace(authRequirement.GetProviderSurfaceBindingId())
	if surfaceID == "" || s.runtimeCatalog == nil || s.auth == nil || s.support == nil || s.egress == nil {
		return nil
	}
	projection, err := s.agentRunAuthProjection(ctx, prepareAgentRunJobTriggerRequest{
		SessionID:                run.GetSpec().GetSessionId(),
		RunID:                    run.GetSpec().GetRunId(),
		ProviderID:               firstNonEmptyString(authRequirement.GetProviderId(), run.GetSpec().GetProviderId(), metadata.GetProviderId()),
		ProviderSurfaceBindingID: surfaceID,
		RuntimeURL:               firstNonEmptyString(authRequirement.GetRuntimeUrl(), metadata.GetRuntimeUrl()),
		AuthMaterializationKey:   firstNonEmptyString(authRequirement.GetMaterializationKey(), metadata.GetAuthMaterializationKey()),
		Job: prepareAgentRunJobPayload{
			CLIID:   firstNonEmptyString(metadata.GetCliId(), run.GetSpec().GetAgentRuntimeId()),
			JobType: "auth",
		},
	})
	if err != nil {
		return err
	}
	agentrunauth.ApplyToRuntimeMetadata(metadata, projection)
	return nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
