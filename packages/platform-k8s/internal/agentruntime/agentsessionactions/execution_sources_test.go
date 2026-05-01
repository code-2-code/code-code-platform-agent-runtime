package agentsessionactions

import (
	"context"
	"strings"
	"testing"

	apiprotocolv1 "code-code.internal/go-contract/api_protocol/v1"
	"code-code.internal/go-contract/domainerror"
	modelv1 "code-code.internal/go-contract/model/v1"
	supportv1 "code-code.internal/go-contract/platform/support/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
	"code-code.internal/platform-k8s/internal/agentruntime/agentexecution"
)

func newTestExecutionResolver(t *testing.T) *agentexecution.Resolver {
	t.Helper()
	resolver, err := agentexecution.NewResolver(testRuntimeCatalog{}, testModelRegistry{})
	if err != nil {
		t.Fatalf("NewResolver() error = %v", err)
	}
	return resolver
}

type testRuntimeCatalog struct{}

func (testRuntimeCatalog) ResolveContainerImage(_ context.Context, providerID, executionClass string) (*agentexecution.ContainerImage, error) {
	if strings.TrimSpace(providerID) != "codex" || strings.TrimSpace(executionClass) != "default" {
		return nil, domainerror.NewValidation("test execution class %q is not declared by cli definition %q", executionClass, providerID)
	}
	return &agentexecution.ContainerImage{
		Image:         "ghcr.io/openai/codex:latest",
		CPURequest:    "1000m",
		MemoryRequest: "2Gi",
	}, nil
}

func (testRuntimeCatalog) GetProvider(_ context.Context, surfaceID string) (*agentexecution.ProviderProjection, error) {
	surfaceID = strings.TrimSpace(surfaceID)
	for _, resource := range []*providerv1.Provider{
		testProvider(),
		testFallbackProvider(),
	} {
		surface, err := testProjectedProvider(resource, surfaceID)
		if err != nil {
			continue
		}
		return &agentexecution.ProviderProjection{
			Provider: surface,
			Endpoint: testProviderEndpointForSurface(surface.GetSurfaceId()),
		}, nil
	}
	return nil, domainerror.NewNotFound("test provider surface %q not found", surfaceID)
}

func (testRuntimeCatalog) GetCLI(_ context.Context, cliID string) (*supportv1.CLI, error) {
	if strings.TrimSpace(cliID) != "codex" {
		return nil, domainerror.NewNotFound("test cli support %q not found", cliID)
	}
	return &supportv1.CLI{
		CliId: "codex",
		ApiKeyProtocols: []*supportv1.APIKeyProtocolSupport{{
			Protocol: apiprotocolv1.Protocol_PROTOCOL_OPENAI_RESPONSES,
		}},
	}, nil
}

func testProjectedProvider(provider *providerv1.Provider, surfaceID string) (*providerv1.Provider, error) {
	if strings.TrimSpace(provider.GetSurfaceId()) == surfaceID {
		return provider, nil
	}
	return nil, domainerror.NewNotFound("surface %q not found", surfaceID)
}

type testModelRegistry struct{}

func (testModelRegistry) ResolveRef(_ context.Context, modelIDOrAlias string) (*modelv1.ModelRef, error) {
	modelIDOrAlias = strings.TrimSpace(modelIDOrAlias)
	switch modelIDOrAlias {
	case "gpt-5", "gpt-4.1-mini":
		return &modelv1.ModelRef{VendorId: "openai", ModelId: modelIDOrAlias}, nil
	default:
		return nil, domainerror.NewNotFound("test model %q not found", modelIDOrAlias)
	}
}

func (testModelRegistry) Resolve(_ context.Context, ref *modelv1.ModelRef, _ *modelv1.ModelOverride) (*modelv1.ResolvedModel, error) {
	if ref == nil || strings.TrimSpace(ref.GetModelId()) == "" {
		return nil, domainerror.NewValidation("test model ref is empty")
	}
	return &modelv1.ResolvedModel{
		ModelId: strings.TrimSpace(ref.GetModelId()),
		EffectiveDefinition: &modelv1.ModelVersion{
			VendorId:         testFirstNonEmpty(strings.TrimSpace(ref.GetVendorId()), "openai"),
			ModelId:          strings.TrimSpace(ref.GetModelId()),
			DisplayName:      strings.TrimSpace(ref.GetModelId()),
			PrimaryShape:     modelv1.ModelShape_MODEL_SHAPE_CHAT_COMPLETIONS,
			SupportedShapes:  []modelv1.ModelShape{modelv1.ModelShape_MODEL_SHAPE_CHAT_COMPLETIONS},
			InputModalities:  []modelv1.Modality{modelv1.Modality_MODALITY_TEXT},
			OutputModalities: []modelv1.Modality{modelv1.Modality_MODALITY_TEXT},
		},
	}, nil
}

func testFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
