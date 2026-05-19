package actions

import "time"

type ActionKind string

const (
	ActionFocusWindow ActionKind = "focus_window"
	ActionClickEntity ActionKind = "click_entity"
	ActionClickPoint  ActionKind = "click_point"
	ActionTypeText    ActionKind = "type_text"
	ActionPressKey    ActionKind = "press_key"
	ActionHotkey      ActionKind = "hotkey"
	ActionScroll      ActionKind = "scroll"
	ActionWait        ActionKind = "wait"
)

type Request struct {
	Kind ActionKind `json:"kind"`

	EntityID string `json:"entity_id,omitempty"`

	X int `json:"x,omitempty"`
	Y int `json:"y,omitempty"`

	Text string `json:"text,omitempty"`

	// Важно для replace_text:
	// если перед этим уже был ctrl+a, нельзя снова кликать в поле,
	// иначе выделение снимается.
	SkipTargetClick bool `json:"skip_target_click,omitempty"`

	Key  string   `json:"key,omitempty"`
	Keys []string `json:"keys,omitempty"`

	Direction string `json:"direction,omitempty"`
	Amount    int    `json:"amount,omitempty"`

	WaitMS int `json:"wait_ms,omitempty"`
}

type Result struct {
	OK        bool       `json:"ok"`
	Action    ActionKind `json:"action"`
	Target    string     `json:"target,omitempty"`
	Message   string     `json:"message,omitempty"`
	Error     string     `json:"error,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   time.Time  `json:"ended_at"`
}

func Success(kind ActionKind, target string, message string, startedAt time.Time) Result {
	return Result{
		OK:        true,
		Action:    kind,
		Target:    target,
		Message:   message,
		StartedAt: startedAt,
		EndedAt:   time.Now(),
	}
}

func Failure(kind ActionKind, target string, err error, startedAt time.Time) Result {
	msg := ""
	if err != nil {
		msg = err.Error()
	}

	return Result{
		OK:        false,
		Action:    kind,
		Target:    target,
		Error:     msg,
		StartedAt: startedAt,
		EndedAt:   time.Now(),
	}
}
