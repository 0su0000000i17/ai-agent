package computer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type FocusEngine struct {
	state *RuntimeState
}

func NewFocusEngine(state *RuntimeState) *FocusEngine {
	return &FocusEngine{state: state}
}

func (f *FocusEngine) IsSelfActive(obs Observation) bool {
	if obs.ActiveWindow == nil {
		return false
	}

	return f.isSelfWindow(*obs.ActiveWindow)
}

func (f *FocusEngine) ActivateBestUserWindow(ctx context.Context, obs Observation) (*WindowInfo, error) {
	target := f.bestUserWindow(obs)
	if target == nil {
		return nil, fmt.Errorf("no suitable user window found")
	}

	if err := f.ActivateWindow(ctx, target.PID); err != nil {
		return nil, err
	}

	if f.state != nil {
		f.state.SetAction("activate_user_window", target.ProcessName, "ok")
	}

	time.Sleep(450 * time.Millisecond)

	return target, nil
}

func (f *FocusEngine) ActivateWindow(ctx context.Context, pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid")
	}

	if runtime.GOOS != "windows" {
		return fmt.Errorf("activate window is currently implemented only for Windows")
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
}
"@

$p = Get-Process -Id %d -ErrorAction SilentlyContinue

if ($p -eq $null) {
	throw "process not found"
}

$h = $p.MainWindowHandle

if ($h -eq 0) {
	throw "process has no main window handle"
}

if ([WinAPI]::IsIconic($h)) {
	[WinAPI]::ShowWindow($h, 9) | Out-Null
	Start-Sleep -Milliseconds 120
}

[WinAPI]::SetForegroundWindow($h) | Out-Null
`, pid)

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

func (f *FocusEngine) bestUserWindow(obs Observation) *WindowInfo {
	for _, window := range obs.Windows {
		if strings.TrimSpace(window.Title) == "" {
			continue
		}

		if window.PID <= 0 {
			continue
		}

		if f.isSelfWindow(window) {
			continue
		}

		copy := window
		return &copy
	}

	return nil
}

func (f *FocusEngine) isSelfWindow(window WindowInfo) bool {
	selfPID := os.Getpid()

	if window.PID == selfPID {
		return true
	}

	selfExe := ""
	if exe, err := os.Executable(); err == nil {
		selfExe = strings.ToLower(filepath.Base(exe))
	}

	process := strings.ToLower(strings.TrimSpace(window.ProcessName))

	if selfExe != "" && process == selfExe {
		return true
	}

	return false
}
