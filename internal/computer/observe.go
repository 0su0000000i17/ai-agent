package computer

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Observer struct {
	state *RuntimeState
}

func NewObserver(state *RuntimeState) *Observer {
	return &Observer{state: state}
}

func (o *Observer) Observe(ctx context.Context) (Observation, error) {
	var obs Observation
	var err error

	switch runtime.GOOS {
	case "windows":
		obs, err = observeWindows(ctx)
	default:
		obs, err = observeUnix(ctx)
	}

	if err != nil {
		return Observation{}, err
	}

	obs.ObservedAt = time.Now()

	if o.state != nil {
		o.state.SetObservation(obs)
	}

	return obs, nil
}

func observeWindows(ctx context.Context) (Observation, error) {
	processes, err := windowsProcesses(ctx)
	if err != nil {
		return Observation{}, err
	}

	windows, err := windowsVisibleWindows(ctx)
	if err != nil {
		return Observation{}, err
	}

	active, _ := windowsActiveWindow(ctx)

	return Observation{
		ActiveWindow: active,
		Windows:      windows,
		Processes:    processes,
		Source:       "windows_observer",
	}, nil
}

func windowsProcesses(ctx context.Context) ([]ProcessInfo, error) {
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

	result := make([]ProcessInfo, 0, len(records))

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

		result = append(result, ProcessInfo{
			Name: name,
			PID:  pid,
		})
	}

	return result, nil
}

type windowsProcessJSON struct {
	ProcessName     string `json:"ProcessName"`
	ID              int    `json:"Id"`
	MainWindowTitle string `json:"MainWindowTitle"`
}

func windowsVisibleWindows(ctx context.Context) ([]WindowInfo, error) {
	script := `
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)

Get-Process |
Where-Object { $_.MainWindowTitle -ne "" } |
Select-Object ProcessName, Id, MainWindowTitle |
ConvertTo-Json -Compress
`

	out, err := runPowerShellUTF8(ctx, script)
	if err != nil {
		return nil, err
	}

	clean := strings.TrimSpace(string(out))
	if clean == "" {
		return []WindowInfo{}, nil
	}

	var many []windowsProcessJSON
	if err := json.Unmarshal([]byte(clean), &many); err == nil {
		return convertWindowsJSON(many), nil
	}

	var one windowsProcessJSON
	if err := json.Unmarshal([]byte(clean), &one); err != nil {
		return []WindowInfo{}, nil
	}

	return convertWindowsJSON([]windowsProcessJSON{one}), nil
}

func convertWindowsJSON(items []windowsProcessJSON) []WindowInfo {
	result := make([]WindowInfo, 0, len(items))

	for _, item := range items {
		if strings.TrimSpace(item.MainWindowTitle) == "" {
			continue
		}

		result = append(result, WindowInfo{
			Title:       item.MainWindowTitle,
			ProcessName: item.ProcessName + ".exe",
			PID:         item.ID,
		})
	}

	return result
}

type activeWindowJSON struct {
	ProcessName string `json:"ProcessName"`
	ID          int    `json:"Id"`
	Title       string `json:"Title"`
}

func windowsActiveWindow(ctx context.Context) (*WindowInfo, error) {
	script := `
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)

Add-Type @"
using System;
using System.Runtime.InteropServices;
public class WinAPI {
	[DllImport("user32.dll")]
	public static extern IntPtr GetForegroundWindow();

	[DllImport("user32.dll")]
	public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint processId);
}
"@

$hwnd = [WinAPI]::GetForegroundWindow()
$pidValue = 0
[WinAPI]::GetWindowThreadProcessId($hwnd, [ref]$pidValue) | Out-Null
$p = Get-Process -Id $pidValue -ErrorAction SilentlyContinue

if ($p -ne $null) {
	[PSCustomObject]@{
		ProcessName = $p.ProcessName
		Id = $p.Id
		Title = $p.MainWindowTitle
	} | ConvertTo-Json -Compress
}
`

	out, err := runPowerShellUTF8(ctx, script)
	if err != nil {
		return nil, err
	}

	clean := strings.TrimSpace(string(out))
	if clean == "" {
		return nil, nil
	}

	var active activeWindowJSON
	if err := json.Unmarshal([]byte(clean), &active); err != nil {
		return nil, err
	}

	return &WindowInfo{
		Title:       active.Title,
		ProcessName: active.ProcessName + ".exe",
		PID:         active.ID,
	}, nil
}

func runPowerShellUTF8(ctx context.Context, script string) ([]byte, error) {
	cmd := exec.CommandContext(
		ctx,
		"powershell",
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-Command",
		script,
	)

	return cmd.Output()
}

func observeUnix(ctx context.Context) (Observation, error) {
	out, err := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,comm=").Output()
	if err != nil {
		return Observation{}, err
	}

	lines := strings.Split(string(out), "\n")
	processes := make([]ProcessInfo, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		pid, _ := strconv.Atoi(parts[0])
		name := parts[1]

		processes = append(processes, ProcessInfo{
			Name: name,
			PID:  pid,
		})
	}

	return Observation{
		Processes: processes,
		Windows:   []WindowInfo{},
		Source:    "unix_process_snapshot",
	}, nil
}
