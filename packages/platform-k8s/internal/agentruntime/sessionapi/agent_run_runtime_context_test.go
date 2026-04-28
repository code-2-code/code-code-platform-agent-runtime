package sessionapi

import (
	"context"
	"testing"
	"time"

	resultv1 "code-code.internal/go-contract/agent/result/v1"
	apiprotocolv1 "code-code.internal/go-contract/api_protocol/v1"
	credentialv1 "code-code.internal/go-contract/credential/v1"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	authv1 "code-code.internal/go-contract/platform/auth/v1"
	egressservicev1 "code-code.internal/go-contract/platform/egress/v1"
	managementv1 "code-code.internal/go-contract/platform/management/v1"
	supportv1 "code-code.internal/go-contract/platform/support/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentexecution"
	"code-code.internal/platform-k8s/internal/agentruntime/agentruns"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolveAgentRunRuntimeContextByPodUsesRunLabel(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "code-code-runs",
			Name:      "run-1-pod",
			UID:       types.UID("pod-uid-1"),
			Labels:    map[string]string{agentruns.RunIDLabelKey: "run-1"},
		},
		Status: corev1.PodStatus{PodIP: "10.0.0.12"},
	}
	runs := &runtimeContextAgentRunService{runs: map[string]*agentrunv1.AgentRunState{
		"run-1": testAgentRunState("run-1"),
	}}
	server := &SessionServer{
		runtimeNamespace: "code-code-runs",
		runtimeClient:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build(),
		agentRuns:        runs,
		runtimeCatalog:   runtimeContextRuntimeCatalog{},
		auth:             runtimeContextAuthClient{},
		support:          runtimeContextSupportClient{},
		egress:           runtimeContextEgressClient{},
	}

	response, err := server.ResolveAgentRunRuntimeContext(context.Background(), &managementv1.ResolveAgentRunRuntimeContextRequest{
		Source: &managementv1.ResolveAgentRunRuntimeContextRequest_Pod{Pod: &managementv1.AgentRunPodRef{
			Namespace: "code-code-runs",
			Name:      "run-1-pod",
			Uid:       "pod-uid-1",
			Ip:        "10.0.0.12",
		}},
	})
	if err != nil {
		t.Fatalf("ResolveAgentRunRuntimeContext() error = %v", err)
	}
	if got, want := runs.getID, "run-1"; got != want {
		t.Fatalf("agentRuns.Get id = %q, want %q", got, want)
	}
	if got, want := response.GetRun().GetSpec().GetAuthRequirement().GetProviderRunBinding().GetRuntimeCliId(), "codex"; got != want {
		t.Fatalf("runtime_cli_id = %q, want %q", got, want)
	}
	if got, want := response.GetMetadata().GetCliId(), "codex"; got != want {
		t.Fatalf("metadata.cli_id = %q, want %q", got, want)
	}
	if got, want := response.GetMetadata().GetProviderId(), "provider-account-1"; got != want {
		t.Fatalf("metadata.provider_id = %q, want %q", got, want)
	}
	if got, want := response.GetMetadata().GetProtocol(), apiprotocolv1.Protocol_PROTOCOL_OPENAI_COMPATIBLE; got != want {
		t.Fatalf("metadata.protocol = %s, want %s", got.String(), want.String())
	}
	if got, want := response.GetMetadata().GetModelId(), "gpt-5"; got != want {
		t.Fatalf("metadata.model_id = %q, want %q", got, want)
	}
	if got, want := response.GetMetadata().GetImageId(), "registry.local/codex-runtime:gpt5"; got != want {
		t.Fatalf("metadata.image_id = %q, want %q", got, want)
	}
	if got, want := response.GetMetadata().GetCredentialId(), "cred-1"; got != want {
		t.Fatalf("metadata.credential_id = %q, want %q", got, want)
	}
	if got, want := response.GetMetadata().GetTargetHosts(), []string{"api.example.test"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("metadata.target_hosts = %v, want %v", got, want)
	}
	if got, want := response.GetMetadata().GetTargetPathPrefixes(), []string{"/v1", "/oauth2"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("metadata.target_path_prefixes = %v, want %v", got, want)
	}
	if got, want := response.GetMetadata().GetRequestHeaderNames(), []string{"authorization", "cookie"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("metadata.request_header_names = %v, want %v", got, want)
	}
	if got := response.GetMetadata().GetResponseHeaderReplacementRules(); len(got) != 1 || got[0].GetHeaderName() != "x-session-id" {
		t.Fatalf("metadata.response_header_replacement_rules = %#v", got)
	}
	if got, want := response.GetPod().GetUid(), "pod-uid-1"; got != want {
		t.Fatalf("pod.uid = %q, want %q", got, want)
	}
}

func TestResolveAgentRunRuntimeContextByRunIDReturnsFrozenRun(t *testing.T) {
	t.Parallel()

	runs := &runtimeContextAgentRunService{runs: map[string]*agentrunv1.AgentRunState{
		"run-2": testAgentRunState("run-2"),
	}}
	server := &SessionServer{agentRuns: runs}

	response, err := server.ResolveAgentRunRuntimeContext(context.Background(), &managementv1.ResolveAgentRunRuntimeContextRequest{
		Source: &managementv1.ResolveAgentRunRuntimeContextRequest_RunId{RunId: "run-2"},
	})
	if err != nil {
		t.Fatalf("ResolveAgentRunRuntimeContext() error = %v", err)
	}
	if got, want := response.GetRun().GetSpec().GetRunId(), "run-2"; got != want {
		t.Fatalf("run_id = %q, want %q", got, want)
	}
	if response.GetPod() != nil {
		t.Fatalf("pod = %#v, want nil", response.GetPod())
	}
	if got, want := response.GetMetadata().GetModelId(), "gpt-5"; got != want {
		t.Fatalf("metadata.model_id = %q, want %q", got, want)
	}
}

func TestResolveAgentRunRuntimeContextByWorkloadIDReturnsPodWhenPresent(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "code-code-runs",
			Name:              "run-3-old",
			Labels:            map[string]string{agentruns.RunIDLabelKey: "run-3"},
			CreationTimestamp: metav1.NewTime(time.Unix(1, 0)),
		},
	}
	newPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "code-code-runs",
			Name:              "run-3-new",
			UID:               types.UID("pod-uid-3"),
			Labels:            map[string]string{agentruns.RunIDLabelKey: "run-3"},
			CreationTimestamp: metav1.NewTime(time.Unix(2, 0)),
		},
		Status: corev1.PodStatus{PodIP: "10.0.0.13"},
	}
	runs := &runtimeContextAgentRunService{runs: map[string]*agentrunv1.AgentRunState{
		"run-3": testAgentRunState("run-3"),
	}}
	server := &SessionServer{
		runtimeNamespace: "code-code-runs",
		runtimeClient:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(oldPod, newPod).Build(),
		agentRuns:        runs,
	}

	response, err := server.ResolveAgentRunRuntimeContext(context.Background(), &managementv1.ResolveAgentRunRuntimeContextRequest{
		Source: &managementv1.ResolveAgentRunRuntimeContextRequest_WorkloadId{WorkloadId: "run-3"},
	})
	if err != nil {
		t.Fatalf("ResolveAgentRunRuntimeContext() error = %v", err)
	}
	if got, want := response.GetPod().GetName(), "run-3-new"; got != want {
		t.Fatalf("pod.name = %q, want %q", got, want)
	}
}

func TestResolveAgentRunRuntimeContextReturnsNotFoundForPodWithoutRunLabel(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "code-code-runs", Name: "orphan"}}
	server := &SessionServer{
		runtimeNamespace: "code-code-runs",
		runtimeClient:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build(),
		agentRuns:        &runtimeContextAgentRunService{},
	}

	if _, err := server.ResolveAgentRunRuntimeContext(context.Background(), &managementv1.ResolveAgentRunRuntimeContextRequest{
		Source: &managementv1.ResolveAgentRunRuntimeContextRequest_Pod{Pod: &managementv1.AgentRunPodRef{Name: "orphan"}},
	}); err == nil {
		t.Fatal("ResolveAgentRunRuntimeContext() error = nil, want not found error")
	} else if got, want := status.Code(err), codes.NotFound; got != want {
		t.Fatalf("ResolveAgentRunRuntimeContext() code = %v, want %v", got, want)
	}
}

type runtimeContextAgentRunService struct {
	getID string
	runs  map[string]*agentrunv1.AgentRunState
}

func (f *runtimeContextAgentRunService) Get(_ context.Context, runID string) (*agentrunv1.AgentRunState, error) {
	f.getID = runID
	return f.runs[runID], nil
}

func (f *runtimeContextAgentRunService) PublishTerminalResult(context.Context, string, *resultv1.RunResult) error {
	return nil
}

type runtimeContextRuntimeCatalog struct{}

func (runtimeContextRuntimeCatalog) ResolveContainerImage(context.Context, string, string) (*agentexecution.ContainerImage, error) {
	return nil, nil
}

func (runtimeContextRuntimeCatalog) GetProviderSurfaceBinding(_ context.Context, surfaceID string) (*agentexecution.SurfaceBindingProjection, error) {
	return &agentexecution.SurfaceBindingProjection{Surface: &providerv1.ProviderSurfaceBinding{
		SurfaceId:             surfaceID,
		ProviderCredentialRef: &providerv1.ProviderCredentialRef{ProviderCredentialId: "cred-1"},
		Runtime: &providerv1.ProviderSurfaceRuntime{
			Access: &providerv1.ProviderSurfaceRuntime_Api{Api: &providerv1.ProviderAPISurfaceRuntime{
				Protocol: apiprotocolv1.Protocol_PROTOCOL_OPENAI_COMPATIBLE,
				BaseUrl:  "https://api.example.test/v1",
			}},
		},
	}}, nil
}

func (runtimeContextRuntimeCatalog) GetCLI(context.Context, string) (*supportv1.CLI, error) {
	return nil, nil
}

type runtimeContextAuthClient struct{}

func (runtimeContextAuthClient) GetCredentialRuntimeProjection(context.Context, *authv1.GetCredentialRuntimeProjectionRequest, ...grpc.CallOption) (*authv1.GetCredentialRuntimeProjectionResponse, error) {
	return &authv1.GetCredentialRuntimeProjectionResponse{Credential: &authv1.CredentialRuntimeProjection{
		CredentialId:   "cred-1",
		CredentialKind: credentialv1.CredentialKind_CREDENTIAL_KIND_OAUTH,
		VendorId:       "openai",
		CliId:          "codex",
	}}, nil
}

func (runtimeContextAuthClient) GetEgressAuthPolicy(context.Context, *authv1.GetEgressAuthPolicyRequest, ...grpc.CallOption) (*authv1.GetEgressAuthPolicyResponse, error) {
	return &authv1.GetEgressAuthPolicyResponse{
		PolicyId:           "auth-policy-1",
		MaterializationKey: "codex.openai-oauth",
		RequestHeaderNames: []string{"authorization", "cookie"},
		HeaderValuePrefix:  "Bearer",
		ResponseReplacementRules: []*authv1.EgressSimpleReplacementRule{{
			HeaderName:  "x-session-id",
			MaterialKey: "session_id",
			Template:    "x-session-id=PLACEHOLDER",
		}},
	}, nil
}

type runtimeContextSupportClient struct{}

func (runtimeContextSupportClient) ResolveProviderCapabilities(context.Context, *supportv1.ResolveProviderCapabilitiesRequest, ...grpc.CallOption) (*supportv1.ResolveProviderCapabilitiesResponse, error) {
	return &supportv1.ResolveProviderCapabilitiesResponse{
		EgressPolicyId:         "egress-policy-1",
		AuthPolicyId:           "auth-policy-1",
		AuthMaterializationKey: "codex.openai-oauth",
	}, nil
}

type runtimeContextEgressClient struct{}

func (runtimeContextEgressClient) GetEgressRuntimePolicy(context.Context, *egressservicev1.GetEgressRuntimePolicyRequest, ...grpc.CallOption) (*egressservicev1.GetEgressRuntimePolicyResponse, error) {
	return &egressservicev1.GetEgressRuntimePolicyResponse{Policy: &egressservicev1.EgressRuntimePolicy{
		PolicyId:           "egress-policy-1",
		TargetHosts:        []string{"api.example.test"},
		TargetPathPrefixes: []string{"/v1", "/oauth2"},
	}}, nil
}

func testAgentRunState(runID string) *agentrunv1.AgentRunState {
	return &agentrunv1.AgentRunState{
		Spec: &agentrunv1.AgentRunSpec{
			RunId:          runID,
			SessionId:      "session-1",
			AgentRuntimeId: "codex",
			ContainerImage: "registry.local/codex-runtime:gpt5",
			AuthRequirement: &agentrunv1.AgentRunAuthRequirement{
				ProviderId:               "provider-account-1",
				ProviderSurfaceBindingId: "surface-1",
				AuthStatus:               "bound",
				RuntimeUrl:               "https://api.example.test/v1",
				MaterializationKey:       "codex.openai-oauth",
				ProviderRunBinding: &providerv1.ProviderRunBinding{
					ProviderId:         "provider-account-1",
					CredentialGrantRef: &credentialv1.CredentialGrantRef{GrantId: "cred-1"},
					RuntimeUrl:         "https://api.example.test/v1",
					MaterializationKey: "codex.openai-oauth",
					RuntimeCliId:       "codex",
					ProviderModelId:    "gpt-5",
					Access: &providerv1.ProviderRunBinding_Api{Api: &providerv1.ProviderRunAPIAccess{
						Protocol: apiprotocolv1.Protocol_PROTOCOL_OPENAI_COMPATIBLE,
					}},
				},
			},
		},
		Status: &agentrunv1.AgentRunStatus{Workload: &agentrunv1.WorkloadRef{WorkloadId: runID}},
	}
}
