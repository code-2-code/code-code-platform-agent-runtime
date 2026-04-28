package runevents

import (
	"context"
	"fmt"
	"strings"
	"time"

	resultv1 "code-code.internal/go-contract/agent/result/v1"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const defaultTerminalResultConsumerName = "platform-agent-runtime-service-terminal-results"

type TerminalResult struct {
	SessionID string
	RunID     string
	Result    *resultv1.RunResult
}

type TerminalResultConsumerConfig struct {
	ClientName   string
	ConsumerName string
	NATSURL      string
}

type TerminalResultConsumer struct {
	consumerName string
	nc           *nats.Conn
	js           jetstream.JetStream
}

func NewTerminalResultConsumer(config TerminalResultConsumerConfig) (*TerminalResultConsumer, error) {
	url := strings.TrimSpace(config.NATSURL)
	if url == "" {
		return nil, fmt.Errorf("platformk8s/internal/runevents: terminal result nats url is empty")
	}
	options := []nats.Option{}
	if name := strings.TrimSpace(config.ClientName); name != "" {
		options = append(options, nats.Name(name))
	}
	nc, err := nats.Connect(url, options...)
	if err != nil {
		return nil, fmt.Errorf("platformk8s/internal/runevents: connect terminal result consumer: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("platformk8s/internal/runevents: create terminal result jetstream client: %w", err)
	}
	consumerName := strings.TrimSpace(config.ConsumerName)
	if consumerName == "" {
		consumerName = defaultTerminalResultConsumerName
	}
	return &TerminalResultConsumer{consumerName: consumerName, nc: nc, js: js}, nil
}

func (c *TerminalResultConsumer) Run(ctx context.Context, yield func(context.Context, TerminalResult) error) error {
	if c == nil || c.js == nil {
		return fmt.Errorf("platformk8s/internal/runevents: terminal result consumer is nil")
	}
	if yield == nil {
		return fmt.Errorf("platformk8s/internal/runevents: terminal result yield is nil")
	}
	stream, err := awaitStream(ctx, c.js, runResultStreamName)
	if err != nil {
		return err
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          c.consumerName,
		Durable:       c.consumerName,
		FilterSubject: "platform.run.result.>",
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    20,
	})
	if err != nil {
		return fmt.Errorf("platformk8s/internal/runevents: create terminal result consumer: %w", err)
	}
	messages, err := consumer.Messages()
	if err != nil {
		return fmt.Errorf("platformk8s/internal/runevents: open terminal result messages: %w", err)
	}
	defer messages.Stop()
	for {
		message, err := messages.Next(jetstream.NextContext(ctx))
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("platformk8s/internal/runevents: next terminal result message: %w", err)
		}
		event, ok, err := decodeResultEvent(message.Data())
		if err != nil {
			_ = message.Term()
			return err
		}
		terminal := TerminalResult{}
		if ok && event.Result != nil && event.Result.GetTerminalResult() != nil {
			terminal = TerminalResult{
				SessionID: strings.TrimSpace(event.Result.GetSessionId()),
				RunID:     strings.TrimSpace(event.Result.GetRunId()),
				Result:    event.Result.GetTerminalResult(),
			}
		}
		if terminal.Result != nil {
			if err := yield(ctx, terminal); err != nil {
				_ = message.Nak()
				return err
			}
		}
		if err := message.Ack(); err != nil && ctx.Err() == nil {
			return fmt.Errorf("platformk8s/internal/runevents: ack terminal result message: %w", err)
		}
	}
}

func (c *TerminalResultConsumer) Close() {
	if c == nil || c.nc == nil {
		return
	}
	c.nc.Close()
}
