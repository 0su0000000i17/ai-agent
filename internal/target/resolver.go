package target

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"jarvis/internal/types"
	"jarvis/internal/world"
)

type Model interface {
	Send(messages []types.Message) (string, error)
}

type Resolver struct {
	model Model
}

func NewResolver(model Model) *Resolver {
	return &Resolver{
		model: model,
	}
}

func (r *Resolver) Resolve(ctx context.Context, userGoal string, snapshot world.WindowSnapshot) (Decision, error) {
	if r == nil || r.model == nil {
		return Decision{}, fmt.Errorf("target resolver model is nil")
	}

	system := `Ты Target Resolver внутри Windows AI computer core.

Твоя задача:
- понять, нужно ли для запроса пользователя целевое окно/приложение
- выбрать уже открытое окно, если оно подходит
- если нужного окна нет, вернуть launch_app с универсальным app_query
- если это обычная беседа и действие на ПК не нужно, вернуть none

Ты НЕ выполняешь действие.
Ты НЕ пишешь сценарии под приложения.
Ты НЕ используешь координаты.
Ты только выбираешь target.

Формат ответа строго JSON:

{
  "kind": "none",
  "reason": "почему окно не нужно",
  "confidence": 0.9
}

или:

{
  "kind": "existing_window",
  "hwnd": 123456,
  "reason": "почему это окно подходит",
  "confidence": 0.9
}

или:

{
  "kind": "launch_app",
  "app_query": "notepad",
  "reason": "почему нужно открыть приложение",
  "confidence": 0.8
}

Правила:
- Если пользователь просит сделать что-то в конкретном приложении, выбери это приложение среди открытых окон.
- Если приложение не открыто, верни launch_app.
- app_query должен быть коротким Windows-запросом запуска: например "notepad", "telegram", "chrome", "code".
- Не выбирай JARVIS как target.
- Не выбирай системные/служебные окна: Program Manager, TextInputHost, Переключение задач, Task Switching.
- Если пользователь просто здоровается или задаёт общий вопрос — kind none.
- Если пользователь говорит "в блокноте", "в телеге", "в браузере", "в vscode" — target нужен.
- Если пользователь говорит "напечатай", "введи", "кликни", "прокрути", "открой" — target обычно нужен.
- Верни только JSON. Без markdown.`

	user := fmt.Sprintf(
		"USER_GOAL:\n%s\n\nWINDOWS:\n%s",
		userGoal,
		compactWindows(snapshot),
	)

	raw, err := r.model.Send([]types.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
	if err != nil {
		return Decision{}, err
	}

	return parseDecision(raw)
}

func compactWindows(snapshot world.WindowSnapshot) string {
	var b strings.Builder

	if snapshot.ActiveWindow != nil {
		b.WriteString("active_window:\n")
		b.WriteString(formatWindow(*snapshot.ActiveWindow))
		b.WriteString("\n")
	}

	if snapshot.LastUserWindow != nil {
		b.WriteString("last_user_window:\n")
		b.WriteString(formatWindow(*snapshot.LastUserWindow))
		b.WriteString("\n")
	}

	b.WriteString("visible_windows:\n")

	limit := len(snapshot.VisibleWindows)
	if limit > 60 {
		limit = 60
	}

	for i := 0; i < limit; i++ {
		b.WriteString(formatWindow(snapshot.VisibleWindows[i]))
	}

	if len(snapshot.VisibleWindows) > limit {
		b.WriteString(fmt.Sprintf("- ...and %d more windows\n", len(snapshot.VisibleWindows)-limit))
	}

	return b.String()
}

func formatWindow(w world.WindowRef) string {
	return fmt.Sprintf(
		"- hwnd=%d title=%q process=%q pid=%d rect=%d,%d %dx%d minimized=%v visible=%v\n",
		w.HWND,
		w.Title,
		w.ProcessName,
		w.PID,
		w.X,
		w.Y,
		w.Width,
		w.Height,
		w.Minimized,
		w.Visible,
	)
}

func parseDecision(raw string) (Decision, error) {
	clean := extractJSON(raw)

	var decision Decision
	if err := json.Unmarshal([]byte(clean), &decision); err != nil {
		return Decision{}, fmt.Errorf("cannot parse target decision: %w. raw=%s", err, raw)
	}

	if decision.Kind == "" {
		decision.Kind = DecisionNone
	}

	return decision, nil
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
