package world

import (
	"sync"
	"time"
)

type WindowRef struct {
	HWND        uintptr   `json:"hwnd"`
	Title       string    `json:"title"`
	ProcessName string    `json:"process_name"`
	PID         int       `json:"pid"`
	X           int       `json:"x"`
	Y           int       `json:"y"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	Visible     bool      `json:"visible"`
	Minimized   bool      `json:"minimized"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ProcessRef struct {
	PID       int       `json:"pid"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updated_at"`
}

type WindowSnapshot struct {
	ActiveWindow   *WindowRef   `json:"active_window,omitempty"`
	LastUserWindow *WindowRef   `json:"last_user_window,omitempty"`
	VisibleWindows []WindowRef  `json:"visible_windows"`
	Processes      []ProcessRef `json:"processes"`
	ObservedAt     time.Time    `json:"observed_at"`
}

type ScreenCaptureRef struct {
	Path      string    `json:"path"`
	Window    WindowRef `json:"window"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	CreatedAt time.Time `json:"created_at"`
	KeepFile  bool      `json:"keep_file"`
}

type OCRWord struct {
	Text       string  `json:"text"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	Confidence float64 `json:"confidence,omitempty"`
}

type OCRLine struct {
	Text  string    `json:"text"`
	Words []OCRWord `json:"words"`
}

type OCRSnapshot struct {
	Window     WindowRef `json:"window"`
	SourcePath string    `json:"source_path,omitempty"`
	FullText   string    `json:"full_text"`
	Lines      []OCRLine `json:"lines"`
	Words      []OCRWord `json:"words"`
	CapturedAt time.Time `json:"captured_at"`
}

type VisionEntity struct {
	Kind        string  `json:"kind"`
	Label       string  `json:"label"`
	Description string  `json:"description,omitempty"`
	X           float64 `json:"x,omitempty"`
	Y           float64 `json:"y,omitempty"`
	Width       float64 `json:"width,omitempty"`
	Height      float64 `json:"height,omitempty"`
	Confidence  float64 `json:"confidence,omitempty"`
}

type VisionSnapshot struct {
	Window          WindowRef      `json:"window"`
	SourcePath      string         `json:"source_path,omitempty"`
	Summary         string         `json:"summary"`
	VisibleText     []string       `json:"visible_text"`
	Entities        []VisionEntity `json:"entities"`
	PossibleActions []string       `json:"possible_actions"`
	Warnings        []string       `json:"warnings,omitempty"`
	Raw             string         `json:"raw,omitempty"`
	CapturedAt      time.Time      `json:"captured_at"`
}

type WorldState struct {
	mu sync.RWMutex

	Windows    WindowSnapshot    `json:"windows"`
	LastScreen *ScreenCaptureRef `json:"last_screen,omitempty"`
	LastOCR    *OCRSnapshot      `json:"last_ocr,omitempty"`
	LastVision *VisionSnapshot   `json:"last_vision,omitempty"`
}

func NewWorldState() *WorldState {
	return &WorldState{}
}

func (s *WorldState) SetWindowSnapshot(snapshot WindowSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Windows = snapshot
}

func (s *WorldState) SetScreenCapture(capture ScreenCaptureRef) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.LastScreen = &capture
}

func (s *WorldState) SetOCRSnapshot(snapshot OCRSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.LastOCR = &snapshot
}

func (s *WorldState) SetVisionSnapshot(snapshot VisionSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.LastVision = &snapshot
}

func (s *WorldState) Snapshot() WindowSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.Windows
}

func (s *WorldState) ScreenSnapshot() *ScreenCaptureRef {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.LastScreen == nil {
		return nil
	}

	copy := *s.LastScreen
	return &copy
}

func (s *WorldState) OCRSnapshot() *OCRSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.LastOCR == nil {
		return nil
	}

	copy := *s.LastOCR
	return &copy
}

func (s *WorldState) VisionSnapshot() *VisionSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.LastVision == nil {
		return nil
	}

	copy := *s.LastVision
	return &copy
}
