package actions

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"jarvis/internal/scene"
	"jarvis/internal/world"
)

type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) Execute(ctx context.Context, currentScene scene.Scene, req Request) Result {
	startedAt := time.Now()

	switch req.Kind {
	case ActionFocusWindow:
		err := focusWindow(ctx, currentScene.Window.HWND)
		if err != nil {
			return Failure(req.Kind, currentScene.Window.Title, err, startedAt)
		}

		return Success(req.Kind, currentScene.Window.Title, "window focused", startedAt)

	case ActionClickEntity:
		entity, ok := findEntity(currentScene, req.EntityID)
		if !ok {
			return Failure(req.Kind, req.EntityID, fmt.Errorf("entity not found: %s", req.EntityID), startedAt)
		}

		x, y := entityCenterToScreen(currentScene.Window, entity)

		if err := focusWindow(ctx, currentScene.Window.HWND); err != nil {
			return Failure(req.Kind, req.EntityID, err, startedAt)
		}

		time.Sleep(180 * time.Millisecond)

		if err := clickAt(ctx, x, y); err != nil {
			return Failure(req.Kind, req.EntityID, err, startedAt)
		}

		return Success(
			req.Kind,
			req.EntityID,
			fmt.Sprintf("clicked entity %s at %d,%d", req.EntityID, x, y),
			startedAt,
		)

	case ActionClickPoint:
		if err := focusWindow(ctx, currentScene.Window.HWND); err != nil {
			return Failure(req.Kind, currentScene.Window.Title, err, startedAt)
		}

		time.Sleep(180 * time.Millisecond)

		if err := clickAt(ctx, req.X, req.Y); err != nil {
			return Failure(req.Kind, fmt.Sprintf("%d,%d", req.X, req.Y), err, startedAt)
		}

		return Success(req.Kind, fmt.Sprintf("%d,%d", req.X, req.Y), "clicked point", startedAt)

	case ActionTypeText:
		if strings.TrimSpace(req.Text) == "" {
			return Failure(req.Kind, "", fmt.Errorf("empty text"), startedAt)
		}

		if err := focusWindow(ctx, currentScene.Window.HWND); err != nil {
			return Failure(req.Kind, currentScene.Window.Title, err, startedAt)
		}

		time.Sleep(180 * time.Millisecond)

		targetDescription := "current focused typing target"

		if !req.SkipTargetClick {
			// Normal insert mode:
			// find and click best typing target.
			x, y, desc := bestTypingPoint(currentScene)
			targetDescription = desc

			if err := clickAt(ctx, x, y); err != nil {
				return Failure(req.Kind, targetDescription, err, startedAt)
			}

			time.Sleep(180 * time.Millisecond)
		}

		if err := pasteText(ctx, req.Text); err != nil {
			return Failure(req.Kind, targetDescription, err, startedAt)
		}

		if req.SkipTargetClick {
			return Success(req.Kind, targetDescription, "text pasted without target click", startedAt)
		}

		return Success(req.Kind, targetDescription, "text pasted into focused typing target", startedAt)

	case ActionPressKey:
		if strings.TrimSpace(req.Key) == "" {
			return Failure(req.Kind, "", fmt.Errorf("empty key"), startedAt)
		}

		if err := focusWindow(ctx, currentScene.Window.HWND); err != nil {
			return Failure(req.Kind, currentScene.Window.Title, err, startedAt)
		}

		time.Sleep(140 * time.Millisecond)

		if err := sendKey(ctx, req.Key); err != nil {
			return Failure(req.Kind, req.Key, err, startedAt)
		}

		return Success(req.Kind, req.Key, "key pressed", startedAt)

	case ActionHotkey:
		if len(req.Keys) == 0 {
			return Failure(req.Kind, "", fmt.Errorf("empty hotkey"), startedAt)
		}

		if err := focusWindow(ctx, currentScene.Window.HWND); err != nil {
			return Failure(req.Kind, currentScene.Window.Title, err, startedAt)
		}

		time.Sleep(140 * time.Millisecond)

		if err := sendHotkey(ctx, req.Keys); err != nil {
			return Failure(req.Kind, strings.Join(req.Keys, "+"), err, startedAt)
		}

		return Success(req.Kind, strings.Join(req.Keys, "+"), "hotkey pressed", startedAt)

	case ActionScroll:
		amount := req.Amount
		if amount <= 0 {
			amount = 3
		}

		if err := focusWindow(ctx, currentScene.Window.HWND); err != nil {
			return Failure(req.Kind, currentScene.Window.Title, err, startedAt)
		}

		time.Sleep(140 * time.Millisecond)

		x := currentScene.Window.X + currentScene.Window.Width/2
		y := currentScene.Window.Y + currentScene.Window.Height/2

		if err := moveCursor(ctx, x, y); err != nil {
			return Failure(req.Kind, currentScene.Window.Title, err, startedAt)
		}

		if err := scroll(ctx, req.Direction, amount); err != nil {
			return Failure(req.Kind, req.Direction, err, startedAt)
		}

		return Success(req.Kind, req.Direction, "scrolled", startedAt)

	case ActionWait:
		waitMS := req.WaitMS
		if waitMS <= 0 {
			waitMS = 700
		}

		time.Sleep(time.Duration(waitMS) * time.Millisecond)

		return Success(req.Kind, strconv.Itoa(waitMS), "waited", startedAt)

	default:
		return Failure(req.Kind, "", fmt.Errorf("unknown action: %s", req.Kind), startedAt)
	}
}

func findEntity(s scene.Scene, entityID string) (scene.SceneEntity, bool) {
	entityID = strings.TrimSpace(entityID)
	if entityID == "" {
		return scene.SceneEntity{}, false
	}

	for _, entity := range s.Entities {
		if entity.ID == entityID {
			return entity, true
		}
	}

	return scene.SceneEntity{}, false
}

func entityCenterToScreen(window world.WindowRef, entity scene.SceneEntity) (int, int) {
	x := entity.X + entity.Width/2
	y := entity.Y + entity.Height/2

	// UI Automation usually gives absolute screen coordinates.
	// OCR and Vision coordinates are relative to the captured target window image.
	if hasSource(entity, "ocr") || hasSource(entity, "vision") {
		x += float64(window.X)
		y += float64(window.Y)
	}

	return int(x), int(y)
}

func hasSource(entity scene.SceneEntity, source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))

	for _, item := range entity.Source {
		if strings.ToLower(strings.TrimSpace(item)) == source {
			return true
		}
	}

	return false
}

func bestTypingPoint(s scene.Scene) (int, int, string) {
	// 1. Prefer semantic scene entities marked as input/search/message_area.
	for _, entity := range s.Entities {
		kind := strings.ToLower(strings.TrimSpace(entity.Kind))
		label := strings.ToLower(strings.TrimSpace(entity.Label))

		if kind == "input" ||
			kind == "search" ||
			kind == "message_area" ||
			strings.Contains(label, "write a message") ||
			strings.Contains(label, "search") ||
			strings.Contains(label, "найти") ||
			strings.Contains(label, "поиск") {

			x, y := entityCenterToScreen(s.Window, entity)
			return x, y, "scene entity " + entity.ID
		}
	}

	// 2. Prefer UIA edit/document/text editor controls.
	if x, y, ok, desc := bestTypingPointFromUI(s); ok {
		return x, y, desc
	}

	// 3. Safe fallback: click inside the window content area.
	// Not ideal, but universal enough for Notepad and many editors.
	x := s.Window.X + s.Window.Width/2
	y := s.Window.Y + s.Window.Height/2

	return x, y, "window center fallback"
}

func bestTypingPointFromUI(s scene.Scene) (int, int, bool, string) {
	bestScore := -1.0
	bestX := 0
	bestY := 0
	bestDesc := ""

	for _, item := range s.UIElements {
		controlType := strings.ToLower(strings.TrimSpace(item.ControlType))
		name := strings.ToLower(strings.TrimSpace(item.Name))
		className := strings.ToLower(strings.TrimSpace(item.ClassName))
		automationID := strings.ToLower(strings.TrimSpace(item.AutomationID))

		if item.Width <= 0 || item.Height <= 0 {
			continue
		}

		score := 0.0

		if strings.Contains(controlType, "edit") {
			score += 100
		}

		if strings.Contains(controlType, "document") {
			score += 95
		}

		if strings.Contains(controlType, "text") {
			score += 35
		}

		if strings.Contains(className, "edit") ||
			strings.Contains(className, "richedit") ||
			strings.Contains(className, "text") {
			score += 60
		}

		if strings.Contains(name, "text editor") ||
			strings.Contains(name, "editor") ||
			strings.Contains(name, "document") ||
			strings.Contains(name, "текст") ||
			strings.Contains(name, "документ") {
			score += 50
		}

		if strings.Contains(name, "search") ||
			strings.Contains(name, "поиск") ||
			strings.Contains(automationID, "search") {
			score += 80
		}

		area := item.Width * item.Height

		// Big editable areas are likely text documents/editors.
		if area > 100000 {
			score += 20
		}

		// Avoid tiny UI chrome.
		if item.Width < 40 || item.Height < 15 {
			score -= 40
		}

		if score > bestScore {
			bestScore = score
			bestX = int(item.X + item.Width/2)
			bestY = int(item.Y + item.Height/2)
			bestDesc = fmt.Sprintf(
				"ui target %s type=%s name=%s class=%s",
				item.ID,
				item.ControlType,
				item.Name,
				item.ClassName,
			)
		}
	}

	if bestScore < 40 {
		return 0, 0, false, ""
	}

	return bestX, bestY, true, bestDesc
}

func focusWindow(ctx context.Context, hwnd uintptr) error {
	if hwnd == 0 {
		return fmt.Errorf("invalid hwnd")
	}

	script := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;

public class WinAPI {
	[DllImport("user32.dll")]
	public static extern bool SetForegroundWindow(IntPtr hWnd);

	[DllImport("user32.dll")]
	public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);

	[DllImport("user32.dll")]
	public static extern bool IsIconic(IntPtr hWnd);

	[DllImport("user32.dll")]
	public static extern bool BringWindowToTop(IntPtr hWnd);

	[DllImport("user32.dll")]
	public static extern IntPtr SetFocus(IntPtr hWnd);
}
"@

$h = [IntPtr]%d

if ([WinAPI]::IsIconic($h)) {
	[WinAPI]::ShowWindow($h, 9) | Out-Null
	Start-Sleep -Milliseconds 120
}

[WinAPI]::ShowWindow($h, 5) | Out-Null
[WinAPI]::BringWindowToTop($h) | Out-Null
[WinAPI]::SetForegroundWindow($h) | Out-Null
[WinAPI]::SetFocus($h) | Out-Null
`, hwnd)

	return runPowerShell(ctx, script)
}

func clickAt(ctx context.Context, x int, y int) error {
	script := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;

public class MouseAPI {
	[DllImport("user32.dll")]
	public static extern bool SetCursorPos(int X, int Y);

	[DllImport("user32.dll")]
	public static extern void mouse_event(uint dwFlags, uint dx, uint dy, uint dwData, UIntPtr dwExtraInfo);
}
"@

[MouseAPI]::SetCursorPos(%d, %d) | Out-Null
Start-Sleep -Milliseconds 80
[MouseAPI]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero)
Start-Sleep -Milliseconds 60
[MouseAPI]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero)
`, x, y)

	return runPowerShell(ctx, script)
}

func moveCursor(ctx context.Context, x int, y int) error {
	script := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;

public class MouseAPI {
	[DllImport("user32.dll")]
	public static extern bool SetCursorPos(int X, int Y);
}
"@

[MouseAPI]::SetCursorPos(%d, %d) | Out-Null
`, x, y)

	return runPowerShell(ctx, script)
}

func pasteText(ctx context.Context, text string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))

	script := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;

public class KeyboardAPI {
	[DllImport("user32.dll")]
	public static extern void keybd_event(byte bVk, byte bScan, uint dwFlags, UIntPtr dwExtraInfo);
}
"@

$bytes = [System.Convert]::FromBase64String('%s')
$text = [System.Text.Encoding]::UTF8.GetString($bytes)

Set-Clipboard -Value $text

Start-Sleep -Milliseconds 180

# CTRL down
[KeyboardAPI]::keybd_event(0x11, 0, 0, [UIntPtr]::Zero)
Start-Sleep -Milliseconds 30

# V down/up
[KeyboardAPI]::keybd_event(0x56, 0, 0, [UIntPtr]::Zero)
Start-Sleep -Milliseconds 30
[KeyboardAPI]::keybd_event(0x56, 0, 0x0002, [UIntPtr]::Zero)

Start-Sleep -Milliseconds 30

# CTRL up
[KeyboardAPI]::keybd_event(0x11, 0, 0x0002, [UIntPtr]::Zero)
`, encoded)

	return runPowerShell(ctx, script)
}

func sendKey(ctx context.Context, key string) error {
	sendKeys := keyToSendKeys(key)
	if sendKeys == "" {
		return fmt.Errorf("unsupported key: %s", key)
	}

	script := fmt.Sprintf(`
$wshell = New-Object -ComObject WScript.Shell
$wshell.SendKeys('%s')
`, escapeSendKeys(sendKeys))

	return runPowerShell(ctx, script)
}

func sendHotkey(ctx context.Context, keys []string) error {
	sendKeys := hotkeyToSendKeys(keys)
	if sendKeys == "" {
		return fmt.Errorf("unsupported hotkey: %s", strings.Join(keys, "+"))
	}

	script := fmt.Sprintf(`
$wshell = New-Object -ComObject WScript.Shell
$wshell.SendKeys('%s')
`, escapeSendKeys(sendKeys))

	return runPowerShell(ctx, script)
}

func scroll(ctx context.Context, direction string, amount int) error {
	direction = strings.ToLower(strings.TrimSpace(direction))

	delta := 120 * amount
	if direction == "down" || direction == "вниз" {
		delta = -delta
	}

	script := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;

public class MouseAPI {
	[DllImport("user32.dll")]
	public static extern void mouse_event(uint dwFlags, uint dx, uint dy, int dwData, UIntPtr dwExtraInfo);
}
"@

[MouseAPI]::mouse_event(0x0800, 0, 0, %d, [UIntPtr]::Zero)
`, delta)

	return runPowerShell(ctx, script)
}

func keyToSendKeys(key string) string {
	k := strings.ToLower(strings.TrimSpace(key))

	switch k {
	case "enter", "return", "ввод", "энтер":
		return "{ENTER}"
	case "tab", "таб":
		return "{TAB}"
	case "esc", "escape":
		return "{ESC}"
	case "backspace":
		return "{BACKSPACE}"
	case "delete", "del":
		return "{DELETE}"
	case "space", "пробел":
		return " "
	case "up", "вверх":
		return "{UP}"
	case "down", "вниз":
		return "{DOWN}"
	case "left", "влево":
		return "{LEFT}"
	case "right", "вправо":
		return "{RIGHT}"
	case "home":
		return "{HOME}"
	case "end":
		return "{END}"
	case "pageup":
		return "{PGUP}"
	case "pagedown":
		return "{PGDN}"
	}

	if len([]rune(k)) == 1 {
		return k
	}

	return ""
}

func hotkeyToSendKeys(keys []string) string {
	if len(keys) == 0 {
		return ""
	}

	var modifiers strings.Builder
	normalKeys := make([]string, 0)

	for _, key := range keys {
		k := strings.ToLower(strings.TrimSpace(key))

		switch k {
		case "ctrl", "control", "контрол":
			modifiers.WriteString("^")
		case "alt":
			modifiers.WriteString("%")
		case "shift":
			modifiers.WriteString("+")
		default:
			normalKeys = append(normalKeys, k)
		}
	}

	if len(normalKeys) == 0 {
		return ""
	}

	main := keyToSendKeys(normalKeys[len(normalKeys)-1])
	if main == "" {
		main = normalKeys[len(normalKeys)-1]
	}

	return modifiers.String() + main
}

func escapeSendKeys(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func runPowerShell(ctx context.Context, script string) error {
	cmd := exec.CommandContext(
		ctx,
		"powershell",
		"-STA",
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-Command",
		script,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}

	return nil
}
