package sessionapi

import (
	"strings"
	"sync"

	"code-code.internal/go-contract/agui"
	"code-code.internal/platform-k8s/internal/platform/runevents"
	sessiondomain "code-code.internal/session"
	aguievents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

type aguiTurnMessageProjector struct {
	mu       sync.Mutex
	messages map[string]*aguiTextMessage
	tools    map[string]*aguiToolCallState
}

type aguiTextMessage struct {
	sessionID string
	runID     string
	messageID string
	role      string
	text      string
	toolCalls []aguitypes.ToolCall
	sequence  int64
	lastSeq   uint64
}

func newAGUITurnMessageProjector() *aguiTurnMessageProjector {
	return &aguiTurnMessageProjector{
		messages: map[string]*aguiTextMessage{},
		tools:    map[string]*aguiToolCallState{},
	}
}

func (p *aguiTurnMessageProjector) Apply(event runevents.OutputEvent) (sessiondomain.TurnMessage, bool, error) {
	if p == nil || event.Output == nil || event.Output.GetEvent() == nil {
		return sessiondomain.TurnMessage{}, false, nil
	}
	fields := event.Output.GetEvent().GetFields()
	switch agui.EventType(event.Output) {
	case aguievents.EventTypeTextMessageStart:
		p.start(event, strings.TrimSpace(fields["messageId"].GetStringValue()), strings.TrimSpace(fields["role"].GetStringValue()))
		return sessiondomain.TurnMessage{}, false, nil
	case aguievents.EventTypeTextMessageContent:
		return p.append(event, strings.TrimSpace(fields["messageId"].GetStringValue()), fields["delta"].GetStringValue())
	case aguievents.EventTypeTextMessageEnd:
		return p.end(event, strings.TrimSpace(fields["messageId"].GetStringValue()))
	case aguievents.EventTypeToolCallStart:
		p.toolCallStart(
			event,
			strings.TrimSpace(fields["toolCallId"].GetStringValue()),
			strings.TrimSpace(fields["toolCallName"].GetStringValue()),
			strings.TrimSpace(fields["parentMessageId"].GetStringValue()),
		)
		return sessiondomain.TurnMessage{}, false, nil
	case aguievents.EventTypeToolCallArgs:
		p.toolCallArgs(event, strings.TrimSpace(fields["toolCallId"].GetStringValue()), fields["delta"].GetStringValue())
		return sessiondomain.TurnMessage{}, false, nil
	case aguievents.EventTypeToolCallEnd:
		return p.toolCallEnd(event, strings.TrimSpace(fields["toolCallId"].GetStringValue()))
	case aguievents.EventTypeToolCallChunk:
		return p.toolCallChunk(
			event,
			strings.TrimSpace(fields["toolCallId"].GetStringValue()),
			strings.TrimSpace(fields["toolCallName"].GetStringValue()),
			strings.TrimSpace(fields["parentMessageId"].GetStringValue()),
			fields["delta"].GetStringValue(),
		)
	case aguievents.EventTypeToolCallResult:
		return p.toolResult(
			event,
			strings.TrimSpace(fields["messageId"].GetStringValue()),
			strings.TrimSpace(fields["toolCallId"].GetStringValue()),
			fields["content"].GetStringValue(),
		)
	default:
		return sessiondomain.TurnMessage{}, false, nil
	}
}

func (p *aguiTurnMessageProjector) start(event runevents.OutputEvent, messageID, role string) {
	if messageID == "" {
		return
	}
	if role == "" {
		role = string(aguitypes.RoleAssistant)
	}
	key := aguiTextMessageKey(event, messageID)
	p.mu.Lock()
	defer p.mu.Unlock()
	if message := p.messages[key]; message != nil {
		message.sessionID = event.SessionID
		message.runID = event.RunID
		message.messageID = messageID
		message.role = role
		if message.sequence == 0 || int64(event.Output.GetSequence()) < message.sequence {
			message.sequence = int64(event.Output.GetSequence())
		}
		if event.Output.GetSequence() > message.lastSeq {
			message.lastSeq = event.Output.GetSequence()
		}
		return
	}
	p.messages[key] = &aguiTextMessage{
		sessionID: event.SessionID,
		runID:     event.RunID,
		messageID: messageID,
		role:      role,
		sequence:  int64(event.Output.GetSequence()),
		lastSeq:   event.Output.GetSequence(),
	}
}

func (p *aguiTurnMessageProjector) append(event runevents.OutputEvent, messageID, delta string) (sessiondomain.TurnMessage, bool, error) {
	if messageID == "" || delta == "" {
		return sessiondomain.TurnMessage{}, false, nil
	}
	p.mu.Lock()
	message := p.ensureMessageLocked(event, messageID)
	if event.Output.GetSequence() > message.lastSeq {
		message.text += delta
		message.lastSeq = event.Output.GetSequence()
	}
	snapshot := snapshotAGUITextMessage(message)
	p.mu.Unlock()
	return buildAGUITurnMessage(snapshot)
}

func (p *aguiTurnMessageProjector) end(event runevents.OutputEvent, messageID string) (sessiondomain.TurnMessage, bool, error) {
	if messageID == "" {
		return sessiondomain.TurnMessage{}, false, nil
	}
	key := aguiTextMessageKey(event, messageID)
	p.mu.Lock()
	message := p.messages[key]
	if message != nil {
		message.lastSeq = event.Output.GetSequence()
	}
	snapshot := snapshotAGUITextMessage(message)
	p.mu.Unlock()
	if message == nil {
		return sessiondomain.TurnMessage{}, false, nil
	}
	return buildAGUITurnMessage(snapshot)
}

func (p *aguiTurnMessageProjector) ensureMessageLocked(event runevents.OutputEvent, messageID string) *aguiTextMessage {
	key := aguiTextMessageKey(event, messageID)
	message := p.messages[key]
	if message != nil {
		return message
	}
	message = &aguiTextMessage{
		sessionID: event.SessionID,
		runID:     event.RunID,
		messageID: messageID,
		role:      string(aguitypes.RoleAssistant),
		sequence:  int64(event.Output.GetSequence()),
	}
	p.messages[key] = message
	return message
}

func snapshotAGUITextMessage(message *aguiTextMessage) aguiTextMessage {
	if message == nil {
		return aguiTextMessage{}
	}
	snapshot := *message
	snapshot.toolCalls = append([]aguitypes.ToolCall(nil), message.toolCalls...)
	return snapshot
}
