package window

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"jarvis/internal/world"
)

func NewSensor(opts Options) *Sensor {
	selfPID := opts.SelfPID
	if selfPID <= 0 {
		selfPID = os.Getpid()
	}

	return &Sensor{
		selfPID: selfPID,
		state:   opts.State,
	}
}

func (s *Sensor) Observe(ctx context.Context, previous world.WindowSnapshot) (world.WindowSnapshot, error) {
	processes, err := s.readProcesses(ctx)
	if err != nil {
		return world.WindowSnapshot{}, err
	}

	windows, err := s.readWindows(ctx)
	if err != nil {
		return world.WindowSnapshot{}, err
	}

	active, err := s.readActiveWindow(ctx)
	if err != nil {
		active = nil
	}

	now := time.Now()

	lastUserWindow := previous.LastUserWindow

	if active != nil && !s.isSelfWindow(*active) {
		copy := *active
		copy.UpdatedAt = now
		lastUserWindow = &copy
	}

	for i := range windows {
		windows[i].UpdatedAt = now
	}

	for i := range processes {
		processes[i].UpdatedAt = now
	}

	snapshot := world.WindowSnapshot{
		ActiveWindow:   active,
		LastUserWindow: lastUserWindow,
		VisibleWindows: windows,
		Processes:      processes,
		ObservedAt:     now,
	}

	if s.state != nil {
		s.state.SetWindowSnapshot(snapshot)
	}

	return snapshot, nil
}

func (s *Sensor) readProcesses(ctx context.Context) ([]world.ProcessRef, error) {
	out, err := exec.CommandContext(ctx, "tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(bytes.NewReader(out))
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	result := make([]world.ProcessRef, 0, len(records))

	for _, record := range records {
		if len(record) < 2 {
			continue
		}

		name := strings.TrimSpace(record[0])
		pidRaw := strings.TrimSpace(record[1])

		pid, _ := strconv.Atoi(pidRaw)

		if name == "" {
			continue
		}

		result = append(result, world.ProcessRef{
			PID:  pid,
			Name: name,
		})
	}

	return result, nil
}

type rawWindow struct {
	HWND        uint64 `json:"hwnd"`
	Title       string `json:"title"`
	ProcessName string `json:"process_name"`
	PID         int    `json:"pid"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Visible     bool   `json:"visible"`
	Minimized   bool   `json:"minimized"`
}

func (s *Sensor) readWindows(ctx context.Context) ([]world.WindowRef, error) {
	script := `
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)

Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
using System.Collections.Generic;

public class WinEnum {
    public delegate bool EnumWindowsProc(IntPtr hWnd, IntPtr lParam);

    [DllImport("user32.dll")]
    public static extern bool EnumWindows(EnumWindowsProc lpEnumFunc, IntPtr lParam);

    [DllImport("user32.dll")]
    public static extern bool IsWindowVisible(IntPtr hWnd);

    [DllImport("user32.dll")]
    public static extern bool IsIconic(IntPtr hWnd);

    [DllImport("user32.dll")]
    public static extern int GetWindowText(IntPtr hWnd, StringBuilder text, int count);

    [DllImport("user32.dll")]
    public static extern int GetWindowTextLength(IntPtr hWnd);

    [DllImport("user32.dll")]
    public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint processId);

    [DllImport("user32.dll")]
    public static extern bool GetWindowRect(IntPtr hWnd, out RECT rect);

    [StructLayout(LayoutKind.Sequential)]
    public struct RECT {
        public int Left;
        public int Top;
        public int Right;
        public int Bottom;
    }
}
"@

$items = New-Object System.Collections.ArrayList

$callback = {
    param($hWnd, $lParam)

    if (-not [WinEnum]::IsWindowVisible($hWnd)) {
        return $true
    }

    $len = [WinEnum]::GetWindowTextLength($hWnd)
    if ($len -le 0) {
        return $true
    }

    $sb = New-Object System.Text.StringBuilder ($len + 1)
    [WinEnum]::GetWindowText($hWnd, $sb, $sb.Capacity) | Out-Null
    $title = $sb.ToString()

    if ([string]::IsNullOrWhiteSpace($title)) {
        return $true
    }

    $pidValue = 0
    [WinEnum]::GetWindowThreadProcessId($hWnd, [ref]$pidValue) | Out-Null

    $proc = Get-Process -Id $pidValue -ErrorAction SilentlyContinue
    if ($proc -eq $null) {
        return $true
    }

    $rect = New-Object WinEnum+RECT
    [WinEnum]::GetWindowRect($hWnd, [ref]$rect) | Out-Null

    $width = $rect.Right - $rect.Left
    $height = $rect.Bottom - $rect.Top

    if ($width -le 0 -or $height -le 0) {
        return $true
    }

    [void]$items.Add([ordered]@{
        hwnd = $hWnd.ToInt64()
        title = $title
        process_name = $proc.ProcessName + ".exe"
        pid = [int]$pidValue
        x = $rect.Left
        y = $rect.Top
        width = $width
        height = $height
        visible = $true
        minimized = [WinEnum]::IsIconic($hWnd)
    })

    return $true
}

[WinEnum]::EnumWindows($callback, [IntPtr]::Zero) | Out-Null

$items | ConvertTo-Json -Depth 5 -Compress
`

	out, err := exec.CommandContext(
		ctx,
		"powershell",
		"-STA",
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-Command",
		script,
	).CombinedOutput()

	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, string(out))
	}

	clean := strings.TrimSpace(string(out))
	if clean == "" {
		return []world.WindowRef{}, nil
	}

	var many []rawWindow
	if err := json.Unmarshal([]byte(clean), &many); err == nil {
		return convertRawWindows(many), nil
	}

	var one rawWindow
	if err := json.Unmarshal([]byte(clean), &one); err != nil {
		return nil, fmt.Errorf("cannot parse windows json: %w. raw=%s", err, clean)
	}

	return convertRawWindows([]rawWindow{one}), nil
}

func (s *Sensor) readActiveWindow(ctx context.Context) (*world.WindowRef, error) {
	script := `
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)

Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;

public class WinActive {
    [DllImport("user32.dll")]
    public static extern IntPtr GetForegroundWindow();

    [DllImport("user32.dll")]
    public static extern bool IsWindowVisible(IntPtr hWnd);

    [DllImport("user32.dll")]
    public static extern bool IsIconic(IntPtr hWnd);

    [DllImport("user32.dll")]
    public static extern int GetWindowText(IntPtr hWnd, StringBuilder text, int count);

    [DllImport("user32.dll")]
    public static extern int GetWindowTextLength(IntPtr hWnd);

    [DllImport("user32.dll")]
    public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint processId);

    [DllImport("user32.dll")]
    public static extern bool GetWindowRect(IntPtr hWnd, out RECT rect);

    [StructLayout(LayoutKind.Sequential)]
    public struct RECT {
        public int Left;
        public int Top;
        public int Right;
        public int Bottom;
    }
}
"@

$hWnd = [WinActive]::GetForegroundWindow()

if ($hWnd -eq [IntPtr]::Zero) {
    exit
}

$len = [WinActive]::GetWindowTextLength($hWnd)
$sb = New-Object System.Text.StringBuilder ($len + 1)
[WinActive]::GetWindowText($hWnd, $sb, $sb.Capacity) | Out-Null

$pidValue = 0
[WinActive]::GetWindowThreadProcessId($hWnd, [ref]$pidValue) | Out-Null
$proc = Get-Process -Id $pidValue -ErrorAction SilentlyContinue

$rect = New-Object WinActive+RECT
[WinActive]::GetWindowRect($hWnd, [ref]$rect) | Out-Null

if ($proc -ne $null) {
    [ordered]@{
        hwnd = $hWnd.ToInt64()
        title = $sb.ToString()
        process_name = $proc.ProcessName + ".exe"
        pid = [int]$pidValue
        x = $rect.Left
        y = $rect.Top
        width = $rect.Right - $rect.Left
        height = $rect.Bottom - $rect.Top
        visible = [WinActive]::IsWindowVisible($hWnd)
        minimized = [WinActive]::IsIconic($hWnd)
    } | ConvertTo-Json -Depth 5 -Compress
}
`

	out, err := exec.CommandContext(
		ctx,
		"powershell",
		"-STA",
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-Command",
		script,
	).CombinedOutput()

	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, string(out))
	}

	clean := strings.TrimSpace(string(out))
	if clean == "" {
		return nil, nil
	}

	var raw rawWindow
	if err := json.Unmarshal([]byte(clean), &raw); err != nil {
		return nil, fmt.Errorf("cannot parse active window json: %w. raw=%s", err, clean)
	}

	converted := convertRawWindows([]rawWindow{raw})
	if len(converted) == 0 {
		return nil, nil
	}

	return &converted[0], nil
}

func convertRawWindows(items []rawWindow) []world.WindowRef {
	result := make([]world.WindowRef, 0, len(items))

	for _, item := range items {
		result = append(result, world.WindowRef{
			HWND:        uintptr(item.HWND),
			Title:       strings.TrimSpace(item.Title),
			ProcessName: strings.TrimSpace(item.ProcessName),
			PID:         item.PID,
			X:           item.X,
			Y:           item.Y,
			Width:       item.Width,
			Height:      item.Height,
			Visible:     item.Visible,
			Minimized:   item.Minimized,
		})
	}

	return result
}

func (s *Sensor) isSelfWindow(w world.WindowRef) bool {
	if w.PID == s.selfPID {
		return true
	}

	selfExe := ""
	if exe, err := os.Executable(); err == nil {
		selfExe = strings.ToLower(filepath.Base(exe))
	}

	process := strings.ToLower(strings.TrimSpace(w.ProcessName))

	if selfExe != "" && process == selfExe {
		return true
	}

	return false
}
