package agentruns

import (
	"context"
	"fmt"
	"strings"
	"time"

	"code-code.internal/go-contract/domainerror"
	agentrunv1 "code-code.internal/go-contract/platform/agent_run/v1"
	platformcontract "code-code.internal/platform-contract"
	platformv1alpha1 "code-code.internal/platform-k8s/api/v1alpha1"
	"code-code.internal/platform-k8s/internal/agentruntime/timeline"
	"code-code.internal/platform-k8s/internal/platform/resourcemeta"
	"code-code.internal/platform-k8s/internal/platform/resourceops"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type activeRunSlotManager interface {
	Claim(ctx context.Context, sessionID string, runID string) (*platformv1alpha1.AgentSessionResource, error)
	Release(ctx context.Context, sessionID string, runID string) (bool, error)
}

type ActiveRunSlotManager = activeRunSlotManager

type Service struct {
	client           ctrlclient.Client
	reader           ctrlclient.Reader
	namespace        string
	runtimeNamespace string
	slots            activeRunSlotManager
	sink             timeline.Sink
	now              func() time.Time
}

// ServiceOption customizes AgentRun management wiring.
type ServiceOption func(*Service)

// WithRuntimeNamespace sets the namespace used for AgentRun execution resources.
func WithRuntimeNamespace(namespace string) ServiceOption {
	return func(service *Service) {
		if strings.TrimSpace(namespace) != "" {
			service.runtimeNamespace = strings.TrimSpace(namespace)
		}
	}
}

func WithActiveRunSlots(slots activeRunSlotManager) ServiceOption {
	return func(service *Service) {
		if slots != nil {
			service.slots = slots
		}
	}
}

func NewService(client ctrlclient.Client, reader ctrlclient.Reader, namespace string, sink timeline.Sink, options ...ServiceOption) (*Service, error) {
	if client == nil {
		return nil, fmt.Errorf("platformk8s/agentruns: client is nil")
	}
	if reader == nil {
		return nil, fmt.Errorf("platformk8s/agentruns: reader is nil")
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, fmt.Errorf("platformk8s/agentruns: namespace is empty")
	}
	service := &Service{
		client:           client,
		reader:           reader,
		namespace:        strings.TrimSpace(namespace),
		runtimeNamespace: strings.TrimSpace(namespace),
		sink:             sink,
		now:              time.Now,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	if service.slots == nil {
		return nil, fmt.Errorf("platformk8s/agentruns: active run slot manager is nil")
	}
	return service, nil
}

func (s *Service) Get(ctx context.Context, runID string) (*agentrunv1.AgentRunState, error) {
	resource := &platformv1alpha1.AgentRunResource{}
	if err := s.reader.Get(ctx, types.NamespacedName{Namespace: s.namespace, Name: strings.TrimSpace(runID)}, resource); err != nil {
		return nil, err
	}
	return runStateFromResource(resource)
}

func (s *Service) Create(ctx context.Context, sessionID string, request *CreateRequest) (*agentrunv1.AgentRunState, error) {
	if request == nil {
		return nil, domainerror.NewValidation("platformk8s/agentruns: run request is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	runID, err := resourcemeta.EnsureResourceID(strings.TrimSpace(request.RunID), sessionID, "run")
	if err != nil {
		return nil, err
	}
	session, err := s.slots.Claim(ctx, sessionID, runID)
	if err != nil {
		return nil, err
	}
	releaseOnError := true
	defer func() {
		if !releaseOnError {
			return
		}
		if _, releaseErr := s.slots.Release(ctx, session.Spec.Session.GetSessionId(), runID); releaseErr != nil {
			s.recordTimelineEvent(ctx, session.Spec.Session.GetSessionId(), "FINISHED", "session_slot", "rollback", map[string]string{"run_id": runID, "error": strings.TrimSpace(releaseErr.Error())})
			return
		}
		s.recordTimelineEvent(ctx, session.Spec.Session.GetSessionId(), "FINISHED", "session_slot", "rollback", map[string]string{"run_id": runID})
	}()
	resource, err := s.runToResource(ctx, session, request, runID)
	if err != nil {
		return nil, err
	}
	if err := resourceops.CreateResource(ctx, s.client, resource, s.namespace, resource.Name); err != nil {
		if apierrors.IsAlreadyExists(err) || apierrors.IsConflict(err) {
			return nil, domainerror.NewAlreadyExists("platformk8s/agentruns: run %q already exists", resource.Name)
		}
		return nil, err
	}
	releaseOnError = false
	s.recordTimelineEvent(ctx, session.Spec.Session.GetSessionId(), "SUBMITTED", "run", "create", map[string]string{"run_id": runID})
	return runStateFromResource(resource)
}

func (s *Service) runToResource(ctx context.Context, session *platformv1alpha1.AgentSessionResource, request *CreateRequest, runID string) (*platformv1alpha1.AgentRunResource, error) {
	run, err := s.normalizeRun(ctx, session, request, runID)
	if err != nil {
		return nil, err
	}
	return &platformv1alpha1.AgentRunResource{
		TypeMeta: metav1.TypeMeta{APIVersion: platformv1alpha1.GroupVersion.String(), Kind: platformv1alpha1.KindAgentRunResource},
		ObjectMeta: metav1.ObjectMeta{
			Name:      run.GetRunId(),
			Namespace: s.namespace,
			Labels: map[string]string{
				sessionIDLabelKey: strings.TrimSpace(run.GetSessionId()),
			},
		},
		Spec: platformv1alpha1.AgentRunResourceSpec{Run: run},
	}, nil
}

func (s *Service) normalizeRun(ctx context.Context, session *platformv1alpha1.AgentSessionResource, request *CreateRequest, runID string) (*agentrunv1.AgentRunSpec, error) {
	if session == nil || session.Spec.Session == nil {
		return nil, domainerror.NewValidation("platformk8s/agentruns: session is invalid")
	}
	run, err := buildRunSpec(session.Spec.Session.GetSessionId(), request, runID)
	if err != nil {
		return nil, err
	}
	return run, nil
}

func (s *Service) SetTimelineSink(sink timeline.Sink) {
	if s == nil {
		return
	}
	s.sink = sink
}

func (s *Service) recordTimelineEvent(ctx context.Context, sessionID string, eventType string, subject string, action string, attributes map[string]string) {
	if s == nil || s.sink == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	if err := s.sink.RecordEvent(ctx, &platformcontract.TimelineEvent{
		ScopeRef: platformcontract.TimelineScopeRef{
			Scope:     platformcontract.TimelineScopeSession,
			SessionID: strings.TrimSpace(sessionID),
		},
		EventType:  eventType,
		Subject:    subject,
		Action:     action,
		OccurredAt: s.now().UTC(),
		Attributes: attributes,
	}); err != nil {
		return
	}
}
