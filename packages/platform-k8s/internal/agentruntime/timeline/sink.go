package timeline

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	platformcontract "code-code.internal/platform-contract"
	timelinenats "code-code.internal/platform-k8s/internal/agentruntime/timeline/nats"
	timelineotel "code-code.internal/platform-k8s/internal/agentruntime/timeline/otel"
)

// Sink records or publishes timeline records for one execution path.
type Sink interface {
	RecordStageInterval(context.Context, *platformcontract.StageInterval) error
	RecordEvent(context.Context, *platformcontract.TimelineEvent) error
	Close()
}

type publisher interface {
	PublishStageInterval(context.Context, *platformcontract.StageInterval) error
	PublishEvent(context.Context, *platformcontract.TimelineEvent) error
}

type metricsProjector interface {
	ObserveStageInterval(context.Context, *platformcontract.StageInterval) error
	ObserveEvent(context.Context, *platformcontract.TimelineEvent) error
}

type closer interface {
	Close()
}

type sink struct {
	publisher publisher
	projector metricsProjector
	logger    *slog.Logger
}

// SinkConfig defines timeline publish/projection settings.
type SinkConfig struct {
	NATSURL           string
	NATSSubjectPrefix string
	ApplicationName   string
	Logger            *slog.Logger
}

// NewSink creates one timeline sink backed by NATS publish and OTel metric projection.
func NewSink(config SinkConfig) (Sink, error) {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if strings.TrimSpace(config.NATSURL) == "" {
		return nil, fmt.Errorf("timeline: nats url is empty")
	}
	publisher, err := timelinenats.NewPublisher(timelinenats.PublisherConfig{
		URL:           config.NATSURL,
		SubjectPrefix: defaultSubjectPrefix(config.NATSSubjectPrefix),
		ClientName:    config.ApplicationName,
	})
	if err != nil {
		return nil, err
	}
	projector, err := timelineotel.NewMetricsProjector()
	if err != nil {
		publisher.Close()
		return nil, err
	}
	return newSink(publisher, projector, config.Logger)
}

func newSink(publisher publisher, projector metricsProjector, logger *slog.Logger) (Sink, error) {
	if publisher == nil {
		return nil, fmt.Errorf("timeline: publisher is nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &sink{
		publisher: publisher,
		projector: projector,
		logger:    logger,
	}, nil
}

func (s *sink) RecordStageInterval(ctx context.Context, interval *platformcontract.StageInterval) error {
	if s == nil || s.publisher == nil {
		return fmt.Errorf("timeline: sink is nil")
	}
	if err := platformcontract.ValidateStageInterval(interval); err != nil {
		return err
	}
	if s.projector != nil {
		if err := s.projector.ObserveStageInterval(ctx, interval); err != nil {
			s.logger.Error("timeline stage interval metrics projection failed", "error", err, "stage", interval.Stage, "subject", interval.Subject, "action", interval.Action)
		}
	}
	return s.publisher.PublishStageInterval(ctx, interval)
}

func (s *sink) RecordEvent(ctx context.Context, event *platformcontract.TimelineEvent) error {
	if s == nil || s.publisher == nil {
		return fmt.Errorf("timeline: sink is nil")
	}
	if err := platformcontract.ValidateTimelineEvent(event); err != nil {
		return err
	}
	if s.projector != nil {
		if err := s.projector.ObserveEvent(ctx, event); err != nil {
			s.logger.Error("timeline event metrics projection failed", "error", err, "eventType", event.EventType, "subject", event.Subject, "action", event.Action)
		}
	}
	return s.publisher.PublishEvent(ctx, event)
}

func (s *sink) Close() {
	if s == nil {
		return
	}
	if publisher, ok := s.publisher.(closer); ok {
		publisher.Close()
	}
}

func defaultSubjectPrefix(value string) string {
	if strings.TrimSpace(value) == "" {
		return "platform.timeline"
	}
	return value
}
