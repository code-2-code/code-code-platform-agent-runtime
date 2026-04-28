package agentexecution

import (
	"context"
	"strings"
	"testing"

	cliruntimev1 "code-code.internal/go-contract/platform/cli_runtime/v1"
	providerservicev1 "code-code.internal/go-contract/platform/provider/v1"
	supportv1 "code-code.internal/go-contract/platform/support/v1"
	"google.golang.org/grpc"
)

func TestRemoteRuntimeCatalogResolvesLatestAvailableImage(t *testing.T) {
	catalog, err := NewRemoteRuntimeCatalog(
		runtimeCatalogProviderClient{},
		runtimeCatalogCLIClient{
			images: []*cliruntimev1.CLIRuntimeImage{{
				CliId:          "codex",
				ExecutionClass: "cli-standard",
				Image:          "code-code/codex:cli-1.2.3",
			}},
		},
		runtimeCatalogSupportClient{
			clis: []*supportv1.CLI{{
				CliId: "codex",
				ContainerImages: []*supportv1.CLIContainerImage{{
					ExecutionClass: "cli-standard",
					Image:          "code-code/codex:0.0.0",
					CpuRequest:     "100m",
					MemoryRequest:  "128Mi",
				}},
			}},
		},
	)
	if err != nil {
		t.Fatalf("NewRemoteRuntimeCatalog() error = %v", err)
	}

	image, err := catalog.ResolveContainerImage(context.Background(), "codex", "cli-standard")
	if err != nil {
		t.Fatalf("ResolveContainerImage() error = %v", err)
	}
	if got, want := image.Image, "code-code/codex:cli-1.2.3"; got != want {
		t.Fatalf("image = %q, want %q", got, want)
	}
	if got, want := image.CPURequest, "100m"; got != want {
		t.Fatalf("cpu request = %q, want %q", got, want)
	}
	if got, want := image.MemoryRequest, "128Mi"; got != want {
		t.Fatalf("memory request = %q, want %q", got, want)
	}
}

func TestRemoteRuntimeCatalogRejectsMissingLatestImage(t *testing.T) {
	catalog, err := NewRemoteRuntimeCatalog(
		runtimeCatalogProviderClient{},
		runtimeCatalogCLIClient{},
		runtimeCatalogSupportClient{
			clis: []*supportv1.CLI{{
				CliId: "codex",
				ContainerImages: []*supportv1.CLIContainerImage{{
					ExecutionClass: "cli-standard",
				}},
			}},
		},
	)
	if err != nil {
		t.Fatalf("NewRemoteRuntimeCatalog() error = %v", err)
	}

	_, err = catalog.ResolveContainerImage(context.Background(), "codex", "cli-standard")
	if err == nil || !strings.Contains(err.Error(), "no available runtime image") {
		t.Fatalf("ResolveContainerImage() error = %v, want no available runtime image", err)
	}
}

type runtimeCatalogProviderClient struct {
	providerservicev1.ProviderServiceClient
	definitions []*providerservicev1.CLIDefinitionView
}

func (c runtimeCatalogProviderClient) ListCLIDefinitions(context.Context, *providerservicev1.ListCLIDefinitionsRequest, ...grpc.CallOption) (*providerservicev1.ListCLIDefinitionsResponse, error) {
	return &providerservicev1.ListCLIDefinitionsResponse{Items: c.definitions}, nil
}

type runtimeCatalogCLIClient struct {
	cliruntimev1.CLIRuntimeServiceClient
	images []*cliruntimev1.CLIRuntimeImage
}

func (c runtimeCatalogCLIClient) GetLatestAvailableCLIRuntimeImages(context.Context, *cliruntimev1.GetLatestAvailableCLIRuntimeImagesRequest, ...grpc.CallOption) (*cliruntimev1.GetLatestAvailableCLIRuntimeImagesResponse, error) {
	return &cliruntimev1.GetLatestAvailableCLIRuntimeImagesResponse{Items: c.images}, nil
}

type runtimeCatalogSupportClient struct {
	supportv1.SupportServiceClient
	clis []*supportv1.CLI
}

func (c runtimeCatalogSupportClient) ListCLIs(context.Context, *supportv1.ListCLIsRequest, ...grpc.CallOption) (*supportv1.ListCLIsResponse, error) {
	return &supportv1.ListCLIsResponse{Items: c.clis}, nil
}

func (c runtimeCatalogSupportClient) GetCLI(_ context.Context, request *supportv1.GetCLIRequest, _ ...grpc.CallOption) (*supportv1.GetCLIResponse, error) {
	for _, cli := range c.clis {
		if cli.GetCliId() == request.GetCliId() {
			return &supportv1.GetCLIResponse{Item: cli}, nil
		}
	}
	return &supportv1.GetCLIResponse{}, nil
}
