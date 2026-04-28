package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	platformcontract "code-code.internal/platform-contract"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// PublisherConfig defines NATS publishing settings.
type PublisherConfig struct {
	URL           string
	SubjectPrefix string
	ClientName    string
}

// Publisher publishes timeline records into NATS JetStream.
type Publisher struct {
	url           string
	subjectPrefix string
	clientName    string

	mu            sync.Mutex
	nc            *nats.Conn
	js            jetstream.JetStream
	streamEnsured bool
}

type publishedRecord struct {
	Kind       string                            `json:"kind"`
	ScopeRef   platformcontract.TimelineScopeRef `json:"scopeRef"`
	Stage      string                            `json:"stage,omitempty"`
	EventType  string                            `json:"eventType,omitempty"`
	Subject    string                            `json:"subject"`
	Action     string                            `json:"action"`
	Status     string                            `json:"status,omitempty"`
	StartedAt  string                            `json:"startedAt,omitempty"`
	EndedAt    string                            `json:"endedAt,omitempty"`
	OccurredAt string                            `json:"occurredAt,omitempty"`
	Attributes map[string]string                 `json:"attributes,omitempty"`
}

// NewPublisher creates one timeline publisher.
func NewPublisher(config PublisherConfig) (*Publisher, error) {
	if strings.TrimSpace(config.URL) == "" {
		return nil, fmt.Errorf("timeline/nats: url is empty")
	}
	if strings.TrimSpace(config.SubjectPrefix) == "" {
		return nil, fmt.Errorf("timeline/nats: subject prefix is empty")
	}
	return &Publisher{
		url:           strings.TrimSpace(config.URL),
		subjectPrefix: strings.Trim(strings.TrimSpace(config.SubjectPrefix), "."),
		clientName:    strings.TrimSpace(config.ClientName),
	}, nil
}

// Close releases publisher resources.
func (p *Publisher) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.nc != nil {
		p.nc.Close()
		p.nc = nil
		p.js = nil
		p.streamEnsured = false
	}
}

// PublishStageInterval publishes one stage interval record.
func (p *Publisher) PublishStageInterval(ctx context.Context, interval *platformcontract.StageInterval) error {
	if err := platformcontract.ValidateStageInterval(interval); err != nil {
		return err
	}
	payload := publishedRecord{
		Kind:       "stage_interval",
		ScopeRef:   interval.ScopeRef,
		Stage:      interval.Stage,
		Subject:    interval.Subject,
		Action:     interval.Action,
		Status:     string(interval.Status),
		StartedAt:  interval.StartedAt.UTC().Format(timeLayout),
		EndedAt:    formatTime(interval.EndedAt),
		Attributes: interval.Attributes,
	}
	return p.publish(ctx, stageSubject(interval.ScopeRef, p.subjectPrefix), payload)
}

// PublishEvent publishes one timeline event record.
func (p *Publisher) PublishEvent(ctx context.Context, event *platformcontract.TimelineEvent) error {
	if err := platformcontract.ValidateTimelineEvent(event); err != nil {
		return err
	}
	payload := publishedRecord{
		Kind:       "event",
		ScopeRef:   event.ScopeRef,
		EventType:  event.EventType,
		Subject:    event.Subject,
		Action:     event.Action,
		OccurredAt: event.OccurredAt.UTC().Format(timeLayout),
		Attributes: event.Attributes,
	}
	return p.publish(ctx, eventSubject(event.ScopeRef, p.subjectPrefix), payload)
}

func (p *Publisher) publish(ctx context.Context, subject string, payload publishedRecord) error {
	if p == nil {
		return fmt.Errorf("timeline/nats: publisher is nil")
	}
	js, err := p.jetStream(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("timeline/nats: marshal payload: %w", err)
	}
	if _, err := js.Publish(ctx, subject, body); err != nil {
		return fmt.Errorf("timeline/nats: publish %q: %w", subject, err)
	}
	return nil
}

func (p *Publisher) jetStream(ctx context.Context) (jetstream.JetStream, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.js != nil {
		return p.js, nil
	}
	options := []nats.Option{}
	if p.clientName != "" {
		options = append(options, nats.Name(p.clientName))
	}
	nc, err := nats.Connect(p.url, options...)
	if err != nil {
		return nil, fmt.Errorf("timeline/nats: connect: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("timeline/nats: create jetstream client: %w", err)
	}
	if err := p.ensureStreamLocked(ctx, js); err != nil {
		nc.Close()
		return nil, err
	}
	p.nc = nc
	p.js = js
	return p.js, nil
}

func (p *Publisher) ensureStreamLocked(ctx context.Context, js jetstream.JetStream) error {
	if p.streamEnsured {
		return nil
	}
	_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:        streamName,
		Description: "Platform timeline realtime projection stream.",
		Subjects: []string{
			fmt.Sprintf("%s.>", p.subjectPrefix),
		},
		Retention: jetstream.LimitsPolicy,
		MaxMsgs:   -1,
		MaxBytes:  -1,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   jetstream.FileStorage,
		Replicas:  1,
	})
	if err != nil {
		return fmt.Errorf("timeline/nats: ensure stream: %w", err)
	}
	p.streamEnsured = true
	return nil
}

func stageSubject(scope platformcontract.TimelineScopeRef, prefix string) string {
	return fmt.Sprintf("%s.%s.stage_interval", prefix, scope.Scope)
}

func eventSubject(scope platformcontract.TimelineScopeRef, prefix string) string {
	return fmt.Sprintf("%s.%s.event", prefix, scope.Scope)
}

func formatTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(timeLayout)
}

const timeLayout = "2006-01-02T15:04:05.999999999Z07:00"

const streamName = "PLATFORM_TIMELINE"

var _ platformcontract.TimelinePublisher = (*Publisher)(nil)
