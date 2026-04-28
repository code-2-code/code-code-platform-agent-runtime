package workflows

import "time"

// State carries one execution workflow observation snapshot.
type State struct {
	Phase      string
	Message    string
	StartedAt  *time.Time
	FinishedAt *time.Time
	Nodes      []NodeState
}

// NodeState carries one normalized execution node observation.
type NodeState struct {
	ID           string
	Name         string
	DisplayName  string
	TemplateName string
	Phase        string
	Message      string
	StartedAt    *time.Time
	FinishedAt   *time.Time
}
