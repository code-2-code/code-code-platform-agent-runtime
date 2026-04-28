package agentexecution

import (
	"context"
	"fmt"
	"strings"

	credentialv1 "code-code.internal/go-contract/credential/v1"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	clisupport "code-code.internal/platform-k8s/internal/supportservice/clidefinitions/support"
)

const authStatusBound = "bound"

// Resolution carries the execution input frozen for one next run.
type Resolution struct {
	ContainerImage    string
	CPURequest        string
	MemoryRequest     string
	AuthRequirement   *agentrunv1.AgentRunAuthRequirement
	RuntimeCandidates []*RuntimeCandidate
}

// RuntimeCandidate carries one resolved provider/model binding and matching
// auth input frozen for one run_turn snapshot.
type RuntimeCandidate struct {
	ResolvedProviderModel *providerv1.ResolvedProviderModel
	AuthRequirement       *agentrunv1.AgentRunAuthRequirement
}

// Resolver resolves one session's next-run execution input.
type Resolver struct {
	runtime RuntimeCatalog
	models  ModelRegistry
}

// NewResolver creates one execution resolver.
func NewResolver(runtime RuntimeCatalog, models ModelRegistry) (*Resolver, error) {
	if runtime == nil {
		return nil, fmt.Errorf("platformk8s/agentexecution: runtime catalog is nil")
	}
	if models == nil {
		return nil, fmt.Errorf("platformk8s/agentexecution: model registry is nil")
	}
	return &Resolver{
		runtime: runtime,
		models:  models,
	}, nil
}

// Resolve builds the execution image and auth requirement for one session.
func (r *Resolver) Resolve(ctx context.Context, session *platformv1alpha1.AgentSessionResource) (*Resolution, error) {
	if session == nil || session.Spec.Session == nil {
		return nil, validation("session is invalid")
	}
	image, err := r.runtime.ResolveContainerImage(ctx, session.Spec.Session.GetProviderId(), session.Spec.Session.GetExecutionClass())
	if err != nil {
		return nil, err
	}
	instance, err := r.loadPrimaryProviderSurfaceBinding(ctx, session)
	if err != nil {
		return nil, err
	}
	authRequirement, err := r.resolveAuthRequirement(ctx, session.Spec.Session.GetProviderId(), instance, providerv1.RuntimeBaseURL(instance.Surface.GetRuntime()))
	if err != nil {
		return nil, err
	}
	return &Resolution{
		ContainerImage:  image.Image,
		CPURequest:      image.CPURequest,
		MemoryRequest:   image.MemoryRequest,
		AuthRequirement: authRequirement,
	}, nil
}

func (r *Resolver) resolveAuthRequirement(ctx context.Context, providerID string, instance *SurfaceBindingProjection, runtimeURL string) (*agentrunv1.AgentRunAuthRequirement, error) {
	if instance == nil || instance.Surface == nil {
		return nil, validation("provider surface binding is invalid")
	}
	materializationKey, err := r.resolveMaterializationKey(ctx, providerID, instance)
	if err != nil {
		return nil, err
	}
	return &agentrunv1.AgentRunAuthRequirement{
		ProviderId:               strings.TrimSpace(providerID),
		ProviderSurfaceBindingId: instance.Surface.GetSurfaceId(),
		AuthStatus:               authStatusBound,
		RuntimeUrl:               strings.TrimSpace(runtimeURL),
		MaterializationKey:       materializationKey,
	}, nil
}

func (r *Resolver) resolveMaterializationKey(ctx context.Context, cliID string, instance *SurfaceBindingProjection) (string, error) {
	if r == nil || r.runtime == nil {
		return "", validation("runtime catalog is unavailable")
	}
	if instance == nil || instance.Surface == nil {
		return "", validation("provider surface binding is invalid")
	}
	cli, err := r.runtime.GetCLI(ctx, cliID)
	if err != nil {
		return "", err
	}
	protocol := providerv1.RuntimeProtocol(instance.Surface.GetRuntime())
	materialization, err := clisupport.ResolveAuthMaterialization(cli, credentialv1.CredentialKind_CREDENTIAL_KIND_API_KEY, protocol)
	if err != nil && cli.GetOauth() != nil {
		materialization, err = clisupport.ResolveAuthMaterialization(cli, credentialv1.CredentialKind_CREDENTIAL_KIND_OAUTH, protocol)
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(materialization.GetMaterializationKey()), nil
}

func (r *Resolver) loadPrimaryProviderSurfaceBinding(ctx context.Context, session *platformv1alpha1.AgentSessionResource) (*SurfaceBindingProjection, error) {
	if session == nil || session.Spec.Session == nil {
		return nil, validation("session is invalid")
	}
	instanceID := strings.TrimSpace(session.Spec.Session.GetRuntimeConfig().GetProviderRuntimeRef().GetSurfaceId())
	if instanceID == "" {
		return nil, validationf("session %q provider_surface_binding_id is empty", session.Spec.Session.GetSessionId())
	}
	return r.loadProviderSurfaceBindingByID(ctx, instanceID)
}

func (r *Resolver) loadProviderSurfaceBindingByID(ctx context.Context, instanceID string) (*SurfaceBindingProjection, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil, validation("provider surface binding id is empty")
	}
	resource, err := r.runtime.GetProviderSurfaceBinding(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if resource.Surface == nil {
		return nil, validationf("provider surface binding %q is missing payload", instanceID)
	}
	if resource.Surface.GetRuntime() == nil || providerv1.RuntimeKind(resource.Surface.GetRuntime()) == providerv1.ProviderSurfaceKind_PROVIDER_SURFACE_KIND_UNSPECIFIED {
		return nil, validationf("provider surface binding %q runtime is missing", instanceID)
	}
	if strings.TrimSpace(resource.Surface.GetProviderCredentialRef().GetProviderCredentialId()) == "" {
		return nil, validationf("provider surface binding %q auth binding is empty", instanceID)
	}
	return resource, nil
}
