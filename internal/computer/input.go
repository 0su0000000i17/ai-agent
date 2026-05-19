package computer

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type InputEngine struct {
	state *RuntimeState
}

func NewInputEngine(state *RuntimeState) *InputEngine {
	return &InputEngine{state: state}
}

func (i *InputEngine) Click(ctx context.Context, x int, y int) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("click is currently implemented only for Windows")
	}

	script := fmt.Sprintf(`
Add-Type @"
using System;
using System.Runtime.InteropServices;

public class MouseInput {
	[DllImport("user32.dll")]
	public static extern bool SetCursorPos(int X, int Y);

	[DllImport("user32.dll")]
	public static extern void mouse_event(uint dwFlags, uint dx, uint dy, uint dwData, UIntPtr dwExtraInfo);
}
"@

[MouseInput]::SetCursorPos(%d, %d) | Out-Null
Start-Sleep -Milliseconds 80
[MouseInput]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero)
Start-Sleep -Milliseconds 60
[MouseInput]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero)
`, x, y)

	err := runPowerShellInput(ctx, script)

	if i.state != nil {
		i.state.SetAction("click", fmt.Sprintf("%d,%d", x, y), errorToResult(err))
	}

	return err
}

func (i *InputEngine) TypeText(ctx context.Context, text string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("type text is currently implemented only for Windows")
	}

	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.Clipboard]::SetText('%s')
[System.Windows.Forms.SendKeys]::SendWait('^v')
`, escapePowerShellSingleQuoted(text))

	err := runPowerShellSTA(ctx, script)

	if i.state != nil {
		i.state.SetAction("type", text, errorToResult(err))
	}

	return err
}

func (i *InputEngine) Press(ctx context.Context, key string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("press key is currently implemented only for Windows")
	}

	normalized := normalizeKey(key)
	if normalized == "" {
		return fmt.Errorf("unknown key: %s", key)
	}

	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.SendKeys]::SendWait('%s')
`, normalized)

	err := runPowerShellSTA(ctx, script)

	if i.state != nil {
		i.state.SetAction("press", key, errorToResult(err))
	}

	return err
}

func (i *InputEngine) Hotkey(ctx context.Context, keys []string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("hotkey is currently implemented only for Windows")
	}

	sequence, err := sendKeysHotkey(keys)
	if err != nil {
		return err
	}

	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
[System.Windows.Forms.SendKeys]::SendWait('%s')
`, sequence)

	err = runPowerShellSTA(ctx, script)

	if i.state != nil {
		i.state.SetAction("hotkey", strings.Join(keys, "+"), errorToResult(err))
	}

	return err
}

func (i *InputEngine) OpenViaLauncher(ctx context.Context, query string) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("empty launcher query")
	}

	if runtime.GOOS != "windows" {
		return fmt.Errorf("launcher open is currently implemented only for Windows")
	}

	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms

Add-Type @"
using System;
using System.Runtime.InteropServices;
public class KeyboardInput {
	[DllImport("user32.dll")]
	public static extern void keybd_event(byte bVk, byte bScan, uint dwFlags, UIntPtr dwExtraInfo);
}
"@

# Win key down/up
[KeyboardInput]::keybd_event(0x5B, 0, 0, [UIntPtr]::Zero)
Start-Sleep -Milliseconds 80
[KeyboardInput]::keybd_event(0x5B, 0, 2, [UIntPtr]::Zero)
Start-Sleep -Milliseconds 450

[System.Windows.Forms.Clipboard]::SetText('%s')
[System.Windows.Forms.SendKeys]::SendWait('^v')
Start-Sleep -Milliseconds 350
[System.Windows.Forms.SendKeys]::SendWait('{ENTER}')
`, escapePowerShellSingleQuoted(query))

	err := runPowerShellSTA(ctx, script)

	if i.state != nil {
		i.state.SetAction("open_launcher", query, errorToResult(err))
	}

	time.Sleep(1200 * time.Millisecond)

	return err
}

func runPowerShellInput(ctx context.Context, script string) error {
	cmd := exec.CommandContext(
		ctx,
		"powershell",
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

func runPowerShellSTA(ctx context.Context, script string) error {
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

func escapePowerShellSingleQuoted(text string) string {
	return strings.ReplaceAll(text, "'", "''")
}

func normalizeKey(key string) string {
	k := strings.ToLower(strings.TrimSpace(key))

	switch k {
	case "enter", "return":
		return "{ENTER}"
	case "esc", "escape":
		return "{ESC}"
	case "tab":
		return "{TAB}"
	case "space":
		return " "
	case "backspace":
		return "{BACKSPACE}"
	case "delete", "del":
		return "{DELETE}"
	case "up":
		return "{UP}"
	case "down":
		return "{DOWN}"
	case "left":
		return "{LEFT}"
	case "right":
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

	if strings.HasPrefix(k, "f") {
		nRaw := strings.TrimPrefix(k, "f")
		n, err := strconv.Atoi(nRaw)
		if err == nil && n >= 1 && n <= 24 {
			return "{F" + strconv.Itoa(n) + "}"
		}
	}

	if len(k) == 1 {
		return k
	}

	return ""
}

func sendKeysHotkey(keys []string) (string, error) {
	if len(keys) == 0 {
		return "", fmt.Errorf("empty hotkey")
	}

	modifiers := ""
	regular := ""

	for _, raw := range keys {
		k := strings.ToLower(strings.TrimSpace(raw))

		switch k {
		case "ctrl", "control":
			modifiers += "^"
		case "shift":
			modifiers += "+"
		case "alt":
			modifiers += "%"
		default:
			regular = normalizeKey(k)
			if regular == "" {
				return "", fmt.Errorf("unknown hotkey key: %s", raw)
			}
		}
	}

	if regular == "" {
		return "", fmt.Errorf("hotkey has no regular key")
	}

	return modifiers + regular, nil
}
