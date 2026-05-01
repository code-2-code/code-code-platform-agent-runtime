package agentexecution

import (
	"context"
	"strings"

	agentcorev1 "code-code.internal/go-contract/agent/core/v1"
	modelv1 "code-code.internal/go-contract/model/v1"
	agentsessionv1 "code-code.internal/go-contract/platform/agent_session/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"google.golang.org/protobuf/proto"
)

type modelBinding struct {
	providerModelID string
	modelRef        *modelv1.ModelRef
}

func (r *Resolver) resolvePrimaryRuntimeCandidate(ctx context.Context, session *platformv1alpha1.AgentSessionResource, request *agentcorev1.RunRequest, instance *ProviderProjection) (*RuntimeCandidate, error) {
	modelRef, providerModelID := primaryModelSelector(session)
	requestModelID := requestModel(request)
	if requestModelID != "" {
		modelRef = nil
		providerModelID = requestModelID
	}
	if modelRef == nil && providerModelID == "" {
		return nil, validationf("session %q primary runtime model is empty", session.Spec.Session.GetSessionId())
	}
	return r.resolveRuntimeCandidate(ctx, session.Spec.Session.GetProviderId(), instance, modelRef, providerModelID)
}

func (r *Resolver) resolveFallbackRuntimeCandidate(ctx context.Context, session *platformv1alpha1.AgentSessionResource, fallback *agentsessionv1.AgentSessionRuntimeFallbackCandidate) (*RuntimeCandidate, error) {
	if fallback == nil {
		return nil, validation("runtime fallback candidate is nil")
	}
	instance, err := r.loadProvider(ctx, fallback.GetProviderId())
	if err != nil {
		return nil, err
	}
	switch selector := fallback.ModelSelector.(type) {
	case *agentsessionv1.AgentSessionRuntimeFallbackCandidate_ModelRef:
		return r.resolveRuntimeCandidate(ctx, session.Spec.Session.GetProviderId(), instance, normalizeModelRef(selector.ModelRef), "")
	case *agentsessionv1.AgentSessionRuntimeFallbackCandidate_ProviderModelId:
		return r.resolveRuntimeCandidate(ctx, session.Spec.Session.GetProviderId(), instance, nil, selector.ProviderModelId)
	default:
		return nil, validation("runtime fallback candidate model selector is required")
	}
}

func (r *Resolver) resolveRuntimeCandidate(ctx context.Context, providerID string, instance *ProviderProjection, modelRef *modelv1.ModelRef, providerModelID string) (*RuntimeCandidate, error) {
	resolvedProviderModel, err := r.resolveProviderModel(ctx, instance, modelRef, providerModelID)
	if err != nil {
		return nil, err
	}
	authRequirement, err := r.resolveAuthRequirement(ctx, providerID, instance, resolvedProviderModel)
	if err != nil {
		return nil, err
	}
	return &RuntimeCandidate{
		ResolvedProviderModel: resolvedProviderModel,
		AuthRequirement:       authRequirement,
	}, nil
}

func (r *Resolver) resolveProviderModel(ctx context.Context, instance *ProviderProjection, modelRef *modelv1.ModelRef, providerModelID string) (*providerv1.ResolvedProviderModel, error) {
	binding, err := r.selectModelBinding(ctx, instance, modelRef, providerModelID)
	if err != nil {
		return nil, err
	}
	resolvedModel, err := r.models.Resolve(ctx, binding.modelRef, nil)
	if err != nil {
		return nil, err
	}
	return &providerv1.ResolvedProviderModel{
		SurfaceId:       instance.Provider.GetSurfaceId(),
		ProviderModelId: binding.providerModelID,
		Endpoint:        cloneProviderEndpoint(instance.Endpoint),
		Model:           resolvedModel,
	}, nil
}

func (r *Resolver) selectModelBinding(ctx context.Context, instance *ProviderProjection, modelRef *modelv1.ModelRef, providerModelID string) (*modelBinding, error) {
	models := instance.Provider.GetModels()
	if normalizedRef := normalizeModelRef(modelRef); normalizedRef != nil {
		entry, err := findEntryByModelRef(models, normalizedRef)
		if err != nil {
			return nil, err
		}
		if entry == nil {
			return nil, validationf("provider surface %q does not expose model_ref %q", instance.Provider.GetSurfaceId(), normalizedRef.GetModelId())
		}
		return &modelBinding{providerModelID: entry.GetProviderModelId(), modelRef: normalizedRef}, nil
	}
	providerModelID = strings.TrimSpace(providerModelID)
	if providerModelID == "" {
		return nil, validationf("provider surface %q provider_model_id is empty", instance.Provider.GetSurfaceId())
	}
	if entry := findEntryByProviderModelID(models, providerModelID); entry != nil {
		ref := normalizeModelRef(entry.GetModelRef())
		if ref == nil {
			resolvedRef, err := r.models.ResolveRef(ctx, providerModelID)
			if err != nil {
				return nil, err
			}
			ref = normalizeModelRef(resolvedRef)
		}
		return &modelBinding{providerModelID: providerModelID, modelRef: ref}, nil
	}
	resolvedRef, err := r.models.ResolveRef(ctx, providerModelID)
	if err != nil {
		return nil, err
	}
	if entry, err := findEntryByModelRef(models, resolvedRef); err != nil {
		return nil, err
	} else if entry != nil {
		return &modelBinding{providerModelID: entry.GetProviderModelId(), modelRef: normalizeModelRef(resolvedRef)}, nil
	}
	return &modelBinding{providerModelID: providerModelID, modelRef: normalizeModelRef(resolvedRef)}, nil
}

func findEntryByProviderModelID(models []*providerv1.ProviderModel, providerModelID string) *providerv1.ProviderModel {
	for _, item := range models {
		if strings.TrimSpace(item.GetProviderModelId()) == strings.TrimSpace(providerModelID) {
			return item
		}
	}
	return nil
}

func findEntryByModelRef(models []*providerv1.ProviderModel, ref *modelv1.ModelRef) (*providerv1.ProviderModel, error) {
	ref = normalizeModelRef(ref)
	if ref == nil {
		return nil, nil
	}
	var match *providerv1.ProviderModel
	for _, item := range models {
		entryRef := normalizeModelRef(item.GetModelRef())
		if entryRef == nil || entryRef.GetModelId() != ref.GetModelId() {
			continue
		}
		if ref.GetVendorId() != "" && entryRef.GetVendorId() != ref.GetVendorId() {
			continue
		}
		if ref.GetVendorId() == "" && match != nil && normalizeModelRef(match.GetModelRef()).GetVendorId() != entryRef.GetVendorId() {
			return nil, validationf("model_ref %q is ambiguous across provider catalog entries", ref.GetModelId())
		}
		match = item
	}
	return match, nil
}

func normalizeModelRef(ref *modelv1.ModelRef) *modelv1.ModelRef {
	if ref == nil || strings.TrimSpace(ref.GetModelId()) == "" {
		return nil
	}
	return &modelv1.ModelRef{
		VendorId: strings.TrimSpace(ref.GetVendorId()),
		ModelId:  strings.TrimSpace(ref.GetModelId()),
	}
}

func primaryModelSelector(session *platformv1alpha1.AgentSessionResource) (*modelv1.ModelRef, string) {
	if session == nil || session.Spec.Session == nil || session.Spec.Session.GetRuntimeConfig() == nil {
		return nil, ""
	}
	modelSelector := session.Spec.Session.GetRuntimeConfig().GetPrimaryModelSelector()
	if modelSelector == nil {
		return nil, ""
	}
	switch selector := modelSelector.Selector.(type) {
	case *agentsessionv1.AgentSessionRuntimeModelSelector_ModelRef:
		return normalizeModelRef(selector.ModelRef), ""
	case *agentsessionv1.AgentSessionRuntimeModelSelector_ProviderModelId:
		return nil, strings.TrimSpace(selector.ProviderModelId)
	default:
		return nil, ""
	}
}

func surfaceID(instance *providerv1.Provider) string {
	if instance == nil {
		return ""
	}
	if strings.TrimSpace(instance.GetSurfaceId()) != "" {
		return strings.TrimSpace(instance.GetSurfaceId())
	}
	return ""
}

func cloneProviderEndpoint(endpoint *providerv1.ProviderEndpoint) *providerv1.ProviderEndpoint {
	if endpoint == nil {
		return nil
	}
	return proto.Clone(endpoint).(*providerv1.ProviderEndpoint)
}

func requestModel(request *agentcorev1.RunRequest) string {
	if request == nil || request.GetInput() == nil {
		return ""
	}
	parameters := request.GetInput().GetParameters()
	if parameters == nil || parameters.GetFields() == nil {
		return ""
	}
	value := parameters.GetFields()["model"]
	if value == nil {
		return ""
	}
	return strings.TrimSpace(value.GetStringValue())
}
