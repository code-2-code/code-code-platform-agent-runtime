package otel

import (
	"context"
	"fmt"
	"sync"

	platformcontract "code-code.internal/platform-contract"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

const (
	stageDurationMetric = "timeline.stage.duration.seconds"
	eventCountMetric    = "timeline.events.total"
)

// MetricsProjector projects timeline records to OTel metrics.
type MetricsProjector struct {
	stageDuration otelmetric.Float64Histogram
	eventCount    otelmetric.Int64Counter
}

var (
	registerTimelineMetrics sync.Once
	registeredProjector     *MetricsProjector
	registerErr             error
)

// NewMetricsProjector creates or reuses the timeline metrics projector.
func NewMetricsProjector() (*MetricsProjector, error) {
	registerTimelineMetrics.Do(func() {
		registeredProjector, registerErr = newMetricsProjector(otel.Meter("platform-k8s/timeline"))
	})
	if registerErr != nil {
		return nil, registerErr
	}
	return registeredProjector, nil
}

func newMetricsProjector(meter otelmetric.Meter) (*MetricsProjector, error) {
	stageDuration, err := meter.Float64Histogram(
		stageDurationMetric,
		otelmetric.WithDescription("Duration of completed timeline stage intervals."),
		otelmetric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("timeline/otel: create stage duration metric: %w", err)
	}
	eventCount, err := meter.Int64Counter(
		eventCountMetric,
		otelmetric.WithDescription("Count of timeline events."),
		otelmetric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("timeline/otel: create event count metric: %w", err)
	}
	return &MetricsProjector{
		stageDuration: stageDuration,
		eventCount:    eventCount,
	}, nil
}

// ObserveStageInterval projects one stage interval to OTel metrics.
func (p *MetricsProjector) ObserveStageInterval(ctx context.Context, interval *platformcontract.StageInterval) error {
	if err := platformcontract.ValidateStageInterval(interval); err != nil {
		return err
	}
	if interval.EndedAt == nil {
		return nil
	}
	p.stageDuration.Record(ctx, interval.EndedAt.Sub(interval.StartedAt).Seconds(), otelmetric.WithAttributes(
		attribute.String("scope", string(interval.ScopeRef.Scope)),
		attribute.String("stage", interval.Stage),
		attribute.String("subject", interval.Subject),
		attribute.String("action", interval.Action),
		attribute.String("status", string(interval.Status)),
	))
	return nil
}

// ObserveEvent projects one timeline event to OTel metrics.
func (p *MetricsProjector) ObserveEvent(ctx context.Context, event *platformcontract.TimelineEvent) error {
	if err := platformcontract.ValidateTimelineEvent(event); err != nil {
		return err
	}
	p.eventCount.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.String("scope", string(event.ScopeRef.Scope)),
		attribute.String("event_type", event.EventType),
		attribute.String("subject", event.Subject),
		attribute.String("action", event.Action),
	))
	return nil
}

var _ platformcontract.TimelineMetricsProjector = (*MetricsProjector)(nil)
