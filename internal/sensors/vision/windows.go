package vision

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	screensensor "jarvis/internal/sensors/screen"
	"jarvis/internal/world"
)

func NewSensor(opts Options) *Sensor {
	return &Sensor{
		state:        opts.State,
		screenSensor: opts.ScreenSensor,
		analyzer:     opts.Analyzer,
	}
}

func (s *Sensor) AnalyzeWindow(ctx context.Context, target world.WindowRef, opts AnalyzeOptions) (world.VisionSnapshot, error) {
	if s.screenSensor == nil {
		return world.VisionSnapshot{}, fmt.Errorf("screen sensor is nil")
	}

	if s.analyzer == nil {
		return world.VisionSnapshot{}, fmt.Errorf("vision analyzer is nil")
	}

	capture, err := s.screenSensor.CaptureWindow(ctx, target, screensensor.CaptureOptions{
		KeepFile: true,
	})
	if err != nil {
		return world.VisionSnapshot{}, err
	}

	prompt := buildVisionPrompt(target)

	raw, err := s.analyzer.AnalyzeImage(ctx, prompt, capture.Path)
	if err != nil {
		if !opts.KeepDebugScreenshot {
			_ = os.Remove(capture.Path)
		}
		return world.VisionSnapshot{}, err
	}

	parsed := parseVisionResponse(raw)

	snapshot := world.VisionSnapshot{
		Window:          target,
		SourcePath:      capture.Path,
		Summary:         strings.TrimSpace(parsed.Summary),
		VisibleText:     parsed.VisibleText,
		Entities:        parsed.Entities,
		PossibleActions: parsed.PossibleActions,
		Warnings:        parsed.Warnings,
		Raw:             raw,
		CapturedAt:      time.Now(),
	}

	if snapshot.Summary == "" {
		snapshot.Summary = "Vision model returned no summary."
	}

	if !opts.KeepDebugScreenshot {
		_ = os.Remove(capture.Path)
		snapshot.SourcePath = ""
	}

	if s.state != nil {
		s.state.SetVisionSnapshot(snapshot)
	}

	return snapshot, nil
}

type visionParsed struct {
	Summary         string               `json:"summary"`
	VisibleText     []string             `json:"visible_text"`
	Entities        []world.VisionEntity `json:"entities"`
	PossibleActions []string             `json:"possible_actions"`
	Warnings        []string             `json:"warnings"`
}

func parseVisionResponse(raw string) visionParsed {
	clean := extractJSON(raw)

	var parsed visionParsed
	if err := json.Unmarshal([]byte(clean), &parsed); err == nil {
		parsed.Summary = strings.TrimSpace(parsed.Summary)
		parsed.VisibleText = cleanStringSlice(parsed.VisibleText)
		parsed.PossibleActions = cleanStringSlice(parsed.PossibleActions)
		parsed.Warnings = cleanStringSlice(parsed.Warnings)

		for i := range parsed.Entities {
			parsed.Entities[i].Kind = strings.TrimSpace(parsed.Entities[i].Kind)
			parsed.Entities[i].Label = strings.TrimSpace(parsed.Entities[i].Label)
			parsed.Entities[i].Description = strings.TrimSpace(parsed.Entities[i].Description)
		}

		return parsed
	}

	return visionParsed{
		Summary: strings.TrimSpace(raw),
		Warnings: []string{
			"vision response was not valid JSON",
		},
	}
}

func buildVisionPrompt(target world.WindowRef) string {
	return fmt.Sprintf(`Analyze this Windows desktop application window as a vision sensor.

Target window:
- title: %s
- process: %s
- pid: %d
- rect: %d,%d %dx%d

Return ONLY valid JSON in this exact schema:

{
  "summary": "short natural-language description of what is visible",
  "visible_text": ["important visible text snippets"],
  "entities": [
    {
      "kind": "button|input|text|list|menu|tab|link|image|panel|window|unknown",
      "label": "visible label if any",
      "description": "what this element appears to be",
      "x": 0,
      "y": 0,
      "width": 0,
      "height": 0,
      "confidence": 0.0
    }
  ],
  "possible_actions": ["what a desktop agent could probably do next"],
  "warnings": ["uncertainties or limitations"]
}

Rules:
- Coordinates must be approximate pixels relative to this captured image.
- Do not invent hidden content.
- If text is unclear, mention uncertainty.
- Prefer useful interactive elements: search fields, message input, send buttons, navigation, tabs, menus, lists.
- Do not output markdown.
- Do not wrap JSON in code fences.
`, target.Title, target.ProcessName, target.PID, target.X, target.Y, target.Width, target.Height)
}

func extractJSON(raw string) string {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	start := strings.Index(clean, "{")
	end := strings.LastIndex(clean, "}")

	if start >= 0 && end > start {
		return clean[start : end+1]
	}

	return clean
}

func cleanStringSlice(items []string) []string {
	result := make([]string, 0, len(items))

	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		result = append(result, item)
	}

	return result
}
