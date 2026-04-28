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
	surfaceProjection, err := s.runtimeCatalog.GetProviderSurfaceBinding(ctx, body.ProviderSurfaceBindingID)
	if err != nil {
		return agentrunauth.Projection{}, err
	}
	if surfaceProjection == nil || surfaceProjection.Surface == nil || surfaceProjection.Surface.GetRuntime() == nil || providerv1.RuntimeKind(surfaceProjection.Surface.GetRuntime()) == providerv1.ProviderSurfaceKind_PROVIDER_SURFACE_KIND_UNSPECIFIED {
		return agentrunauth.Projection{}, fmt.Errorf("platformk8s/sessionapi: provider surface binding %q runtime is invalid", body.ProviderSurfaceBindingID)
	}
	credentialID := strings.TrimSpace(surfaceProjection.Surface.GetProviderCredentialRef().GetProviderCredentialId())
	if credentialID == "" {
		return agentrunauth.Projection{}, fmt.Errorf("platformk8s/sessionapi: provider surface binding %q credential is empty", body.ProviderSurfaceBindingID)
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
	runtime := surfaceProjection.Surface.GetRuntime()
	cliID := firstNonEmpty(body.Job.CLIID, body.ProviderID, providerv1.RuntimeCLIID(runtime))
	runtimeURL := firstNonEmpty(body.RuntimeURL, providerv1.RuntimeBaseURL(runtime))
	protocol := providerv1.RuntimeProtocol(runtime)
	capabilities, err := s.support.ResolveProviderCapabilities(ctx, &supportv1.ResolveProviderCapabilitiesRequest{
		Subject: &supportv1.ResolveProviderCapabilitiesRequest_Runtime{Runtime: &supportv1.RuntimeCapabilitySubject{
			ProviderId:             firstNonEmpty(body.ProviderID, cliID),
			SurfaceId:              surfaceProjection.Surface.GetSurfaceId(),
			Protocol:               protocol,
			CredentialKind:         credential.GetCredentialKind(),
			RuntimeUrl:             runtimeURL,
			AuthMaterializationKey: body.AuthMaterializationKey,
			ExecutionContext:       supportv1.CapabilityExecutionContext_CAPABILITY_EXECUTION_CONTEXT_AGENT_RUN,
		}},
	})
	if err != nil {
		return agentrunauth.Projection{}, err
	}
	materializationKey := strings.TrimSpace(firstNonEmpty(capabilities.GetAuthMaterializationKey(), body.AuthMaterializationKey))
	if expected := strings.TrimSpace(body.AuthMaterializationKey); expected != "" && materializationKey != "" && expected != materializationKey {
		return agentrunauth.Projection{}, fmt.Errorf("platformk8s/sessionapi: auth materialization key %q does not match %q", expected, materializationKey)
	}
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
		ObservabilityProfileIDs:        observabilityProfileIDs(capabilities.GetObservability()),
		ProviderID:                     firstNonEmpty(body.ProviderID, cliID),
		VendorID:                       credential.GetVendorId(),
		ProviderSurfaceBindingID:       surfaceProjection.Surface.GetSurfaceId(),
		CLIID:                          cliID,
	}, nil
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
