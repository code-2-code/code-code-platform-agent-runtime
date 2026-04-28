package agentexecution

import (
	"context"
	"testing"

	modelv1 "code-code.internal/go-contract/model/v1"
	modelservicev1 "code-code.internal/go-contract/platform/model/v1"
	"google.golang.org/grpc"
)

func TestRemoteModelRegistryResolveRefDelegatesToResolveModelRef(t *testing.T) {
	t.Parallel()

	client := &modelServiceClientStub{
		resolveRefResponse: &modelservicev1.ResolveModelRefResponse{
			Ref: &modelv1.ModelRef{VendorId: "openai", ModelId: "gpt-5"},
		},
	}
	registry := &RemoteModelRegistry{client: client}

	ref, err := registry.ResolveRef(context.Background(), "gpt-5")
	if err != nil {
		t.Fatalf("ResolveRef() error = %v", err)
	}
	if got, want := ref.GetVendorId(), "openai"; got != want {
		t.Fatalf("vendor_id = %q, want %q", got, want)
	}
	if got, want := ref.GetModelId(), "gpt-5"; got != want {
		t.Fatalf("model_id = %q, want %q", got, want)
	}
	if got, want := client.resolveRefCalls, 1; got != want {
		t.Fatalf("ResolveModelRef calls = %d, want %d", got, want)
	}
	if got, want := client.listCalls, 0; got != want {
		t.Fatalf("ListModelDefinitions calls = %d, want %d", got, want)
	}
}

func TestRemoteModelRegistryResolveUsesGetModelDefinition(t *testing.T) {
	t.Parallel()

	client := &modelServiceClientStub{
		getModelDefinitionResponse: &modelservicev1.GetModelVersionResponse{
			Item: &modelservicev1.ModelRegistryEntry{
				Definition: &modelv1.ModelVersion{
					VendorId:        "openai",
					ModelId:         "gpt-5",
					PrimaryShape:    modelv1.ModelShape_MODEL_SHAPE_RESPONSES,
					SupportedShapes: []modelv1.ModelShape{modelv1.ModelShape_MODEL_SHAPE_RESPONSES},
				},
			},
		},
	}
	registry := &RemoteModelRegistry{client: client}

	resolved, err := registry.Resolve(context.Background(), &modelv1.ModelRef{
		VendorId: "openai",
		ModelId:  "gpt-5",
	}, nil)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got, want := resolved.GetModelId(), "gpt-5"; got != want {
		t.Fatalf("resolved model_id = %q, want %q", got, want)
	}
	if got, want := client.getModelDefinitionCalls, 1; got != want {
		t.Fatalf("GetModelDefinition calls = %d, want %d", got, want)
	}
	if got, want := client.listCalls, 0; got != want {
		t.Fatalf("ListModelDefinitions calls = %d, want %d", got, want)
	}
}

func TestRemoteModelRegistryResolveRejectsMismatchedDefinitionIdentity(t *testing.T) {
	t.Parallel()

	client := &modelServiceClientStub{
		getModelDefinitionResponse: &modelservicev1.GetModelVersionResponse{
			Item: &modelservicev1.ModelRegistryEntry{
				Definition: &modelv1.ModelVersion{
					VendorId:        "anthropic",
					ModelId:         "claude-sonnet-4",
					PrimaryShape:    modelv1.ModelShape_MODEL_SHAPE_RESPONSES,
					SupportedShapes: []modelv1.ModelShape{modelv1.ModelShape_MODEL_SHAPE_RESPONSES},
				},
			},
		},
	}
	registry := &RemoteModelRegistry{client: client}

	_, err := registry.Resolve(context.Background(), &modelv1.ModelRef{
		VendorId: "openai",
		ModelId:  "gpt-5",
	}, nil)
	if err == nil {
		t.Fatal("Resolve() error = nil, want mismatched identity error")
	}
}

type modelServiceClientStub struct {
	modelservicev1.ModelServiceClient

	listCalls                  int
	resolveRefCalls            int
	getModelDefinitionCalls    int
	resolveRefResponse         *modelservicev1.ResolveModelRefResponse
	getModelDefinitionResponse *modelservicev1.GetModelVersionResponse
}

func (c *modelServiceClientStub) ListModels(context.Context, *modelservicev1.ListModelsRequest, ...grpc.CallOption) (*modelservicev1.ListModelsResponse, error) {
	c.listCalls++
	return &modelservicev1.ListModelsResponse{}, nil
}

func (c *modelServiceClientStub) ResolveModelRef(context.Context, *modelservicev1.ResolveModelRefRequest, ...grpc.CallOption) (*modelservicev1.ResolveModelRefResponse, error) {
	c.resolveRefCalls++
	if c.resolveRefResponse != nil {
		return c.resolveRefResponse, nil
	}
	return &modelservicev1.ResolveModelRefResponse{}, nil
}

func (c *modelServiceClientStub) GetModelVersion(context.Context, *modelservicev1.GetModelVersionRequest, ...grpc.CallOption) (*modelservicev1.GetModelVersionResponse, error) {
	c.getModelDefinitionCalls++
	if c.getModelDefinitionResponse != nil {
		return c.getModelDefinitionResponse, nil
	}
	return &modelservicev1.GetModelVersionResponse{}, nil
}

func (c *modelServiceClientStub) SyncModelDefinitions(context.Context, *modelservicev1.SyncModelDefinitionsRequest, ...grpc.CallOption) (*modelservicev1.SyncModelDefinitionsResponse, error) {
	return &modelservicev1.SyncModelDefinitionsResponse{}, nil
}
