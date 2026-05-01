package sessionapi

import (
	"context"
	"fmt"
	"strings"

	authv1 "code-code.internal/go-contract/platform/auth/v1"
	egressservicev1 "code-code.internal/go-contract/platform/egress/v1"
	managementv1 "code-code.internal/go-contract/platform/management/v1"
	supportv1 "code-code.internal/go-contract/platform/support/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentrunauth"
)

func (s *SessionServer) validateAgentRunAuthProjection(ctx context.Context, body prepareAgentRunJobTriggerRequest) error {
	if strings.TrimSpace(body.Job.JobType) != "auth" {
		return nil
	}
	_, err := s.agentRunAuthProjection(ctx, body)
	return err
}

func (s *SessionServer) agentRunAuthProjection(ctx context.Context, body prepareAgentRunJobTriggerRequest) (agentrunauth.Projection, error) {
	providerLookupID := firstNonEmptyString(body.ProviderID, body.SurfaceID)
	providerProjection, err := s.runtimeCatalog.GetProvider(ctx, providerLookupID)
	if err != nil {
		return agentrunauth.Projection{}, err
	}
	if providerProjection == nil || providerProjection.Provider == nil || providerProjection.Endpoint == nil {
		return agentrunauth.Projection{}, fmt.Errorf("platformk8s/sessionapi: provider %q endpoint is invalid", providerLookupID)
	}
	credentialID := strings.TrimSpace(providerProjection.Provider.GetProviderCredentialRef().GetProviderCredentialId())
	if credentialID == "" {
		return agentrunauth.Projection{}, fmt.Errorf("platformk8s/sessionapi: provider %q credential is empty", providerLookupID)
	}
	credentialResponse, err := s.auth.GetCredentialRuntimeProjection(ctx, &authv1.GetCredentialRuntimeProjectionRequest{
		CredentialId: credentialID,
	})
	if err != nil {
		return agentrunauth.Projection{}, err
	}
	credential := credentialResponse.GetCredential()
	if credential == nil {
		return agentrunauth.Projection{}, fmt.Errorf("platformk8s/sessionapi: credential %q runtime projection is empty", credentialID)
	}
	endpoint := providerProjection.Endpoint
	cliID := firstNonEmpty(body.Job.CLIID, body.ProviderID, credential.GetCliId(), providerv1.EndpointCLIID(endpoint))
	runtimeURL := firstNonEmpty(body.RuntimeURL, providerv1.EndpointBaseURL(endpoint))
	protocol := providerv1.EndpointProtocol(endpoint)
	capabilities, err := s.support.ResolveProviderCapabilities(ctx, &supportv1.ResolveProviderCapabilitiesRequest{
		Subject: &supportv1.ResolveProviderCapabilitiesRequest_Provider{Provider: &supportv1.ProviderCapabilitySubject{
			ProviderId:       firstNonEmpty(providerProjection.Provider.GetProviderId(), body.ProviderID),
			SurfaceId:        providerProjection.Provider.GetSurfaceId(),
			Endpoint:         endpoint,
			CredentialKind:   credential.GetCredentialKind(),
			ExecutionContext: supportv1.CapabilityExecutionContext_CAPABILITY_EXECUTION_CONTEXT_AGENT_RUN,
		}},
	})
	if err != nil {
		return agentrunauth.Projection{}, err
	}
	materializationKey := strings.TrimSpace(body.AuthMaterializationKey)
	egressPolicy, err := s.egress.GetEgressRuntimePolicy(ctx, &egressservicev1.GetEgressRuntimePolicyRequest{
		PolicyId:   capabilities.GetEgressPolicyId(),
		RuntimeUrl: runtimeURL,
	})
	if err != nil {
		return agentrunauth.Projection{}, err
	}
	authPolicy, err := s.auth.GetEgressAuthPolicy(ctx, &authv1.GetEgressAuthPolicyRequest{
		PolicyId:           capabilities.GetAuthPolicyId(),
		MaterializationKey: materializationKey,
		CredentialKind:     credential.GetCredentialKind(),
		Protocol:           protocol,
	})
	if err != nil {
		return agentrunauth.Projection{}, err
	}
	return agentrunauth.Projection{
		MaterializationKey:             firstNonEmpty(authPolicy.GetMaterializationKey(), materializationKey),
		RuntimeURL:                     runtimeURL,
		TargetHosts:                    append([]string(nil), egressPolicy.GetPolicy().GetTargetHosts()...),
		TargetPathPrefixes:             append([]string(nil), egressPolicy.GetPolicy().GetTargetPathPrefixes()...),
		RequestHeaderNames:             append([]string(nil), authPolicy.GetRequestHeaderNames()...),
		HeaderValuePrefix:              authPolicy.GetHeaderValuePrefix(),
		RequestHeaderReplacementRules:  authRulesToRuntimeRules(authPolicy.GetRequestReplacementRules()),
		ResponseHeaderReplacementRules: authRulesToRuntimeRules(authPolicy.GetResponseReplacementRules()),
		EgressPolicyID:                 capabilities.GetEgressPolicyId(),
		AuthPolicyID:                   capabilities.GetAuthPolicyId(),
		ObservabilityProfileIDs:        singleNonEmpty(capabilities.GetObservabilityPolicyId()),
		ProviderID:                     firstNonEmpty(providerProjection.Provider.GetProviderId(), body.ProviderID, cliID),
		VendorID:                       credential.GetVendorId(),
		SurfaceID:                      providerProjection.Provider.GetSurfaceId(),
		CLIID:                          cliID,
	}, nil
}

func singleNonEmpty(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return []string{value}
}

func authRulesToRuntimeRules(rules []*authv1.EgressSimpleReplacementRule) []*managementv1.AgentRunRuntimeHeaderReplacementRule {
	out := make([]*managementv1.AgentRunRuntimeHeaderReplacementRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		out = append(out, &managementv1.AgentRunRuntimeHeaderReplacementRule{
			Mode:              rule.GetMode(),
			HeaderName:        rule.GetHeaderName(),
			MaterialKey:       rule.GetMaterialKey(),
			HeaderValuePrefix: rule.GetHeaderValuePrefix(),
			Template:          rule.GetTemplate(),
		})
	}
	return out
}
