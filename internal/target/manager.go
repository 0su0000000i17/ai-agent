package target

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	windowsensor "jarvis/internal/sensors/window"
	"jarvis/internal/world"
)

type Manager struct {
	world        *world.WorldState
	resolver     *Resolver
	windowSensor *windowsensor.Sensor
}

func NewManager(worldState *world.WorldState, resolver *Resolver, windowSensor *windowsensor.Sensor) *Manager {
	return &Manager{
		world:        worldState,
		resolver:     resolver,
		windowSensor: windowSensor,
	}
}

func (m *Manager) ResolveForGoal(ctx context.Context, userGoal string) (ResolveResult, error) {
	if m == nil || m.world == nil || m.resolver == nil {
		return ResolveResult{}, fmt.Errorf("target manager is not initialized")
	}

	snapshot := m.world.Snapshot()

	decision, err := m.resolver.Resolve(ctx, userGoal, snapshot)
	if err != nil {
		return ResolveResult{}, err
	}

	switch decision.Kind {
	case DecisionNone:
		return ResolveResult{
			HasTarget: false,
			Message:   decision.Reason,
		}, nil

	case DecisionExistingWindow:
		window, ok := findWindowByHWND(snapshot, decision.HWND)
		if !ok {
			return ResolveResult{}, fmt.Errorf("target hwnd not found: %d", decision.HWND)
		}

		if !isValidUserTarget(window) {
			return ResolveResult{}, fmt.Errorf("target window is not valid user target: %s [%s]", window.Title, window.ProcessName)
		}

		return ResolveResult{
			HasTarget: true,
			Target: ResolvedTarget{
				Window:     window,
				Reason:     decision.Reason,
				Confidence: decision.Confidence,
				Launched:   false,
			},
		}, nil

	case DecisionLaunchApp:
		appQuery := strings.TrimSpace(decision.AppQuery)
		if appQuery == "" {
			return ResolveResult{}, fmt.Errorf("launch_app requires app_query")
		}

		before := snapshot

		if err := launchAppQuery(ctx, appQuery); err != nil {
			return ResolveResult{}, err
		}

		resolved, err := m.waitForTargetAfterLaunch(ctx, userGoal, before, decision)
		if err != nil {
			return ResolveResult{}, err
		}

		return ResolveResult{
			HasTarget: true,
			Target:    resolved,
		}, nil

	default:
		return ResolveResult{}, fmt.Errorf("unknown target decision kind: %s", decision.Kind)
	}
}

func (m *Manager) waitForTargetAfterLaunch(
	ctx context.Context,
	userGoal string,
	before world.WindowSnapshot,
	launchDecision Decision,
) (ResolvedTarget, error) {
	deadline := time.Now().Add(8 * time.Second)

	var previous = before

	for time.Now().Before(deadline) {
		time.Sleep(700 * time.Millisecond)

		snapshot, err := m.windowSensor.Observe(ctx, previous)
		if err == nil {
			previous = snapshot
		} else {
			snapshot = m.world.Snapshot()
		}

		decision, err := m.resolver.Resolve(ctx, userGoal, snapshot)
		if err != nil {
			continue
		}

		if decision.Kind != DecisionExistingWindow {
			continue
		}

		window, ok := findWindowByHWND(snapshot, decision.HWND)
		if !ok {
			continue
		}

		if !isValidUserTarget(window) {
			continue
		}

		return ResolvedTarget{
			Window:     window,
			Reason:     decision.Reason,
			Confidence: decision.Confidence,
			Launched:   true,
		}, nil
	}

	return ResolvedTarget{}, fmt.Errorf("launched app but could not resolve target window: %s", launchDecision.AppQuery)
}

func findWindowByHWND(snapshot world.WindowSnapshot, hwnd uintptr) (world.WindowRef, bool) {
	if snapshot.ActiveWindow != nil && snapshot.ActiveWindow.HWND == hwnd {
		return *snapshot.ActiveWindow, true
	}

	if snapshot.LastUserWindow != nil && snapshot.LastUserWindow.HWND == hwnd {
		return *snapshot.LastUserWindow, true
	}

	for _, window := range snapshot.VisibleWindows {
		if window.HWND == hwnd {
			return window, true
		}
	}

	return world.WindowRef{}, false
}

func isValidUserTarget(w world.WindowRef) bool {
	title := strings.ToLower(strings.TrimSpace(w.Title))
	process := strings.ToLower(strings.TrimSpace(w.ProcessName))

	if w.HWND == 0 || w.PID <= 0 {
		return false
	}

	if !w.Visible || w.Minimized {
		return false
	}

	if w.Width < 120 || w.Height < 80 {
		return false
	}

	blockedProcesses := []string{
		"jarvis",
		"textinputhost.exe",
		"applicationframehost.exe",
	}

	for _, blocked := range blockedProcesses {
		if strings.Contains(process, blocked) {
			return false
		}
	}

	blockedTitles := []string{
		"jarvis",
		"program manager",
		"переключение задач",
		"task switching",
		"интерфейс ввода windows",
	}

	for _, blocked := range blockedTitles {
		if strings.Contains(title, blocked) {
			return false
		}
	}

	if title == "" {
		return false
	}

	return true
}

func launchAppQuery(ctx context.Context, appQuery string) error {
	appQuery = strings.TrimSpace(appQuery)
	if appQuery == "" {
		return fmt.Errorf("empty app query")
	}

	// Generic Windows launch. Not app-specific.
	// For known executable aliases like notepad, code, chrome, telegram this works directly.
	script := fmt.Sprintf(`
$ErrorActionPreference = "SilentlyContinue"

$query = '%s'

try {
	Start-Process -FilePath $query
	exit 0
} catch {}

try {
	Start-Process $query
	exit 0
} catch {}

# Fallback through Start Menu search.
Add-Type @"
using System;
using System.Runtime.InteropServices;

public class KeyboardAPI {
	[DllImport("user32.dll")]
	public static extern void keybd_event(byte bVk, byte bScan, uint dwFlags, UIntPtr dwExtraInfo);
}
"@

Set-Clipboard -Value $query
Start-Sleep -Milliseconds 100

# WIN key
[KeyboardAPI]::keybd_event(0x5B, 0, 0, [UIntPtr]::Zero)
Start-Sleep -Milliseconds 40
[KeyboardAPI]::keybd_event(0x5B, 0, 0x0002, [UIntPtr]::Zero)

Start-Sleep -Milliseconds 350

$wshell = New-Object -ComObject WScript.Shell
$wshell.SendKeys('^v')
Start-Sleep -Milliseconds 250
$wshell.SendKeys('{ENTER}')
`, escapePowerShellSingleQuoted(appQuery))

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

func escapePowerShellSingleQuoted(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
