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
	source          providerv1.CatalogSource
}

func (r *Resolver) resolvePrimaryRuntimeCandidate(ctx context.Context, session *platformv1alpha1.AgentSessionResource, request *agentcorev1.RunRequest, instance *SurfaceBindingProjection) (*RuntimeCandidate, error) {
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
	instance, err := r.loadProviderSurfaceBindingByID(ctx, fallback.GetProviderRuntimeRef().GetSurfaceId())
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

func (r *Resolver) resolveRuntimeCandidate(ctx context.Context, providerID string, instance *SurfaceBindingProjection, modelRef *modelv1.ModelRef, providerModelID string) (*RuntimeCandidate, error) {
	resolvedProviderModel, err := r.resolveProviderModel(ctx, instance, modelRef, providerModelID)
	if err != nil {
		return nil, err
	}
	authRequirement, err := r.resolveAuthRequirement(ctx, providerID, instance, resolvedProviderModel.GetBaseUrl())
	if err != nil {
		return nil, err
	}
	return &RuntimeCandidate{
		ResolvedProviderModel: resolvedProviderModel,
		AuthRequirement:       authRequirement,
	}, nil
}

func (r *Resolver) resolveProviderModel(ctx context.Context, instance *SurfaceBindingProjection, modelRef *modelv1.ModelRef, providerModelID string) (*providerv1.ResolvedProviderModel, error) {
	binding, err := r.selectModelBinding(ctx, instance, modelRef, providerModelID)
	if err != nil {
		return nil, err
	}
	resolvedModel, err := r.models.Resolve(ctx, binding.modelRef, nil)
	if err != nil {
		return nil, err
	}
	runtime := proto.Clone(instance.Surface.GetRuntime()).(*providerv1.ProviderSurfaceRuntime)
	return &providerv1.ResolvedProviderModel{
		SurfaceId:       surfaceID(instance.Surface),
		ProviderModelId: binding.providerModelID,
		Protocol:        providerv1.RuntimeProtocol(runtime),
		BaseUrl:         providerv1.RuntimeBaseURL(runtime),
		Model:           resolvedModel,
		Source:          binding.source,
		Surface:         &providerv1.ResolvedProviderSurface{Surface: runtime},
	}, nil
}

func (r *Resolver) selectModelBinding(ctx context.Context, instance *SurfaceBindingProjection, modelRef *modelv1.ModelRef, providerModelID string) (*modelBinding, error) {
	catalog := instance.Surface.GetRuntime().GetCatalog()
	if normalizedRef := normalizeModelRef(modelRef); normalizedRef != nil {
		entry, err := findEntryByModelRef(catalog, normalizedRef)
		if err != nil {
			return nil, err
		}
		if entry == nil {
			return nil, validationf("provider surface binding %q does not expose model_ref %q", instance.Surface.GetSurfaceId(), normalizedRef.GetModelId())
		}
		return &modelBinding{providerModelID: entry.GetProviderModelId(), modelRef: normalizedRef, source: sourceFromCatalog(catalog)}, nil
	}
	providerModelID = strings.TrimSpace(providerModelID)
	if providerModelID == "" {
		return nil, validationf("provider surface binding %q provider_model_id is empty", instance.Surface.GetSurfaceId())
	}
	if entry := findEntryByProviderModelID(catalog, providerModelID); entry != nil {
		ref := normalizeModelRef(entry.GetModelRef())
		if ref == nil {
			resolvedRef, err := r.models.ResolveRef(ctx, providerModelID)
			if err != nil {
				return nil, err
			}
			ref = normalizeModelRef(resolvedRef)
		}
		return &modelBinding{providerModelID: providerModelID, modelRef: ref, source: sourceFromCatalog(catalog)}, nil
	}
	resolvedRef, err := r.models.ResolveRef(ctx, providerModelID)
	if err != nil {
		return nil, err
	}
	if entry, err := findEntryByModelRef(catalog, resolvedRef); err != nil {
		return nil, err
	} else if entry != nil {
		return &modelBinding{providerModelID: entry.GetProviderModelId(), modelRef: normalizeModelRef(resolvedRef), source: sourceFromCatalog(catalog)}, nil
	}
	return &modelBinding{providerModelID: providerModelID, modelRef: normalizeModelRef(resolvedRef), source: sourceFromCatalog(catalog)}, nil
}

func findEntryByProviderModelID(catalog *providerv1.ProviderModelCatalog, providerModelID string) *providerv1.ProviderModelCatalogEntry {
	for _, item := range catalog.GetModels() {
		if strings.TrimSpace(item.GetProviderModelId()) == strings.TrimSpace(providerModelID) {
			return item
		}
	}
	return nil
}

func findEntryByModelRef(catalog *providerv1.ProviderModelCatalog, ref *modelv1.ModelRef) (*providerv1.ProviderModelCatalogEntry, error) {
	ref = normalizeModelRef(ref)
	if ref == nil {
		return nil, nil
	}
	var match *providerv1.ProviderModelCatalogEntry
	for _, item := range catalog.GetModels() {
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

func sourceFromCatalog(catalog *providerv1.ProviderModelCatalog) providerv1.CatalogSource {
	if catalog != nil && catalog.GetSource() != providerv1.CatalogSource_CATALOG_SOURCE_UNSPECIFIED {
		return catalog.GetSource()
	}
	return providerv1.CatalogSource_CATALOG_SOURCE_FALLBACK_CONFIG
}

func surfaceID(instance *providerv1.ProviderSurfaceBinding) string {
	if instance == nil {
		return ""
	}
	if strings.TrimSpace(instance.GetSurfaceId()) != "" {
		return strings.TrimSpace(instance.GetSurfaceId())
	}
	return ""
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
