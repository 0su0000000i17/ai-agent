package scene

import (
	"time"

	"jarvis/internal/world"
)

type Scene struct {
	Window     world.WindowRef `json:"window"`
	Screenshot string          `json:"screenshot,omitempty"`

	UIElements []UIEntity            `json:"ui_elements"`
	OCRWords   []TextEntity          `json:"ocr_words"`
	OCRLines   []TextLine            `json:"ocr_lines"`
	Vision     *world.VisionSnapshot `json:"vision,omitempty"`
	Entities   []SceneEntity         `json:"entities"`

	Summary    string    `json:"summary"`
	ObservedAt time.Time `json:"observed_at"`
}

type UIEntity struct {
	ID           string `json:"id"`
	Name         string `json:"name,omitempty"`
	ControlType  string `json:"control_type,omitempty"`
	AutomationID string `json:"automation_id,omitempty"`
	ClassName    string `json:"class_name,omitempty"`

	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`

	Enabled bool `json:"enabled"`
	Depth   int  `json:"depth"`
}

type TextEntity struct {
	ID         string  `json:"id"`
	Text       string  `json:"text"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	Confidence float64 `json:"confidence,omitempty"`
}

type TextLine struct {
	ID    string       `json:"id"`
	Text  string       `json:"text"`
	Words []TextEntity `json:"words"`
}

type SceneEntity struct {
	ID     string   `json:"id"`
	Kind   string   `json:"kind"`
	Label  string   `json:"label"`
	Source []string `json:"source"`

	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`

	Confidence  float64 `json:"confidence"`
	Description string  `json:"description,omitempty"`
}
