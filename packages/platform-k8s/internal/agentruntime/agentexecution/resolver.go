package agentexecution

import (
	"context"
	"fmt"
	"strings"

	apiprotocolv1 "code-code.internal/go-contract/api_protocol/v1"
	credentialv1 "code-code.internal/go-contract/credential/v1"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
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
	instance, err := r.loadPrimaryProvider(ctx, session)
	if err != nil {
		return nil, err
	}
	authRequirement, err := r.resolveAuthRequirement(ctx, session.Spec.Session.GetProviderId(), instance, nil)
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

func (r *Resolver) resolveAuthRequirement(ctx context.Context, providerID string, instance *ProviderProjection, resolvedModel *providerv1.ResolvedProviderModel) (*agentrunv1.AgentRunAuthRequirement, error) {
	if instance == nil || instance.Provider == nil {
		return nil, validation("provider surface is invalid")
	}
	materializationKey, err := r.resolveMaterializationKey(ctx, providerID, instance)
	if err != nil {
		return nil, err
	}
	credentialID := strings.TrimSpace(instance.Provider.GetProviderCredentialRef().GetProviderCredentialId())
	binding := &agentrunv1.AgentRunProviderBinding{
		ProviderId:         strings.TrimSpace(instance.Provider.GetProviderId()),
		CredentialGrantRef: &credentialv1.CredentialGrantRef{GrantId: credentialID},
		Endpoint:           cloneProviderEndpoint(instance.Endpoint),
		MaterializationKey: materializationKey,
	}
	if resolvedModel != nil {
		binding.ProviderModelId = strings.TrimSpace(resolvedModel.GetProviderModelId())
		binding.CanonicalModelId = strings.TrimSpace(resolvedModel.GetModel().GetModelId())
		binding.SourceModelId = strings.TrimSpace(resolvedModel.GetModel().GetEffectiveDefinition().GetModelId())
	}
	return &agentrunv1.AgentRunAuthRequirement{
		ProviderId:         strings.TrimSpace(providerID),
		SurfaceId:          instance.Provider.GetSurfaceId(),
		AuthStatus:         authStatusBound,
		EndpointUrl:        endpointURL(instance.Endpoint),
		MaterializationKey: materializationKey,
		ProviderBinding:    binding,
	}, nil
}

func (r *Resolver) resolveMaterializationKey(ctx context.Context, cliID string, instance *ProviderProjection) (string, error) {
	if r == nil || r.runtime == nil {
		return "", validation("runtime catalog is unavailable")
	}
	if instance == nil || instance.Provider == nil {
		return "", validation("provider surface is invalid")
	}
	cli, err := r.runtime.GetCLI(ctx, cliID)
	if err != nil {
		return "", err
	}
	return materializationKeyForEndpoint(cli.GetCliId(), instance.Endpoint), nil
}

func (r *Resolver) loadPrimaryProvider(ctx context.Context, session *platformv1alpha1.AgentSessionResource) (*ProviderProjection, error) {
	if session == nil || session.Spec.Session == nil {
		return nil, validation("session is invalid")
	}
	instanceID := strings.TrimSpace(session.Spec.Session.GetRuntimeConfig().GetProviderId())
	if instanceID == "" {
		return nil, validationf("session %q provider_id is empty", session.Spec.Session.GetSessionId())
	}
	return r.loadProvider(ctx, instanceID)
}

func (r *Resolver) loadProvider(ctx context.Context, instanceID string) (*ProviderProjection, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil, validation("provider id is empty")
	}
	resource, err := r.runtime.GetProvider(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if resource.Provider == nil {
		return nil, validationf("provider %q is missing payload", instanceID)
	}
	if resource.Endpoint == nil {
		if endpoint := sessionEndpointPlaceholder(resource.Provider); endpoint != nil {
			resource.Endpoint = endpoint
		}
	}
	if resource.Endpoint == nil {
		return nil, validationf("provider %q endpoint is missing", instanceID)
	}
	if strings.TrimSpace(resource.Provider.GetProviderCredentialRef().GetProviderCredentialId()) == "" {
		return nil, validationf("provider %q auth binding is empty", instanceID)
	}
	return resource, nil
}

func endpointURL(endpoint *providerv1.ProviderEndpoint) string {
	return providerv1.EndpointBaseURL(endpoint)
}

func materializationKeyForEndpoint(cliID string, endpoint *providerv1.ProviderEndpoint) string {
	cliID = strings.TrimSpace(cliID)
	if cliID == "" {
		return ""
	}
	protocol := providerv1.EndpointProtocol(endpoint)
	switch protocol {
	case apiprotocolv1.Protocol_PROTOCOL_OPENAI_COMPATIBLE:
		return cliID + ".openai-compatible-api-key"
	case apiprotocolv1.Protocol_PROTOCOL_OPENAI_RESPONSES:
		return cliID + ".openai-responses-api-key"
	case apiprotocolv1.Protocol_PROTOCOL_GEMINI:
		return cliID + ".gemini-api-key"
	case apiprotocolv1.Protocol_PROTOCOL_ANTHROPIC:
		return cliID + ".anthropic-api-key"
	default:
		if providerv1.EndpointCLIID(endpoint) != "" {
			return cliID + ".oauth"
		}
		return cliID + ".api-key"
	}
}

func sessionEndpointPlaceholder(provider *providerv1.Provider) *providerv1.ProviderEndpoint {
	return nil
}
