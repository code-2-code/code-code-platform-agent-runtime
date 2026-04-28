package sessionapi

import (
	"strings"

	"code-code.internal/platform-k8s/internal/platform/runevents"
	sessiondomain "code-code.internal/session"
	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
)

type aguiToolCallState struct {
	toolCallID      string
	parentMessageID string
	name            string
	args            string
	lastSeq         uint64
}

func (p *aguiTurnMessageProjector) toolCallStart(event runevents.OutputEvent, toolCallID, toolCallName, parentMessageID string) {
	if toolCallID == "" || toolCallName == "" {
		return
	}
	if parentMessageID == "" {
		parentMessageID = toolCallID
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	tool := p.ensureToolCallLocked(event, toolCallID)
	tool.parentMessageID = parentMessageID
	tool.name = toolCallName
	if event.Output.GetSequence() > tool.lastSeq {
		tool.lastSeq = event.Output.GetSequence()
	}
	p.ensureMessageLocked(event, parentMessageID)
}

func (p *aguiTurnMessageProjector) toolCallArgs(event runevents.OutputEvent, toolCallID, delta string) {
	if toolCallID == "" || delta == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	tool := p.ensureToolCallLocked(event, toolCallID)
	if event.Output.GetSequence() > tool.lastSeq {
		tool.args += delta
		tool.lastSeq = event.Output.GetSequence()
	}
}

func (p *aguiTurnMessageProjector) toolCallEnd(event runevents.OutputEvent, toolCallID string) (sessiondomain.TurnMessage, bool, error) {
	if toolCallID == "" {
		return sessiondomain.TurnMessage{}, false, nil
	}
	key := aguiToolCallKey(event, toolCallID)
	p.mu.Lock()
	tool := p.tools[key]
	if tool == nil || tool.name == "" {
		p.mu.Unlock()
		return sessiondomain.TurnMessage{}, false, nil
	}
	delete(p.tools, key)
	parentMessageID := tool.parentMessageID
	if parentMessageID == "" {
		parentMessageID = tool.toolCallID
	}
	message := p.ensureMessageLocked(event, parentMessageID)
	message.toolCalls = upsertAGUIToolCall(message.toolCalls, aguitypes.ToolCall{
		ID:   tool.toolCallID,
		Type: aguitypes.ToolCallTypeFunction,
		Function: aguitypes.FunctionCall{
			Name:      tool.name,
			Arguments: tool.args,
		},
	})
	snapshot := snapshotAGUITextMessage(message)
	p.mu.Unlock()
	return buildAGUITurnMessage(snapshot)
}

func (p *aguiTurnMessageProjector) toolCallChunk(event runevents.OutputEvent, toolCallID, toolCallName, parentMessageID, delta string) (sessiondomain.TurnMessage, bool, error) {
	if toolCallID == "" {
		return sessiondomain.TurnMessage{}, false, nil
	}
	p.mu.Lock()
	tool := p.ensureToolCallLocked(event, toolCallID)
	if parentMessageID != "" {
		tool.parentMessageID = parentMessageID
	}
	if toolCallName != "" {
		tool.name = toolCallName
	}
	if delta != "" && event.Output.GetSequence() > tool.lastSeq {
		tool.args += delta
		tool.lastSeq = event.Output.GetSequence()
	}
	if tool.name == "" {
		p.mu.Unlock()
		return sessiondomain.TurnMessage{}, false, nil
	}
	message := p.ensureMessageLocked(event, tool.parentMessageID)
	message.toolCalls = upsertAGUIToolCall(message.toolCalls, aguitypes.ToolCall{
		ID:   tool.toolCallID,
		Type: aguitypes.ToolCallTypeFunction,
		Function: aguitypes.FunctionCall{
			Name:      tool.name,
			Arguments: tool.args,
		},
	})
	snapshot := snapshotAGUITextMessage(message)
	p.mu.Unlock()
	return buildAGUITurnMessage(snapshot)
}

func (p *aguiTurnMessageProjector) toolResult(event runevents.OutputEvent, messageID, toolCallID, content string) (sessiondomain.TurnMessage, bool, error) {
	if messageID == "" || toolCallID == "" {
		return sessiondomain.TurnMessage{}, false, nil
	}
	runID := strings.TrimSpace(event.RunID)
	if runID == "" {
		return sessiondomain.TurnMessage{}, false, nil
	}
	message, err := newToolTurnMessage(toolTurnMessageInput{
		sessionID:  event.SessionID,
		turnID:     turnIDFromRunID(runID),
		runID:      runID,
		messageID:  persistentAGUIMessageID(string(aguitypes.RoleTool), runID, messageID),
		toolCallID: toolCallID,
		content:    content,
		sequence:   int64(event.Output.GetSequence()),
	})
	return message, true, err
}

func (p *aguiTurnMessageProjector) ensureToolCallLocked(event runevents.OutputEvent, toolCallID string) *aguiToolCallState {
	key := aguiToolCallKey(event, toolCallID)
	tool := p.tools[key]
	if tool != nil {
		return tool
	}
	tool = &aguiToolCallState{
		toolCallID:      toolCallID,
		parentMessageID: toolCallID,
	}
	p.tools[key] = tool
	return tool
}

func (p *aguiTurnMessageProjector) clearRun(sessionID, runID string) {
	prefix := strings.TrimSpace(sessionID) + "/" + strings.TrimSpace(runID) + "/"
	if p == nil || strings.TrimSpace(runID) == "" || prefix == "//" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for key := range p.messages {
		if strings.HasPrefix(key, prefix) {
			delete(p.messages, key)
		}
	}
	for key := range p.tools {
		if strings.HasPrefix(key, prefix) {
			delete(p.tools, key)
		}
	}
}

func aguiToolCallKey(event runevents.OutputEvent, toolCallID string) string {
	return strings.TrimSpace(event.SessionID) + "/" + strings.TrimSpace(event.RunID) + "/" + strings.TrimSpace(toolCallID)
}

func upsertAGUIToolCall(calls []aguitypes.ToolCall, call aguitypes.ToolCall) []aguitypes.ToolCall {
	for index := range calls {
		if calls[index].ID == call.ID {
			calls[index] = call
			return calls
		}
	}
	return append(calls, call)
}
