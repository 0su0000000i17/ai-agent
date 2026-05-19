package planner

import "jarvis/internal/actions"

type DecisionType string

const (
	DecisionChatReply DecisionType = "chat_reply"
	DecisionAction    DecisionType = "action"
	DecisionDone      DecisionType = "done"
)

type Decision struct {
	Type DecisionType `json:"type"`

	Reply  string `json:"reply,omitempty"`
	Reason string `json:"reason,omitempty"`

	Action  actions.Request   `json:"action,omitempty"`
	Actions []actions.Request `json:"actions,omitempty"`
}
