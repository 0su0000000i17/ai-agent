package computer

import "time"

type WorldState struct {
	LastObservation *Observation  `json:"last_observation,omitempty"`
	LastAction      *ActionRecord `json:"last_action,omitempty"`
	UpdatedAt       time.Time     `json:"updated_at,omitempty"`
}

type Observation struct {
	ActiveWindow *WindowInfo   `json:"active_window,omitempty"`
	Windows      []WindowInfo  `json:"windows"`
	Processes    []ProcessInfo `json:"processes"`
	ObservedAt   time.Time     `json:"observed_at"`
	Source       string        `json:"source"`
}

type WindowInfo struct {
	Title       string `json:"title"`
	ProcessName string `json:"process_name"`
	PID         int    `json:"pid"`
}

type ProcessInfo struct {
	Name string `json:"name"`
	PID  int    `json:"pid"`
}

type ActionRecord struct {
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Result    string    `json:"result"`
	CreatedAt time.Time `json:"created_at"`
}

type RuntimeState struct {
	world WorldState
}

func NewRuntimeState() *RuntimeState {
	return &RuntimeState{}
}

func (s *RuntimeState) SetObservation(obs Observation) {
	s.world.LastObservation = &obs
	s.world.UpdatedAt = time.Now()
}

func (s *RuntimeState) SetAction(action string, target string, result string) {
	s.world.LastAction = &ActionRecord{
		Action:    action,
		Target:    target,
		Result:    result,
		CreatedAt: time.Now(),
	}
	s.world.UpdatedAt = time.Now()
}

func (s *RuntimeState) Snapshot() WorldState {
	return s.world
}
