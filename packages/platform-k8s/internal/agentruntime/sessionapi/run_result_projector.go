package sessionapi

import (
	"context"
	"strings"
	"time"

	"code-code.internal/platform-k8s/internal/platform/runevents"
)

const terminalResultProjectorRetryDelay = 2 * time.Second

func (s *SessionServer) RunTerminalResultProjector(ctx context.Context, natsURL string) error {
	if strings.TrimSpace(natsURL) == "" {
		return nil
	}
	for {
		consumer, err := runevents.NewTerminalResultConsumer(runevents.TerminalResultConsumerConfig{
			ClientName: "platform-agent-runtime-service-terminal-results",
			NATSURL:    natsURL,
		})
		if err == nil {
			err = consumer.Run(ctx, s.recordTerminalResult)
			consumer.Close()
		}
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			timer := time.NewTimer(terminalResultProjectorRetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
			continue
		}
		return nil
	}
}

func (s *SessionServer) recordTerminalResult(ctx context.Context, terminal runevents.TerminalResult) error {
	if terminal.Result == nil || strings.TrimSpace(terminal.RunID) == "" {
		return nil
	}
	if err := s.agentRuns.PublishTerminalResult(ctx, terminal.RunID, terminal.Result); err != nil {
		return err
	}
	if s.turnOutputMessages != nil {
		s.turnOutputMessages.clearRun(terminal.SessionID, terminal.RunID)
	}
	return nil
}

func (s *SessionServer) RunOutputMessageProjector(ctx context.Context, natsURL string) error {
	if strings.TrimSpace(natsURL) == "" || s == nil || s.turnMessages == nil {
		return nil
	}
	for {
		consumer, err := runevents.NewOutputEventConsumer(runevents.OutputEventConsumerConfig{
			ClientName: "platform-agent-runtime-service-output-messages",
			NATSURL:    natsURL,
		})
		if err == nil {
			err = consumer.Run(ctx, s.recordAssistantTurnMessage)
			consumer.Close()
		}
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			timer := time.NewTimer(terminalResultProjectorRetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
			continue
		}
		return nil
	}
}
