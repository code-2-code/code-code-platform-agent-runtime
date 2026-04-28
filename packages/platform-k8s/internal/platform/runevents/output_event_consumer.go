package runevents

import (
	"context"
	"fmt"
	"strings"
	"time"

	outputv1 "code-code.internal/go-contract/agent/output/v1"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const defaultOutputEventConsumerName = "platform-agent-runtime-service-output-events"

type OutputEvent struct {
	SessionID string
	RunID     string
	Output    *outputv1.RunOutput
}

type OutputEventConsumerConfig struct {
	ClientName   string
	ConsumerName string
	NATSURL      string
}

type OutputEventConsumer struct {
	consumerName string
	nc           *nats.Conn
	js           jetstream.JetStream
}

func NewOutputEventConsumer(config OutputEventConsumerConfig) (*OutputEventConsumer, error) {
	url := strings.TrimSpace(config.NATSURL)
	if url == "" {
		return nil, fmt.Errorf("platformk8s/internal/runevents: output event nats url is empty")
	}
	options := []nats.Option{}
	if name := strings.TrimSpace(config.ClientName); name != "" {
		options = append(options, nats.Name(name))
	}
	nc, err := nats.Connect(url, options...)
	if err != nil {
		return nil, fmt.Errorf("platformk8s/internal/runevents: connect output event consumer: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("platformk8s/internal/runevents: create output event jetstream client: %w", err)
	}
	consumerName := strings.TrimSpace(config.ConsumerName)
	if consumerName == "" {
		consumerName = defaultOutputEventConsumerName
	}
	return &OutputEventConsumer{consumerName: consumerName, nc: nc, js: js}, nil
}

func (c *OutputEventConsumer) Run(ctx context.Context, yield func(context.Context, OutputEvent) error) error {
	if c == nil || c.js == nil {
		return fmt.Errorf("platformk8s/internal/runevents: output event consumer is nil")
	}
	if yield == nil {
		return fmt.Errorf("platformk8s/internal/runevents: output event yield is nil")
	}
	stream, err := awaitStream(ctx, c.js, runDeltaStreamName)
	if err != nil {
		return err
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          c.consumerName,
		Durable:       c.consumerName,
		FilterSubject: "platform.run.delta.>",
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    20,
	})
	if err != nil {
		return fmt.Errorf("platformk8s/internal/runevents: create output event consumer: %w", err)
	}
	messages, err := consumer.Messages()
	if err != nil {
		return fmt.Errorf("platformk8s/internal/runevents: open output event messages: %w", err)
	}
	defer messages.Stop()
	for {
		message, err := messages.Next(jetstream.NextContext(ctx))
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("platformk8s/internal/runevents: next output event message: %w", err)
		}
		event, ok, err := decodeDeltaEvent(message.Data())
		if err != nil {
			_ = message.Term()
			return err
		}
		output := OutputEvent{}
		if ok && event.Delta != nil && event.Delta.GetOutput() != nil {
			output = OutputEvent{
				SessionID: strings.TrimSpace(event.Delta.GetSessionId()),
				RunID:     strings.TrimSpace(event.Delta.GetRunId()),
				Output:    event.Delta.GetOutput(),
			}
		}
		if output.Output != nil {
			if err := yield(ctx, output); err != nil {
				_ = message.Nak()
				return err
			}
		}
		if err := message.Ack(); err != nil && ctx.Err() == nil {
			return fmt.Errorf("platformk8s/internal/runevents: ack output event message: %w", err)
		}
	}
}

func (c *OutputEventConsumer) Close() {
	if c == nil || c.nc == nil {
		return
	}
	c.nc.Close()
}
