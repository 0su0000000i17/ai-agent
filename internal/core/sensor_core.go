package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"jarvis/internal/actions"
	"jarvis/internal/scene"
	ocrsensor "jarvis/internal/sensors/ocr"
	screensensor "jarvis/internal/sensors/screen"
	uisensor "jarvis/internal/sensors/ui"
	visionsensor "jarvis/internal/sensors/vision"
	windowsensor "jarvis/internal/sensors/window"
	"jarvis/internal/world"
)

type SensorCore struct {
	world *world.WorldState

	windowSensor *windowsensor.Sensor
	screenSensor *screensensor.Sensor
	uiSensor     *uisensor.Sensor
	ocrSensor    *ocrsensor.Sensor
	visionSensor *visionsensor.Sensor

	sceneFusion  *scene.FusionEngine
	actionEngine *actions.Engine

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
}

func NewSensorCore(
	worldState *world.WorldState,
	windowSensor *windowsensor.Sensor,
	screenSensor *screensensor.Sensor,
	uiSensor *uisensor.Sensor,
	ocrSensor *ocrsensor.Sensor,
	visionSensor *visionsensor.Sensor,
	sceneFusion *scene.FusionEngine,
	actionEngine *actions.Engine,
) *SensorCore {
	return &SensorCore{
		world: worldState,

		windowSensor: windowSensor,
		screenSensor: screenSensor,
		uiSensor:     uiSensor,
		ocrSensor:    ocrSensor,
		visionSensor: visionSensor,

		sceneFusion:  sceneFusion,
		actionEngine: actionEngine,
	}
}

func (s *SensorCore) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return
	}

	runCtx, cancel := context.WithCancel(ctx)

	s.cancel = cancel
	s.running = true

	go s.loop(runCtx)
}

func (s *SensorCore) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	s.running = false
}

func (s *SensorCore) loop(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var previous world.WindowSnapshot

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			snapshot, err := s.windowSensor.Observe(ctx, previous)
			if err == nil {
				previous = snapshot
			}
		}
	}
}

func (s *SensorCore) WindowReport() string {
	snapshot := s.world.Snapshot()

	var b strings.Builder

	b.WriteString("Window Sensor Report\n\n")

	if snapshot.ActiveWindow != nil {
		b.WriteString("Active window:\n")
		b.WriteString(formatWindow(*snapshot.ActiveWindow))
		b.WriteString("\n")
	}

	if snapshot.LastUserWindow != nil {
		b.WriteString("Last user window:\n")
		b.WriteString(formatWindow(*snapshot.LastUserWindow))
		b.WriteString("\n")
	}

	b.WriteString("Visible windows:\n")
	if len(snapshot.VisibleWindows) == 0 {
		b.WriteString("- none\n")
	} else {
		limit := len(snapshot.VisibleWindows)
		if limit > 20 {
			limit = 20
		}

		for i := 0; i < limit; i++ {
			b.WriteString(formatWindow(snapshot.VisibleWindows[i]))
		}

		if len(snapshot.VisibleWindows) > limit {
			b.WriteString(fmt.Sprintf("- ...and %d more\n", len(snapshot.VisibleWindows)-limit))
		}
	}

	b.WriteString(fmt.Sprintf("\nProcesses: %d\n", len(snapshot.Processes)))
	b.WriteString("Observed at: ")
	b.WriteString(snapshot.ObservedAt.Format(time.RFC3339))

	return b.String()
}

func (s *SensorCore) WindowJSON() string {
	raw, _ := json.MarshalIndent(s.world.Snapshot(), "", "  ")
	return string(raw)
}

func (s *SensorCore) CaptureLastUserWindowReport(ctx context.Context, keepFile bool) string {
	snapshot := s.world.Snapshot()

	if snapshot.LastUserWindow == nil {
		return "Screen Sensor: last user window is empty."
	}

	capture, err := s.screenSensor.CaptureWindow(
		ctx,
		*snapshot.LastUserWindow,
		screensensor.CaptureOptions{
			KeepFile: keepFile,
		},
	)
	if err != nil {
		return "Screen Sensor error: " + err.Error()
	}

	var b strings.Builder

	b.WriteString("Screen Sensor Report\n\n")
	b.WriteString("Target window:\n")
	b.WriteString(formatWindow(capture.Window))
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("Captured size: %dx%d\n", capture.Width, capture.Height))

	if keepFile {
		b.WriteString("Debug screenshot saved:\n")
		b.WriteString(capture.Path)
	} else {
		b.WriteString("Screenshot was captured and deleted after use.")
	}

	return b.String()
}

func (s *SensorCore) UISensorReport(ctx context.Context) string {
	snapshot := s.world.Snapshot()

	if snapshot.LastUserWindow == nil {
		return "UI Sensor: last user window is empty."
	}

	uiSnapshot, err := s.uiSensor.SnapshotWindow(
		ctx,
		*snapshot.LastUserWindow,
		uisensor.SnapshotOptions{
			MaxDepth:    6,
			MaxChildren: 220,
		},
	)
	if err != nil {
		return "UI Sensor error: " + err.Error()
	}

	var b strings.Builder

	b.WriteString("UI Sensor Report\n\n")
	b.WriteString("Target window:\n")
	b.WriteString(formatWindow(uiSnapshot.Window))
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("UI elements found: %d\n\n", len(uiSnapshot.Elements)))

	if len(uiSnapshot.Elements) == 0 {
		b.WriteString("No UI elements exposed by Windows UI Automation for this window.\n")
		b.WriteString("This is normal for some Chromium/Electron/custom-rendered apps. Vision/OCR sensor will cover this later.")
		return b.String()
	}

	b.WriteString("Elements:\n")

	limit := len(uiSnapshot.Elements)
	if limit > 80 {
		limit = 80
	}

	for i := 0; i < limit; i++ {
		el := uiSnapshot.Elements[i]

		name := strings.TrimSpace(el.Name)
		if name == "" {
			name = strings.TrimSpace(el.AutomationID)
		}
		if name == "" {
			name = strings.TrimSpace(el.ClassName)
		}
		if name == "" {
			name = "—"
		}

		b.WriteString(fmt.Sprintf(
			"- %s | type=%s | id=%s | depth=%d | %.0f,%.0f %.0fx%.0f\n",
			name,
			empty(el.ControlType),
			el.ID,
			el.Depth,
			el.X,
			el.Y,
			el.Width,
			el.Height,
		))
	}

	if len(uiSnapshot.Elements) > limit {
		b.WriteString(fmt.Sprintf("- ...and %d more elements\n", len(uiSnapshot.Elements)-limit))
	}

	return b.String()
}

func (s *SensorCore) UIJSON(ctx context.Context) string {
	snapshot := s.world.Snapshot()

	if snapshot.LastUserWindow == nil {
		return `{"error":"last user window is empty"}`
	}

	uiSnapshot, err := s.uiSensor.SnapshotWindow(
		ctx,
		*snapshot.LastUserWindow,
		uisensor.SnapshotOptions{
			MaxDepth:    6,
			MaxChildren: 220,
		},
	)
	if err != nil {
		raw, _ := json.MarshalIndent(map[string]string{
			"error": err.Error(),
		}, "", "  ")
		return string(raw)
	}

	raw, _ := json.MarshalIndent(uiSnapshot, "", "  ")
	return string(raw)
}

func (s *SensorCore) OCRReport(ctx context.Context, keepDebugScreenshot bool) string {
	snapshot := s.world.Snapshot()

	if snapshot.LastUserWindow == nil {
		return "OCR Sensor: last user window is empty."
	}

	ocrSnapshot, err := s.ocrSensor.RecognizeWindow(
		ctx,
		*snapshot.LastUserWindow,
		ocrsensor.OCRCaptureOptions{
			KeepDebugScreenshot: keepDebugScreenshot,
		},
	)
	if err != nil {
		return "OCR Sensor error: " + err.Error()
	}

	var b strings.Builder

	b.WriteString("OCR Sensor Report\n\n")
	b.WriteString("Target window:\n")
	b.WriteString(formatWindow(ocrSnapshot.Window))
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("Lines: %d\n", len(ocrSnapshot.Lines)))
	b.WriteString(fmt.Sprintf("Words: %d\n\n", len(ocrSnapshot.Words)))

	if strings.TrimSpace(ocrSnapshot.FullText) == "" {
		b.WriteString("No text recognized.\n")
	} else {
		b.WriteString("Recognized text:\n")
		b.WriteString(trimLongText(ocrSnapshot.FullText, 3000))
		b.WriteString("\n")
	}

	if keepDebugScreenshot && ocrSnapshot.SourcePath != "" {
		b.WriteString("\nDebug screenshot:\n")
		b.WriteString(ocrSnapshot.SourcePath)
	} else {
		b.WriteString("\nTemporary screenshot was deleted after OCR.")
	}

	return b.String()
}

func (s *SensorCore) OCRJSON(ctx context.Context) string {
	snapshot := s.world.Snapshot()

	if snapshot.LastUserWindow == nil {
		return `{"error":"last user window is empty"}`
	}

	ocrSnapshot, err := s.ocrSensor.RecognizeWindow(
		ctx,
		*snapshot.LastUserWindow,
		ocrsensor.OCRCaptureOptions{
			KeepDebugScreenshot: true,
		},
	)
	if err != nil {
		raw, _ := json.MarshalIndent(map[string]string{
			"error": err.Error(),
		}, "", "  ")
		return string(raw)
	}

	raw, _ := json.MarshalIndent(ocrSnapshot, "", "  ")
	return string(raw)
}

func (s *SensorCore) VisionReport(ctx context.Context, keepDebugScreenshot bool) string {
	snapshot := s.world.Snapshot()

	if snapshot.LastUserWindow == nil {
		return "Vision Sensor: last user window is empty."
	}

	if s.visionSensor == nil {
		return "Vision Sensor: vision sensor is nil."
	}

	visionSnapshot, err := s.visionSensor.AnalyzeWindow(
		ctx,
		*snapshot.LastUserWindow,
		visionsensor.AnalyzeOptions{
			KeepDebugScreenshot: keepDebugScreenshot,
		},
	)
	if err != nil {
		return "Vision Sensor error: " + err.Error()
	}

	var b strings.Builder

	b.WriteString("Vision Sensor Report\n\n")
	b.WriteString("Target window:\n")
	b.WriteString(formatWindow(visionSnapshot.Window))
	b.WriteString("\n")

	b.WriteString("Summary:\n")
	b.WriteString(empty(visionSnapshot.Summary))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("Visible text snippets: %d\n", len(visionSnapshot.VisibleText)))
	b.WriteString(fmt.Sprintf("Vision entities: %d\n", len(visionSnapshot.Entities)))
	b.WriteString(fmt.Sprintf("Possible actions: %d\n\n", len(visionSnapshot.PossibleActions)))

	if len(visionSnapshot.VisibleText) > 0 {
		b.WriteString("Visible text:\n")

		limit := len(visionSnapshot.VisibleText)
		if limit > 30 {
			limit = 30
		}

		for i := 0; i < limit; i++ {
			text := strings.TrimSpace(visionSnapshot.VisibleText[i])
			if text == "" {
				continue
			}

			b.WriteString("- ")
			b.WriteString(text)
			b.WriteString("\n")
		}

		if len(visionSnapshot.VisibleText) > limit {
			b.WriteString(fmt.Sprintf("- ...and %d more text snippets\n", len(visionSnapshot.VisibleText)-limit))
		}

		b.WriteString("\n")
	}

	if len(visionSnapshot.Entities) > 0 {
		b.WriteString("Entities:\n")

		limit := len(visionSnapshot.Entities)
		if limit > 40 {
			limit = 40
		}

		for i := 0; i < limit; i++ {
			item := visionSnapshot.Entities[i]

			b.WriteString(fmt.Sprintf(
				"- %s | kind=%s | %.0f,%.0f %.0fx%.0f | conf=%.2f\n",
				empty(item.Label),
				empty(item.Kind),
				item.X,
				item.Y,
				item.Width,
				item.Height,
				item.Confidence,
			))
		}

		if len(visionSnapshot.Entities) > limit {
			b.WriteString(fmt.Sprintf("- ...and %d more entities\n", len(visionSnapshot.Entities)-limit))
		}
	}

	if len(visionSnapshot.PossibleActions) > 0 {
		b.WriteString("\nPossible actions:\n")

		for _, action := range visionSnapshot.PossibleActions {
			action = strings.TrimSpace(action)
			if action == "" {
				continue
			}

			b.WriteString("- ")
			b.WriteString(action)
			b.WriteString("\n")
		}
	}

	if len(visionSnapshot.Warnings) > 0 {
		b.WriteString("\nWarnings:\n")

		for _, warning := range visionSnapshot.Warnings {
			warning = strings.TrimSpace(warning)
			if warning == "" {
				continue
			}

			b.WriteString("- ")
			b.WriteString(warning)
			b.WriteString("\n")
		}
	}

	if keepDebugScreenshot && visionSnapshot.SourcePath != "" {
		b.WriteString("\nDebug screenshot:\n")
		b.WriteString(visionSnapshot.SourcePath)
	} else {
		b.WriteString("\nTemporary screenshot was deleted after vision analysis.")
	}

	return b.String()
}

func (s *SensorCore) VisionJSON(ctx context.Context) string {
	snapshot := s.world.Snapshot()

	if snapshot.LastUserWindow == nil {
		return `{"error":"last user window is empty"}`
	}

	if s.visionSensor == nil {
		return `{"error":"vision sensor is nil"}`
	}

	visionSnapshot, err := s.visionSensor.AnalyzeWindow(
		ctx,
		*snapshot.LastUserWindow,
		visionsensor.AnalyzeOptions{
			KeepDebugScreenshot: true,
		},
	)
	if err != nil {
		raw, _ := json.MarshalIndent(map[string]string{
			"error": err.Error(),
		}, "", "  ")
		return string(raw)
	}

	raw, _ := json.MarshalIndent(visionSnapshot, "", "  ")
	return string(raw)
}

func (s *SensorCore) SceneReport(ctx context.Context, keepDebugScreenshot bool, useVision bool) string {
	if s.sceneFusion == nil {
		return "Scene Fusion error: scene fusion engine is nil."
	}

	sceneSnapshot, err := s.sceneFusion.BuildLastUserWindowScene(
		ctx,
		scene.FusionOptions{
			KeepDebugScreenshot: keepDebugScreenshot,
			UseVision:           useVision,
		},
	)
	if err != nil {
		return "Scene Fusion error: " + err.Error()
	}

	return sceneSnapshot.Summary
}

func (s *SensorCore) CurrentScene(ctx context.Context, useVision bool) (scene.Scene, error) {
	if s.sceneFusion == nil {
		return scene.Scene{}, fmt.Errorf("scene fusion engine is nil")
	}

	return s.sceneFusion.BuildLastUserWindowScene(
		ctx,
		scene.FusionOptions{
			KeepDebugScreenshot: true,
			UseVision:           useVision,
		},
	)
}

func (s *SensorCore) SceneForWindow(ctx context.Context, target world.WindowRef, useVision bool) (scene.Scene, error) {
	if s.sceneFusion == nil {
		return scene.Scene{}, fmt.Errorf("scene fusion engine is nil")
	}

	return s.sceneFusion.BuildWindowScene(
		ctx,
		target,
		scene.FusionOptions{
			KeepDebugScreenshot: true,
			UseVision:           useVision,
		},
	)
}

func (s *SensorCore) ActionSequenceForScene(
	ctx context.Context,
	initialScene scene.Scene,
	requests []actions.Request,
	useVision bool,
) string {
	if s.sceneFusion == nil {
		return "Action Engine error: scene fusion engine is nil."
	}

	if s.actionEngine == nil {
		return "Action Engine error: action engine is nil."
	}

	if len(requests) == 0 {
		return "Action Engine error: empty action sequence."
	}

	currentScene := initialScene

	var b strings.Builder

	b.WriteString("Action Engine Sequence Report\n\n")
	b.WriteString(fmt.Sprintf("Target window: %s [%s]\n", currentScene.Window.Title, currentScene.Window.ProcessName))
	b.WriteString(fmt.Sprintf("Actions: %d\n\n", len(requests)))

	for i, req := range requests {
		b.WriteString(fmt.Sprintf("Step %d/%d\n", i+1, len(requests)))

		actionResult := s.actionEngine.Execute(ctx, currentScene, req)

		time.Sleep(650 * time.Millisecond)

		afterScene, err := s.sceneFusion.BuildWindowScene(
			ctx,
			currentScene.Window,
			scene.FusionOptions{
				KeepDebugScreenshot: true,
				UseVision:           useVision,
			},
		)
		if err != nil {
			b.WriteString(formatActionResult(actionResult))
			b.WriteString("\nObserve-after error:\n")
			b.WriteString(err.Error())
			b.WriteString("\n")
			return b.String()
		}

		verification := VerifyAction(currentScene, afterScene, actionResult)

		b.WriteString(formatActionResult(actionResult))
		b.WriteString("\n")
		b.WriteString(verification.Summary)
		b.WriteString("\n")

		currentScene = afterScene

		if !actionResult.OK {
			b.WriteString("Sequence stopped because action failed.\n")
			return b.String()
		}
	}

	b.WriteString("Final scene:\n")
	b.WriteString(fmt.Sprintf("- window: %s [%s]\n", currentScene.Window.Title, currentScene.Window.ProcessName))
	b.WriteString(fmt.Sprintf("- ui_elements: %d\n", len(currentScene.UIElements)))
	b.WriteString(fmt.Sprintf("- ocr_lines: %d\n", len(currentScene.OCRLines)))
	b.WriteString(fmt.Sprintf("- entities: %d\n", len(currentScene.Entities)))

	return b.String()
}

func (s *SensorCore) SceneJSON(ctx context.Context, useVision bool) string {
	if s.sceneFusion == nil {
		return `{"error":"scene fusion engine is nil"}`
	}

	sceneSnapshot, err := s.sceneFusion.BuildLastUserWindowScene(
		ctx,
		scene.FusionOptions{
			KeepDebugScreenshot: true,
			UseVision:           useVision,
		},
	)
	if err != nil {
		raw, _ := json.MarshalIndent(map[string]string{
			"error": err.Error(),
		}, "", "  ")
		return string(raw)
	}

	raw, _ := json.MarshalIndent(sceneSnapshot, "", "  ")
	return string(raw)
}

func (s *SensorCore) ActionReport(ctx context.Context, req actions.Request, useVision bool) string {
	if s.sceneFusion == nil {
		return "Action Engine error: scene fusion engine is nil."
	}

	if s.actionEngine == nil {
		return "Action Engine error: action engine is nil."
	}

	before, err := s.sceneFusion.BuildLastUserWindowScene(
		ctx,
		scene.FusionOptions{
			KeepDebugScreenshot: true,
			UseVision:           useVision,
		},
	)
	if err != nil {
		return "Action Engine observe-before error: " + err.Error()
	}

	actionResult := s.actionEngine.Execute(ctx, before, req)

	time.Sleep(550 * time.Millisecond)

	after, err := s.sceneFusion.BuildLastUserWindowScene(
		ctx,
		scene.FusionOptions{
			KeepDebugScreenshot: true,
			UseVision:           useVision,
		},
	)
	if err != nil {
		var b strings.Builder

		b.WriteString("Action Engine Report\n\n")
		b.WriteString(formatActionResult(actionResult))
		b.WriteString("\nObserve-after error:\n")
		b.WriteString(err.Error())

		return b.String()
	}

	verification := VerifyAction(before, after, actionResult)

	var b strings.Builder

	b.WriteString("Action Engine Report\n\n")
	b.WriteString(formatActionResult(actionResult))
	b.WriteString("\n")
	b.WriteString(verification.Summary)
	b.WriteString("\nAfter scene:\n")
	b.WriteString(fmt.Sprintf("- window: %s [%s]\n", after.Window.Title, after.Window.ProcessName))
	b.WriteString(fmt.Sprintf("- ui_elements: %d\n", len(after.UIElements)))
	b.WriteString(fmt.Sprintf("- ocr_lines: %d\n", len(after.OCRLines)))
	b.WriteString(fmt.Sprintf("- entities: %d\n", len(after.Entities)))

	return b.String()
}

func formatActionResult(result actions.Result) string {
	var b strings.Builder

	b.WriteString("Action result\n")
	b.WriteString(fmt.Sprintf("- ok: %v\n", result.OK))
	b.WriteString(fmt.Sprintf("- action: %s\n", result.Action))

	if result.Target != "" {
		b.WriteString(fmt.Sprintf("- target: %s\n", result.Target))
	}

	if result.Message != "" {
		b.WriteString(fmt.Sprintf("- message: %s\n", result.Message))
	}

	if result.Error != "" {
		b.WriteString(fmt.Sprintf("- error: %s\n", result.Error))
	}

	return b.String()
}

func trimLongText(value string, limit int) string {
	value = strings.TrimSpace(value)

	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}

	return string(runes[:limit]) + "\n...text truncated..."
}

func formatWindow(w world.WindowRef) string {
	return fmt.Sprintf(
		"- %s [%s] pid=%d hwnd=%d rect=%d,%d %dx%d minimized=%v\n",
		empty(w.Title),
		empty(w.ProcessName),
		w.PID,
		w.HWND,
		w.X,
		w.Y,
		w.Width,
		w.Height,
		w.Minimized,
	)
}

func empty(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}

	return value
}
