package otel

import (
	"context"
	"testing"
	"time"

	platformcontract "code-code.internal/platform-contract"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestNewMetricsProjectorReusesRegisteredProjector(t *testing.T) {
	first, err := NewMetricsProjector()
	if err != nil {
		t.Fatalf("NewMetricsProjector() error = %v", err)
	}
	second, err := NewMetricsProjector()
	if err != nil {
		t.Fatalf("NewMetricsProjector() second error = %v", err)
	}
	if first != second {
		t.Fatal("NewMetricsProjector() expected shared projector instance")
	}
}

func TestMetricsProjectorObservesTerminalInterval(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	projector, err := newMetricsProjector(provider.Meter("timeline-test"))
	if err != nil {
		t.Fatalf("newMetricsProjector() error = %v", err)
	}
	startedAt := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(time.Second)
	err = projector.ObserveStageInterval(context.Background(), &platformcontract.StageInterval{
		ScopeRef: platformcontract.TimelineScopeRef{
			Scope:     platformcontract.TimelineScopeTurn,
			SessionID: "session-1",
			TurnID:    "turn-1",
		},
		Stage:     "EXECUTE",
		Subject:   "run",
		Action:    "execute",
		Status:    platformcontract.TimelineStageStatusSucceeded,
		StartedAt: startedAt,
		EndedAt:   &endedAt,
	})
	if err != nil {
		t.Fatalf("ObserveStageInterval() error = %v", err)
	}
	if got := histogramCount(t, reader, stageDurationMetric); got != 1 {
		t.Fatalf("stage duration count = %d, want 1", got)
	}
}

func histogramCount(t *testing.T, reader *sdkmetric.ManualReader, name string) uint64 {
	t.Helper()
	var resourceMetrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &resourceMetrics); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	for _, scopeMetrics := range resourceMetrics.ScopeMetrics {
		for _, observed := range scopeMetrics.Metrics {
			if observed.Name != name {
				continue
			}
			histogram, ok := observed.Data.(metricdata.Histogram[float64])
			if !ok || len(histogram.DataPoints) == 0 {
				t.Fatalf("metric %q data = %T, want float64 histogram", name, observed.Data)
			}
			return histogram.DataPoints[0].Count
		}
	}
	t.Fatalf("metric %q not found", name)
	return 0
}
