package agentexecution

import (
	"context"
	"strings"

	"code-code.internal/go-contract/domainerror"
	modelv1 "code-code.internal/go-contract/model/v1"
	modelservicev1 "code-code.internal/go-contract/platform/model/v1"
	"google.golang.org/protobuf/proto"
)

func (r *RemoteModelRegistry) ResolveRef(ctx context.Context, modelIDOrAlias string) (*modelv1.ModelRef, error) {
	modelIDOrAlias = strings.TrimSpace(modelIDOrAlias)
	if modelIDOrAlias == "" {
		return nil, domainerror.NewValidation("platformk8s/agentexecution: model id or alias is empty")
	}
	response, err := r.client.ResolveModelRef(ctx, &modelservicev1.ResolveModelRefRequest{
		ModelIdOrAlias: modelIDOrAlias,
	})
	if err != nil {
		return nil, err
	}
	ref := response.GetRef()
	if ref == nil {
		return nil, domainerror.NewNotFound("platformk8s/agentexecution: model id or alias %q not found", modelIDOrAlias)
	}
	if err := modelv1.ValidateRef(ref); err != nil {
		return nil, domainerror.NewValidation("platformk8s/agentexecution: resolved model ref is invalid: %v", err)
	}
	return proto.Clone(ref).(*modelv1.ModelRef), nil
}

func (r *RemoteModelRegistry) Resolve(ctx context.Context, ref *modelv1.ModelRef, override *modelv1.ModelOverride) (*modelv1.ResolvedModel, error) {
	if override != nil {
		return nil, domainerror.NewValidation("platformk8s/agentexecution: model override is not supported by remote registry")
	}
	if err := modelv1.ValidateRef(ref); err != nil {
		return nil, err
	}
	response, err := r.client.GetModelVersion(ctx, &modelservicev1.GetModelVersionRequest{
		Ref: proto.Clone(ref).(*modelv1.ModelRef),
	})
	if err != nil {
		return nil, err
	}
	item := response.GetItem()
	if item == nil || item.GetDefinition() == nil {
		return nil, domainerror.NewNotFound("platformk8s/agentexecution: model definition %q/%q not found", ref.GetVendorId(), ref.GetModelId())
	}
	effective := proto.Clone(item.GetDefinition()).(*modelv1.ModelVersion)
	if identityKey(effective.GetVendorId(), effective.GetModelId()) != identityKey(ref.GetVendorId(), ref.GetModelId()) {
		return nil, domainerror.NewValidation(
			"platformk8s/agentexecution: model service returned mismatched definition identity %q/%q for request %q/%q",
			effective.GetVendorId(),
			effective.GetModelId(),
			ref.GetVendorId(),
			ref.GetModelId(),
		)
	}
	resolved := &modelv1.ResolvedModel{
		ModelId:             effective.GetModelId(),
		EffectiveDefinition: effective,
	}
	if err := modelv1.ValidateResolvedModel(resolved); err != nil {
		return nil, domainerror.NewValidation("platformk8s/agentexecution: resolved model %q is invalid: %v", resolved.GetModelId(), err)
	}
	return resolved, nil
}

func identityKey(vendorID string, modelID string) string {
	return strings.TrimSpace(vendorID) + "\x00" + strings.TrimSpace(modelID)
}
