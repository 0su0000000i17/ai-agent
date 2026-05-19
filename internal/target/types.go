package target

import "jarvis/internal/world"

type DecisionKind string

const (
	DecisionNone           DecisionKind = "none"
	DecisionExistingWindow DecisionKind = "existing_window"
	DecisionLaunchApp      DecisionKind = "launch_app"
)

type Decision struct {
	Kind       DecisionKind `json:"kind"`
	HWND       uintptr      `json:"hwnd,omitempty"`
	AppQuery   string       `json:"app_query,omitempty"`
	Reason     string       `json:"reason,omitempty"`
	Confidence float64      `json:"confidence,omitempty"`
}

type ResolvedTarget struct {
	Window     world.WindowRef `json:"window"`
	Reason     string          `json:"reason"`
	Confidence float64         `json:"confidence"`
	Launched   bool            `json:"launched"`
}

type ResolveResult struct {
	HasTarget bool
	Target    ResolvedTarget
	Message   string
}
