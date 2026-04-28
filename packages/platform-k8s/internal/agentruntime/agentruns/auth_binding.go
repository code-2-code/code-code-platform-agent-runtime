package agentruns

import (
	apiprotocolv1 "code-code.internal/go-contract/api_protocol/v1"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
)

func providerRunBinding(auth *agentrunv1.AgentRunAuthRequirement) providerBindingView {
	if auth == nil {
		return providerBindingView{}
	}
	return providerBindingView{binding: auth.GetProviderRunBinding()}
}

type providerBindingView struct {
	binding *providerv1.ProviderRunBinding
}

func (v providerBindingView) ProviderID() string {
	if v.binding == nil {
		return ""
	}
	return v.binding.GetProviderId()
}

func (v providerBindingView) RuntimeCLIID() string {
	if v.binding == nil {
		return ""
	}
	return v.binding.GetRuntimeCliId()
}

func (v providerBindingView) CredentialGrantID() string {
	if v.binding == nil || v.binding.GetCredentialGrantRef() == nil {
		return ""
	}
	return v.binding.GetCredentialGrantRef().GetGrantId()
}

func (v providerBindingView) HasTarget() bool {
	return v.binding != nil && (v.binding.GetCli() != nil || v.binding.GetApi() != nil)
}

func (v providerBindingView) IsCLI() bool {
	return v.binding != nil && v.binding.GetCli() != nil
}

func (v providerBindingView) Protocol() apiprotocolv1.Protocol {
	if v.binding == nil || v.binding.GetApi() == nil {
		return apiprotocolv1.Protocol_PROTOCOL_UNSPECIFIED
	}
	return v.binding.GetApi().GetProtocol()
}

func (v providerBindingView) RuntimeURL() string {
	if v.binding == nil {
		return ""
	}
	return v.binding.GetRuntimeUrl()
}

func (v providerBindingView) MaterializationKey() string {
	if v.binding == nil {
		return ""
	}
	return v.binding.GetMaterializationKey()
}
