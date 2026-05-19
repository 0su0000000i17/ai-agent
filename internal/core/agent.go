package core

import (
	"context"
	"fmt"
	"strings"

	"jarvis/internal/actions"
	"jarvis/internal/computer"
	"jarvis/internal/ops"
	"jarvis/internal/planner"
	"jarvis/internal/scene"
	ocrsensor "jarvis/internal/sensors/ocr"
	screensensor "jarvis/internal/sensors/screen"
	uisensor "jarvis/internal/sensors/ui"
	visionsensor "jarvis/internal/sensors/vision"
	windowsensor "jarvis/internal/sensors/window"
	"jarvis/internal/target"
	"jarvis/internal/types"
	"jarvis/internal/world"
)

type Model interface {
	Send(messages []types.Message) (string, error)
}

type Agent struct {
	model Model

	world         *world.WorldState
	sensorCore    *SensorCore
	planner       *planner.Planner // legacy planner, пока оставляем для dev/GoalLoop, но НЕ используем по дефолту
	opPlanner     *ops.Planner
	targetManager *target.Manager

	state  *computer.RuntimeState
	input  *computer.InputEngine
	screen *computer.ScreenEngine
	ui     *computer.UIEngine
}

func NewAgent(model Model) *Agent {
	worldState := world.NewWorldState()

	windowSensor := windowsensor.NewSensor(windowsensor.Options{
		State: worldState,
	})

	screenSensor := screensensor.NewSensor(screensensor.Options{
		State: worldState,
	})

	uiSensor := uisensor.NewSensor(uisensor.Options{
		State: worldState,
	})

	ocrSensor := ocrsensor.NewSensor(ocrsensor.Options{
		State:        worldState,
		ScreenSensor: screenSensor,
	})

	var visionAnalyzer visionsensor.Analyzer
	if analyzer, ok := model.(visionsensor.Analyzer); ok {
		visionAnalyzer = analyzer
	}

	visionSensor := visionsensor.NewSensor(visionsensor.Options{
		State:        worldState,
		ScreenSensor: screenSensor,
		Analyzer:     visionAnalyzer,
	})

	sceneFusion := scene.NewFusionEngine(
		worldState,
		screenSensor,
		uiSensor,
		ocrSensor,
		visionSensor,
	)

	actionEngine := actions.NewEngine()

	sensorCore := NewSensorCore(
		worldState,
		windowSensor,
		screenSensor,
		uiSensor,
		ocrSensor,
		visionSensor,
		sceneFusion,
		actionEngine,
	)

	targetResolver := target.NewResolver(model)
	targetManager := target.NewManager(worldState, targetResolver, windowSensor)

	computerState := computer.NewRuntimeState()
	input := computer.NewInputEngine(computerState)
	screen := computer.NewScreenEngine(computerState)
	ui := computer.NewUIEngine(computerState)

	return &Agent{
		model: model,

		world:         worldState,
		sensorCore:    sensorCore,
		planner:       planner.NewPlanner(model),
		opPlanner:     ops.NewPlanner(model),
		targetManager: targetManager,

		state:  computerState,
		input:  input,
		screen: screen,
		ui:     ui,
	}
}

func (a *Agent) Start(ctx context.Context) {
	a.sensorCore.Start(ctx)
}

func (a *Agent) Stop() {
	a.sensorCore.Stop()
}

func (a *Agent) Handle(ctx context.Context, userMessage string) (string, error) {
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return "", nil
	}

	// Dev/manual action hooks.
	// Оставляем только для диагностики runtime.
	// Обычные пользовательские запросы должны идти через Operation Pipeline ниже.
	if req, ok := parseActionRequest(userMessage); ok {
		return a.sensorCore.ActionReport(ctx, req, wantsVision(userMessage)), nil
	}

	// Dev sensor reports.
	if isSceneJSONQuestion(userMessage) {
		return a.sensorCore.SceneJSON(ctx, wantsVision(userMessage)), nil
	}

	if isSceneQuestion(userMessage) {
		return a.sensorCore.SceneReport(ctx, true, wantsVision(userMessage)), nil
	}

	if isVisionJSONQuestion(userMessage) {
		return a.sensorCore.VisionJSON(ctx), nil
	}

	if isVisionSensorQuestion(userMessage) {
		return a.sensorCore.VisionReport(ctx, true), nil
	}

	if isWindowJSONQuestion(userMessage) {
		return a.sensorCore.WindowJSON(), nil
	}

	if isUIJSONQuestion(userMessage) {
		return a.sensorCore.UIJSON(ctx), nil
	}

	if isOCRJSONQuestion(userMessage) {
		return a.sensorCore.OCRJSON(ctx), nil
	}

	if isOCRSensorQuestion(userMessage) {
		return a.sensorCore.OCRReport(ctx, true), nil
	}

	if isUIScreenSensorQuestion(userMessage) {
		return a.sensorCore.UISensorReport(ctx), nil
	}

	if isScreenSensorQuestion(userMessage) {
		return a.sensorCore.CaptureLastUserWindowReport(ctx, true), nil
	}

	if isWindowSensorQuestion(userMessage) {
		return a.sensorCore.WindowReport(), nil
	}

	// Main path:
	// natural language → operation → target → scene → compiled actions → verifier
	if reply, ok := a.tryOperationPipeline(ctx, userMessage); ok {
		return reply, nil
	}

	return a.Chat(ctx, userMessage)
}

func (a *Agent) tryOperationPipeline(ctx context.Context, userMessage string) (string, bool) {
	if a.opPlanner == nil || a.targetManager == nil || a.sensorCore == nil {
		return "", false
	}

	decision, err := a.opPlanner.Plan(ctx, userMessage)
	if err != nil {
		return "Operation planner error: " + err.Error(), true
	}

	if decision.Type == ops.DecisionChatReply {
		reply := strings.TrimSpace(decision.Reply)
		if reply == "" {
			return "", false
		}

		return reply, true
	}

	if decision.Type != ops.DecisionOperation {
		return "", false
	}

	operation := decision.Operation

	targetResult, err := a.targetManager.ResolveByHint(
		ctx,
		operation.TargetHint,
		operation.AppQuery,
	)
	if err != nil {
		return "Target resolve error: " + err.Error(), true
	}

	if !targetResult.HasTarget {
		message := strings.TrimSpace(targetResult.Message)
		if message == "" {
			message = "Не понял, в каком окне выполнить действие."
		}

		return message, true
	}

	sceneSnapshot, err := a.sensorCore.SceneForWindow(
		ctx,
		targetResult.Target.Window,
		wantsVision(userMessage),
	)
	if err != nil {
		return "Scene error for target: " + err.Error(), true
	}

	requests, err := ops.Compile(operation)
	if err != nil {
		return "Operation compile error: " + err.Error(), true
	}

	var b strings.Builder

	b.WriteString("Operation Pipeline Report\n\n")

	b.WriteString("Operation:\n")
	b.WriteString("- kind: ")
	b.WriteString(string(operation.Kind))
	b.WriteString("\n")

	b.WriteString("- target_hint: ")
	if operation.TargetHint == "" {
		b.WriteString("—")
	} else {
		b.WriteString(operation.TargetHint)
	}
	b.WriteString("\n")

	b.WriteString("- app_query: ")
	if operation.AppQuery == "" {
		b.WriteString("—")
	} else {
		b.WriteString(operation.AppQuery)
	}
	b.WriteString("\n")

	if operation.Text != "" {
		b.WriteString("- text: ")
		b.WriteString(operation.Text)
		b.WriteString("\n")
	}

	if operation.Direction != "" {
		b.WriteString("- direction: ")
		b.WriteString(operation.Direction)
		b.WriteString("\n")
	}

	if decision.Reason != "" {
		b.WriteString("- reason: ")
		b.WriteString(decision.Reason)
		b.WriteString("\n")
	}

	b.WriteString("\nTarget:\n")
	b.WriteString("- ")
	b.WriteString(targetResult.Target.Window.Title)
	b.WriteString(" [")
	b.WriteString(targetResult.Target.Window.ProcessName)
	b.WriteString("]\n")

	b.WriteString("- launched: ")
	if targetResult.Target.Launched {
		b.WriteString("true\n")
	} else {
		b.WriteString("false\n")
	}

	b.WriteString("- confidence: ")
	b.WriteString(formatFloat(targetResult.Target.Confidence))
	b.WriteString("\n")

	b.WriteString("- reason: ")
	if targetResult.Target.Reason == "" {
		b.WriteString("—")
	} else {
		b.WriteString(targetResult.Target.Reason)
	}
	b.WriteString("\n\n")

	b.WriteString(a.sensorCore.ActionSequenceForScene(
		ctx,
		sceneSnapshot,
		requests,
		wantsVision(userMessage),
	))

	return b.String(), true
}

func formatFloat(value float64) string {
	return fmt.Sprintf("%.2f", value)
}

func parseActionRequest(message string) (actions.Request, bool) {
	m := strings.ToLower(strings.TrimSpace(message))

	if strings.Contains(m, "клик") && strings.Contains(m, "entity_") {
		entityID := extractEntityID(message)
		if entityID == "" {
			return actions.Request{}, false
		}

		return actions.Request{
			Kind:     actions.ActionClickEntity,
			EntityID: entityID,
		}, true
	}

	for _, prefix := range []string{"напиши текст:", "введи текст:", "type text:"} {
		if strings.HasPrefix(m, prefix) {
			text := strings.TrimSpace(message[len(prefix):])
			if text == "" {
				return actions.Request{}, false
			}

			return actions.Request{
				Kind: actions.ActionTypeText,
				Text: text,
			}, true
		}
	}

	for _, prefix := range []string{"нажми ", "press "} {
		if strings.HasPrefix(m, prefix) {
			key := strings.TrimSpace(message[len(prefix):])
			if key == "" {
				return actions.Request{}, false
			}

			return actions.Request{
				Kind: actions.ActionPressKey,
				Key:  key,
			}, true
		}
	}

	for _, prefix := range []string{"hotkey ", "хоткей "} {
		if strings.HasPrefix(m, prefix) {
			raw := strings.TrimSpace(message[len(prefix):])
			keys := splitHotkey(raw)
			if len(keys) == 0 {
				return actions.Request{}, false
			}

			return actions.Request{
				Kind: actions.ActionHotkey,
				Keys: keys,
			}, true
		}
	}

	if strings.Contains(m, "скролл вниз") || strings.Contains(m, "прокрути вниз") {
		return actions.Request{
			Kind:      actions.ActionScroll,
			Direction: "down",
			Amount:    4,
		}, true
	}

	if strings.Contains(m, "скролл вверх") || strings.Contains(m, "прокрути вверх") {
		return actions.Request{
			Kind:      actions.ActionScroll,
			Direction: "up",
			Amount:    4,
		}, true
	}

	return actions.Request{}, false
}

func extractEntityID(message string) string {
	fields := strings.Fields(message)

	for _, field := range fields {
		clean := strings.Trim(field, " \t\r\n.,;:()[]{}<>\"'")
		if strings.HasPrefix(clean, "entity_") {
			return clean
		}
	}

	idx := strings.Index(message, "entity_")
	if idx < 0 {
		return ""
	}

	value := message[idx:]

	stop := len(value)
	for i, r := range value {
		if i == 0 {
			continue
		}

		if !(r == '_' || r == '-' || r == '.' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			stop = i
			break
		}
	}

	return value[:stop]
}

func splitHotkey(raw string) []string {
	raw = strings.ReplaceAll(raw, "+", " ")
	raw = strings.ReplaceAll(raw, ",", " ")

	parts := strings.Fields(raw)
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		result = append(result, part)
	}

	return result
}

func isSceneQuestion(message string) bool {
	m := strings.ToLower(strings.TrimSpace(message))

	return strings.Contains(m, "scene") ||
		strings.Contains(m, "сцена") ||
		strings.Contains(m, "собери сцену") ||
		strings.Contains(m, "проанализируй окно") ||
		strings.Contains(m, "проанализируй экран")
}

func isSceneJSONQuestion(message string) bool {
	m := strings.ToLower(strings.TrimSpace(message))

	return strings.Contains(m, "json") &&
		(strings.Contains(m, "scene") ||
			strings.Contains(m, "сцена") ||
			strings.Contains(m, "сцены"))
}

func isVisionSensorQuestion(message string) bool {
	m := strings.ToLower(strings.TrimSpace(message))

	return strings.Contains(m, "vision сенсор") ||
		strings.Contains(m, "сенсор vision") ||
		strings.Contains(m, "визуальный сенсор") ||
		strings.Contains(m, "ии зрение") ||
		strings.Contains(m, "ai vision") ||
		strings.Contains(m, "визуально проанализируй")
}

func isVisionJSONQuestion(message string) bool {
	m := strings.ToLower(strings.TrimSpace(message))

	return strings.Contains(m, "json") &&
		(strings.Contains(m, "vision") ||
			strings.Contains(m, "зрение"))
}

func wantsVision(message string) bool {
	m := strings.ToLower(strings.TrimSpace(message))

	return strings.Contains(m, "vision") ||
		strings.Contains(m, "зрение") ||
		strings.Contains(m, "визуально") ||
		strings.Contains(m, "ии")
}

func isWindowSensorQuestion(message string) bool {
	m := strings.ToLower(strings.TrimSpace(message))

	return strings.Contains(m, "сенсор окон") ||
		strings.Contains(m, "по окнам") ||
		strings.Contains(m, "что открыто") ||
		strings.Contains(m, "активное окно") ||
		strings.Contains(m, "last user window")
}

func isWindowJSONQuestion(message string) bool {
	m := strings.ToLower(strings.TrimSpace(message))

	return strings.Contains(m, "json") &&
		(strings.Contains(m, "окон") ||
			strings.Contains(m, "окна") ||
			strings.Contains(m, "window"))
}

func isScreenSensorQuestion(message string) bool {
	m := strings.ToLower(strings.TrimSpace(message))

	return strings.Contains(m, "сенсор экрана") ||
		strings.Contains(m, "снимок целевого окна") ||
		strings.Contains(m, "скрин целевого окна") ||
		strings.Contains(m, "захвати окно")
}

func isUIScreenSensorQuestion(message string) bool {
	m := strings.ToLower(strings.TrimSpace(message))

	return strings.Contains(m, "ui сенсор") ||
		strings.Contains(m, "сенсор ui") ||
		strings.Contains(m, "сенсор интерфейса") ||
		strings.Contains(m, "интерфейс целевого окна")
}

func isUIJSONQuestion(message string) bool {
	m := strings.ToLower(strings.TrimSpace(message))

	return strings.Contains(m, "json") &&
		(strings.Contains(m, "ui") ||
			strings.Contains(m, "интерфейс"))
}

func isOCRSensorQuestion(message string) bool {
	m := strings.ToLower(strings.TrimSpace(message))

	return strings.Contains(m, "ocr сенсор") ||
		strings.Contains(m, "сенсор ocr") ||
		strings.Contains(m, "распознай текст") ||
		strings.Contains(m, "считай текст") ||
		strings.Contains(m, "прочитай экран")
}

func isOCRJSONQuestion(message string) bool {
	m := strings.ToLower(strings.TrimSpace(message))

	return strings.Contains(m, "json") &&
		(strings.Contains(m, "ocr") ||
			strings.Contains(m, "распознан"))
}
