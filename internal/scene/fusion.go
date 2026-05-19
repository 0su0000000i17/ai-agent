package scene

import (
	"context"
	"fmt"
	"strings"
	"time"

	ocrsensor "jarvis/internal/sensors/ocr"
	screensensor "jarvis/internal/sensors/screen"
	uisensor "jarvis/internal/sensors/ui"
	visionsensor "jarvis/internal/sensors/vision"
	"jarvis/internal/world"
)

type FusionEngine struct {
	world        *world.WorldState
	screenSensor *screensensor.Sensor
	uiSensor     *uisensor.Sensor
	ocrSensor    *ocrsensor.Sensor
	visionSensor *visionsensor.Sensor
}

type FusionOptions struct {
	KeepDebugScreenshot bool
	UseVision           bool
}

func NewFusionEngine(
	worldState *world.WorldState,
	screenSensor *screensensor.Sensor,
	uiSensor *uisensor.Sensor,
	ocrSensor *ocrsensor.Sensor,
	visionSensor *visionsensor.Sensor,
) *FusionEngine {
	return &FusionEngine{
		world:        worldState,
		screenSensor: screenSensor,
		uiSensor:     uiSensor,
		ocrSensor:    ocrSensor,
		visionSensor: visionSensor,
	}
}

func (f *FusionEngine) BuildLastUserWindowScene(ctx context.Context, opts FusionOptions) (Scene, error) {
	ws := f.world.Snapshot()

	if ws.LastUserWindow == nil {
		return Scene{}, fmt.Errorf("last user window is empty")
	}

	return f.BuildWindowScene(ctx, *ws.LastUserWindow, opts)
}

func (f *FusionEngine) BuildWindowScene(ctx context.Context, target world.WindowRef, opts FusionOptions) (Scene, error) {
	var screenshotPath string

	if f.screenSensor != nil {
		capture, err := f.screenSensor.CaptureWindow(ctx, target, screensensor.CaptureOptions{
			KeepFile: opts.KeepDebugScreenshot,
		})
		if err == nil {
			screenshotPath = capture.Path
		}
	}

	var uiEntities []UIEntity

	if f.uiSensor != nil {
		uiSnapshot, err := f.uiSensor.SnapshotWindow(ctx, target, uisensor.SnapshotOptions{
			MaxDepth:    6,
			MaxChildren: 220,
		})
		if err == nil {
			uiEntities = convertUIEntities(uiSnapshot.Elements)
		}
	}

	var ocrWords []TextEntity
	var ocrLines []TextLine

	if f.ocrSensor != nil {
		ocrSnapshot, err := f.ocrSensor.RecognizeWindow(ctx, target, ocrsensor.OCRCaptureOptions{
			KeepDebugScreenshot: opts.KeepDebugScreenshot,
		})
		if err == nil {
			ocrWords = convertOCRWords(ocrSnapshot.Words)
			ocrLines = convertOCRLines(ocrSnapshot.Lines)
		}
	}

	var visionSnapshot *world.VisionSnapshot

	if opts.UseVision && f.visionSensor != nil {
		visionResult, err := f.visionSensor.AnalyzeWindow(ctx, target, visionsensor.AnalyzeOptions{
			KeepDebugScreenshot: opts.KeepDebugScreenshot,
		})
		if err == nil {
			visionSnapshot = &visionResult
		} else {
			visionSnapshot = &world.VisionSnapshot{
				Window:  target,
				Summary: "Vision sensor failed: " + err.Error(),
				Warnings: []string{
					err.Error(),
				},
				CapturedAt: time.Now(),
			}
		}
	}

	entities := buildSceneEntities(uiEntities, ocrLines, visionSnapshot)

	scene := Scene{
		Window:     target,
		Screenshot: screenshotPath,
		UIElements: uiEntities,
		OCRWords:   ocrWords,
		OCRLines:   ocrLines,
		Vision:     visionSnapshot,
		Entities:   entities,
		Summary:    buildSceneSummary(target, uiEntities, ocrWords, ocrLines, visionSnapshot, entities),
		ObservedAt: time.Now(),
	}

	return scene, nil
}

func convertUIEntities(items []uisensor.UIElementFlat) []UIEntity {
	result := make([]UIEntity, 0, len(items))

	for _, item := range items {
		result = append(result, UIEntity{
			ID:           item.ID,
			Name:         strings.TrimSpace(item.Name),
			ControlType:  strings.TrimSpace(item.ControlType),
			AutomationID: strings.TrimSpace(item.AutomationID),
			ClassName:    strings.TrimSpace(item.ClassName),
			X:            item.X,
			Y:            item.Y,
			Width:        item.Width,
			Height:       item.Height,
			Enabled:      item.IsEnabled,
			Depth:        item.Depth,
		})
	}

	return result
}

func convertOCRWords(items []world.OCRWord) []TextEntity {
	result := make([]TextEntity, 0, len(items))

	for i, item := range items {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}

		result = append(result, TextEntity{
			ID:         fmt.Sprintf("ocr_word_%d", i+1),
			Text:       text,
			X:          item.X,
			Y:          item.Y,
			Width:      item.Width,
			Height:     item.Height,
			Confidence: item.Confidence,
		})
	}

	return result
}

func convertOCRLines(items []world.OCRLine) []TextLine {
	result := make([]TextLine, 0, len(items))

	for i, line := range items {
		words := make([]TextEntity, 0, len(line.Words))

		for j, word := range line.Words {
			text := strings.TrimSpace(word.Text)
			if text == "" {
				continue
			}

			words = append(words, TextEntity{
				ID:         fmt.Sprintf("ocr_line_%d_word_%d", i+1, j+1),
				Text:       text,
				X:          word.X,
				Y:          word.Y,
				Width:      word.Width,
				Height:     word.Height,
				Confidence: word.Confidence,
			})
		}

		text := strings.TrimSpace(line.Text)
		if text == "" && len(words) == 0 {
			continue
		}

		result = append(result, TextLine{
			ID:    fmt.Sprintf("ocr_line_%d", i+1),
			Text:  text,
			Words: words,
		})
	}

	return result
}

func buildSceneEntities(
	uiItems []UIEntity,
	lines []TextLine,
	visionSnapshot *world.VisionSnapshot,
) []SceneEntity {
	result := make([]SceneEntity, 0)

	for _, item := range uiItems {
		label := strings.TrimSpace(item.Name)
		if label == "" {
			label = strings.TrimSpace(item.AutomationID)
		}
		if label == "" {
			label = strings.TrimSpace(item.ClassName)
		}

		kind := classifyUIEntity(item)

		if kind == "container" && label == "" {
			continue
		}

		result = append(result, SceneEntity{
			ID:         "entity_" + item.ID,
			Kind:       kind,
			Label:      label,
			Source:     []string{"ui"},
			X:          item.X,
			Y:          item.Y,
			Width:      item.Width,
			Height:     item.Height,
			Confidence: uiConfidence(kind, label),
		})
	}

	for _, line := range lines {
		text := strings.TrimSpace(line.Text)
		if text == "" {
			continue
		}

		x, y, w, h := boundsFromWords(line.Words)

		result = append(result, SceneEntity{
			ID:         "entity_" + line.ID,
			Kind:       "text",
			Label:      text,
			Source:     []string{"ocr"},
			X:          x,
			Y:          y,
			Width:      w,
			Height:     h,
			Confidence: 0.55,
		})
	}

	if visionSnapshot != nil {
		for i, item := range visionSnapshot.Entities {
			label := strings.TrimSpace(item.Label)
			if label == "" {
				label = strings.TrimSpace(item.Description)
			}
			if label == "" {
				label = "vision entity"
			}

			kind := strings.TrimSpace(item.Kind)
			if kind == "" {
				kind = "unknown"
			}

			conf := item.Confidence
			if conf <= 0 {
				conf = 0.7
			}

			result = append(result, SceneEntity{
				ID:          fmt.Sprintf("entity_vision_%d", i+1),
				Kind:        kind,
				Label:       label,
				Source:      []string{"vision"},
				X:           item.X,
				Y:           item.Y,
				Width:       item.Width,
				Height:      item.Height,
				Confidence:  conf,
				Description: item.Description,
			})
		}
	}

	return result
}

func classifyUIEntity(item UIEntity) string {
	t := strings.ToLower(item.ControlType)
	name := strings.ToLower(item.Name)

	switch {
	case strings.Contains(t, "edit"):
		return "input"
	case strings.Contains(t, "button"):
		return "button"
	case strings.Contains(t, "listitem"):
		return "list_item"
	case strings.Contains(t, "menu"):
		return "menu"
	case strings.Contains(name, "search"):
		return "search"
	case strings.Contains(name, "write a message"):
		return "input"
	case strings.Contains(name, "message"):
		return "message_area"
	case strings.Contains(t, "window"):
		return "window"
	case strings.Contains(t, "text"):
		return "text"
	case strings.Contains(t, "group") || strings.Contains(t, "pane"):
		return "container"
	default:
		return "unknown"
	}
}

func uiConfidence(kind string, label string) float64 {
	if kind == "input" || kind == "button" || kind == "search" {
		return 0.9
	}

	if label != "" {
		return 0.65
	}

	return 0.45
}

func boundsFromWords(words []TextEntity) (float64, float64, float64, float64) {
	if len(words) == 0 {
		return 0, 0, 0, 0
	}

	minX := words[0].X
	minY := words[0].Y
	maxX := words[0].X + words[0].Width
	maxY := words[0].Y + words[0].Height

	for _, word := range words[1:] {
		if word.X < minX {
			minX = word.X
		}
		if word.Y < minY {
			minY = word.Y
		}

		right := word.X + word.Width
		bottom := word.Y + word.Height

		if right > maxX {
			maxX = right
		}
		if bottom > maxY {
			maxY = bottom
		}
	}

	return minX, minY, maxX - minX, maxY - minY
}

func buildSceneSummary(
	target world.WindowRef,
	uiItems []UIEntity,
	words []TextEntity,
	lines []TextLine,
	visionSnapshot *world.VisionSnapshot,
	entities []SceneEntity,
) string {
	var b strings.Builder

	b.WriteString("Scene Fusion Report\n\n")
	b.WriteString("Target window:\n")
	b.WriteString(fmt.Sprintf(
		"- %s [%s] pid=%d hwnd=%d rect=%d,%d %dx%d\n\n",
		empty(target.Title),
		empty(target.ProcessName),
		target.PID,
		target.HWND,
		target.X,
		target.Y,
		target.Width,
		target.Height,
	))

	b.WriteString(fmt.Sprintf("UI elements: %d\n", len(uiItems)))
	b.WriteString(fmt.Sprintf("OCR words: %d\n", len(words)))
	b.WriteString(fmt.Sprintf("OCR lines: %d\n", len(lines)))

	if visionSnapshot != nil {
		b.WriteString("Vision: enabled\n")
		b.WriteString("Vision summary: ")
		b.WriteString(empty(visionSnapshot.Summary))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("Vision entities: %d\n", len(visionSnapshot.Entities)))
	} else {
		b.WriteString("Vision: not used\n")
	}

	b.WriteString(fmt.Sprintf("Scene entities: %d\n\n", len(entities)))

	if visionSnapshot != nil && len(visionSnapshot.PossibleActions) > 0 {
		b.WriteString("Vision possible actions:\n")
		for _, action := range visionSnapshot.PossibleActions {
			action = strings.TrimSpace(action)
			if action == "" {
				continue
			}

			b.WriteString("- ")
			b.WriteString(action)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(entities) > 0 {
		b.WriteString("Top entities:\n")

		limit := len(entities)
		if limit > 50 {
			limit = 50
		}

		for i := 0; i < limit; i++ {
			item := entities[i]
			b.WriteString(fmt.Sprintf(
				"- %s | kind=%s | source=%s | %.0f,%.0f %.0fx%.0f | conf=%.2f\n",
				empty(item.Label),
				item.Kind,
				strings.Join(item.Source, "+"),
				item.X,
				item.Y,
				item.Width,
				item.Height,
				item.Confidence,
			))
		}

		if len(entities) > limit {
			b.WriteString(fmt.Sprintf("- ...and %d more entities\n", len(entities)-limit))
		}
	}

	return b.String()
}

func empty(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}

	return value
}
