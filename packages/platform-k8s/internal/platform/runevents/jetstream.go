package runevents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	runeventv1 "code-code.internal/go-contract/platform/run_event/v1"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
)

const (
	runDeltaStreamName       = "RUN_DELTA"
	runResultStreamName      = "RUN_RESULT"
	runDeltaSubjectTemplate  = "platform.run.delta.%s.%s"
	runResultSubjectTemplate = "platform.run.result.%s.%s"
	streamRetryInterval      = 500 * time.Millisecond
)

type Config struct {
	ClientName string
	NATSURL    string
}

type JetStreamReader struct {
	nc *nats.Conn
	js jetstream.JetStream
}

func NewJetStreamReader(config Config) (*JetStreamReader, error) {
	url := strings.TrimSpace(config.NATSURL)
	if url == "" {
		return nil, fmt.Errorf("platformk8s/internal/runevents: nats url is empty")
	}
	options := []nats.Option{}
	if name := strings.TrimSpace(config.ClientName); name != "" {
		options = append(options, nats.Name(name))
	}
	nc, err := nats.Connect(url, options...)
	if err != nil {
		return nil, fmt.Errorf("platformk8s/internal/runevents: connect: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("platformk8s/internal/runevents: create jetstream client: %w", err)
	}
	return &JetStreamReader{nc: nc, js: js}, nil
}

func (r *JetStreamReader) Stream(ctx context.Context, request Request, yield func(StreamEvent) error) error {
	if r == nil || r.js == nil {
		return fmt.Errorf("platformk8s/internal/runevents: reader is nil")
	}
	sessionID := strings.TrimSpace(request.SessionID)
	runID := strings.TrimSpace(request.RunID)
	if sessionID == "" || runID == "" {
		return fmt.Errorf("platformk8s/internal/runevents: session_id and run_id are required")
	}
	if yield == nil {
		return fmt.Errorf("platformk8s/internal/runevents: yield is nil")
	}
	subjectTokenValue := subjectToken(sessionID)
	runToken := subjectToken(runID)
	updates := make(chan StreamEvent, 32)
	errs := make(chan error, 2)
	readerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	afterSequence := request.AfterSequence
	go r.consume(readerCtx, runDeltaStreamName, fmt.Sprintf(runDeltaSubjectTemplate, subjectTokenValue, runToken), func(body []byte) (StreamEvent, bool, error) {
		return decodeDeltaEvent(body)
	}, afterSequence, updates, errs)
	go r.consume(readerCtx, runResultStreamName, fmt.Sprintf(runResultSubjectTemplate, subjectTokenValue, runToken), func(body []byte) (StreamEvent, bool, error) {
		return decodeResultEvent(body)
	}, afterSequence, updates, errs)
	completed := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errs:
			if err == nil {
				completed++
				if completed == 2 {
					return nil
				}
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		case event := <-updates:
			if event.Delta == nil && event.Result == nil {
				continue
			}
			if err := yield(event); err != nil {
				return err
			}
			if event.Result != nil && event.Result.GetTerminalResult() != nil {
				cancel()
			}
		}
	}
}

func (r *JetStreamReader) Close() {
	if r == nil || r.nc == nil {
		return
	}
	r.nc.Close()
}

func (r *JetStreamReader) consume(
	ctx context.Context,
	streamName string,
	subject string,
	decode func([]byte) (StreamEvent, bool, error),
	afterSequence uint64,
	updates chan<- StreamEvent,
	errs chan<- error,
) {
	err := r.consumeStream(ctx, streamName, subject, decode, afterSequence, updates)
	select {
	case <-ctx.Done():
		errs <- nil
	case errs <- err:
	}
}

func (r *JetStreamReader) consumeStream(
	ctx context.Context,
	streamName string,
	subject string,
	decode func([]byte) (StreamEvent, bool, error),
	afterSequence uint64,
	updates chan<- StreamEvent,
) error {
	stream, err := r.awaitStream(ctx, streamName)
	if err != nil {
		return err
	}
	consumer, err := stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		FilterSubjects:    []string{subject},
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		InactiveThreshold: time.Minute,
		MaxResetAttempts:  3,
	})
	if err != nil {
		return fmt.Errorf("platformk8s/internal/runevents: create consumer: %w", err)
	}
	messages, err := consumer.Messages()
	if err != nil {
		return fmt.Errorf("platformk8s/internal/runevents: open messages: %w", err)
	}
	defer messages.Stop()
	for {
		message, err := messages.Next(jetstream.NextContext(ctx))
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("platformk8s/internal/runevents: next message: %w", err)
		}
		event, ok, err := decode(message.Data())
		if ackErr := message.Ack(); ackErr != nil && ctx.Err() == nil {
			return fmt.Errorf("platformk8s/internal/runevents: ack message: %w", ackErr)
		}
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if sequence := eventSequence(event); sequence != 0 && sequence <= afterSequence {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case updates <- event:
		}
	}
}

func eventSequence(event StreamEvent) uint64 {
	switch {
	case event.Delta != nil && event.Delta.GetOutput() != nil:
		return event.Delta.GetOutput().GetSequence()
	case event.Result != nil && event.Result.GetOutput() != nil:
		return event.Result.GetOutput().GetSequence()
	default:
		return 0
	}
}

func (r *JetStreamReader) awaitStream(ctx context.Context, streamName string) (jetstream.Stream, error) {
	return awaitStream(ctx, r.js, streamName)
}

func awaitStream(ctx context.Context, js jetstream.JetStream, streamName string) (jetstream.Stream, error) {
	for {
		stream, err := js.Stream(ctx, streamName)
		if err == nil {
			return stream, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var apiErr *jetstream.APIError
		if !errors.As(err, &apiErr) || apiErr.ErrorCode != jetstream.JSErrCodeStreamNotFound {
			return nil, fmt.Errorf("platformk8s/internal/runevents: get stream %q: %w", streamName, err)
		}
		timer := time.NewTimer(streamRetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func decodeDeltaEvent(body []byte) (StreamEvent, bool, error) {
	delta := &runeventv1.RunDeltaEvent{}
	if err := proto.Unmarshal(body, delta); err != nil {
		return StreamEvent{}, false, fmt.Errorf("platformk8s/internal/runevents: unmarshal delta event: %w", err)
	}
	if delta.GetOutput() == nil {
		return StreamEvent{}, false, nil
	}
	return StreamEvent{Delta: delta}, true, nil
}

func decodeResultEvent(body []byte) (StreamEvent, bool, error) {
	result := &runeventv1.RunResultEvent{}
	if err := proto.Unmarshal(body, result); err != nil {
		return StreamEvent{}, false, fmt.Errorf("platformk8s/internal/runevents: unmarshal result event: %w", err)
	}
	if result.GetOutput() == nil && result.GetTerminalResult() == nil {
		return StreamEvent{}, false, nil
	}
	return StreamEvent{Result: result}, true, nil
}

func subjectToken(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = strings.ReplaceAll(normalized, ".", "_")
	normalized = strings.ReplaceAll(normalized, "*", "_")
	normalized = strings.ReplaceAll(normalized, ">", "_")
	if normalized == "" {
		return "_"
	}
	return normalized
}
