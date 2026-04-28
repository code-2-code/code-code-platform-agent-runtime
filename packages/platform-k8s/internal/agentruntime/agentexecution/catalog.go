package agentexecution

import (
	"context"
	"fmt"
	"strings"

	"code-code.internal/go-contract/domainerror"
	modelv1 "code-code.internal/go-contract/model/v1"
	cliruntimev1 "code-code.internal/go-contract/platform/cli_runtime/v1"
	modelservicev1 "code-code.internal/go-contract/platform/model/v1"
	providerservicev1 "code-code.internal/go-contract/platform/provider/v1"
	supportv1 "code-code.internal/go-contract/platform/support/v1"
	providerv1 "code-code.internal/go-contract/provider/v1"
	"google.golang.org/protobuf/proto"
)

type ContainerImage struct {
	Image         string
	CPURequest    string
	MemoryRequest string
}

type SurfaceBindingProjection struct {
	Surface *providerv1.ProviderSurfaceBinding
}

type RuntimeCatalog interface {
	ResolveContainerImage(ctx context.Context, providerID, executionClass string) (*ContainerImage, error)
	GetProviderSurfaceBinding(ctx context.Context, surfaceID string) (*SurfaceBindingProjection, error)
	GetCLI(ctx context.Context, cliID string) (*supportv1.CLI, error)
}

type ModelRegistry interface {
	ResolveRef(ctx context.Context, modelIDOrAlias string) (*modelv1.ModelRef, error)
	Resolve(ctx context.Context, ref *modelv1.ModelRef, override *modelv1.ModelOverride) (*modelv1.ResolvedModel, error)
}

type RemoteRuntimeCatalog struct {
	providers  providerservicev1.ProviderServiceClient
	cliRuntime cliruntimev1.CLIRuntimeServiceClient
	support    supportv1.SupportServiceClient
}

func NewRemoteRuntimeCatalog(providers providerservicev1.ProviderServiceClient, cliRuntime cliruntimev1.CLIRuntimeServiceClient, support supportv1.SupportServiceClient) (*RemoteRuntimeCatalog, error) {
	if providers == nil {
		return nil, fmt.Errorf("platformk8s/agentexecution: provider service client is nil")
	}
	if cliRuntime == nil {
		return nil, fmt.Errorf("platformk8s/agentexecution: cli runtime service client is nil")
	}
	if support == nil {
		return nil, fmt.Errorf("platformk8s/agentexecution: support service client is nil")
	}
	return &RemoteRuntimeCatalog{providers: providers, cliRuntime: cliRuntime, support: support}, nil
}

func (c *RemoteRuntimeCatalog) ResolveContainerImage(ctx context.Context, providerID, executionClass string) (*ContainerImage, error) {
	declared, err := c.declaredContainerImage(ctx, providerID, executionClass)
	if err != nil {
		return nil, err
	}
	image, err := c.latestAvailableRuntimeImage(ctx, providerID, executionClass)
	if err != nil {
		return nil, err
	}
	return &ContainerImage{
		Image:         image,
		CPURequest:    declared.GetCpuRequest(),
		MemoryRequest: declared.GetMemoryRequest(),
	}, nil
}

func (c *RemoteRuntimeCatalog) declaredContainerImage(ctx context.Context, providerID, executionClass string) (*supportv1.CLIContainerImage, error) {
	response, err := c.support.ListCLIs(ctx, &supportv1.ListCLIsRequest{})
	if err != nil {
		return nil, err
	}
	providerID = strings.TrimSpace(providerID)
	executionClass = strings.TrimSpace(executionClass)
	for _, cli := range response.GetItems() {
		if strings.TrimSpace(cli.GetCliId()) != providerID {
			continue
		}
		for _, image := range cli.GetContainerImages() {
			if strings.TrimSpace(image.GetExecutionClass()) == executionClass {
				return image, nil
			}
		}
		return nil, domainerror.NewValidation("platformk8s/agentexecution: execution class %q is not declared by cli definition %q", executionClass, providerID)
	}
	return nil, domainerror.NewNotFound("platformk8s/agentexecution: cli definition %q not found", providerID)
}

func (c *RemoteRuntimeCatalog) latestAvailableRuntimeImage(ctx context.Context, providerID, executionClass string) (string, error) {
	providerID = strings.TrimSpace(providerID)
	executionClass = strings.TrimSpace(executionClass)
	response, err := c.cliRuntime.GetLatestAvailableCLIRuntimeImages(ctx, &cliruntimev1.GetLatestAvailableCLIRuntimeImagesRequest{
		CliId: providerID,
	})
	if err != nil {
		return "", err
	}
	for _, item := range response.GetItems() {
		if strings.TrimSpace(item.GetCliId()) != providerID {
			continue
		}
		if strings.TrimSpace(item.GetExecutionClass()) != executionClass {
			continue
		}
		if image := strings.TrimSpace(item.GetImage()); image != "" {
			return image, nil
		}
	}
	return "", domainerror.NewNotFound("platformk8s/agentexecution: no available runtime image for cli %q execution class %q", providerID, executionClass)
}

func (c *RemoteRuntimeCatalog) GetProviderSurfaceBinding(ctx context.Context, surfaceID string) (*SurfaceBindingProjection, error) {
	response, err := c.providers.ListProviderSurfaceBindings(ctx, &providerservicev1.ListProviderSurfaceBindingsRequest{})
	if err != nil {
		return nil, err
	}
	surfaceID = strings.TrimSpace(surfaceID)
	for _, item := range response.GetItems() {
		if strings.TrimSpace(item.GetSurfaceId()) != surfaceID {
			continue
		}
		surface := &providerv1.ProviderSurfaceBinding{
			SurfaceId:             item.GetSurfaceId(),
			ProviderCredentialRef: &providerv1.ProviderCredentialRef{ProviderCredentialId: item.GetProviderCredentialId()},
		}
		if item.GetRuntime() != nil {
			surface.Runtime = proto.Clone(item.GetRuntime()).(*providerv1.ProviderSurfaceRuntime)
		}
		return &SurfaceBindingProjection{Surface: surface}, nil
	}
	return nil, domainerror.NewNotFound("platformk8s/agentexecution: provider surface binding %q not found", surfaceID)
}

func (c *RemoteRuntimeCatalog) GetCLI(ctx context.Context, cliID string) (*supportv1.CLI, error) {
	cliID = strings.TrimSpace(cliID)
	response, err := c.support.GetCLI(ctx, &supportv1.GetCLIRequest{CliId: cliID})
	if err == nil && response.GetItem() != nil {
		return proto.Clone(response.GetItem()).(*supportv1.CLI), nil
	}
	return nil, domainerror.NewNotFound("platformk8s/agentexecution: cli %q not found", cliID)
}

type RemoteModelRegistry struct {
	client modelservicev1.ModelServiceClient
}

func NewRemoteModelRegistry(client modelservicev1.ModelServiceClient) (*RemoteModelRegistry, error) {
	if client == nil {
		return nil, fmt.Errorf("platformk8s/agentexecution: model service client is nil")
	}
	return &RemoteModelRegistry{client: client}, nil
}
