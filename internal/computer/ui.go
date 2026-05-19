package computer

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type UIEngine struct {
	state *RuntimeState
}

func NewUIEngine(state *RuntimeState) *UIEngine {
	return &UIEngine{state: state}
}

type UISnapshot struct {
	WindowTitle string     `json:"window_title"`
	ProcessName string     `json:"process_name"`
	PID         int        `json:"pid"`
	CapturedAt  time.Time  `json:"captured_at"`
	Root        *UIElement `json:"root,omitempty"`
}

type UIElement struct {
	Name         string      `json:"name,omitempty"`
	ControlType  string      `json:"control_type,omitempty"`
	AutomationID string      `json:"automation_id,omitempty"`
	ClassName    string      `json:"class_name,omitempty"`
	Bounds       UIRect      `json:"bounds"`
	IsEnabled    bool        `json:"is_enabled"`
	Children     []UIElement `json:"children,omitempty"`
}

type UIRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func (u *UIEngine) SnapshotActiveWindow(ctx context.Context, depth int, maxChildren int) (UISnapshot, error) {
	if runtime.GOOS != "windows" {
		return UISnapshot{}, fmt.Errorf("ui snapshot is currently implemented only for Windows")
	}

	if depth <= 0 {
		depth = 4
	}

	if maxChildren <= 0 {
		maxChildren = 80
	}

	script := fmt.Sprintf(`
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)

Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes

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

function Get-ControlTypeName($element) {
	try {
		$raw = $element.Current.ControlType.ProgrammaticName
		if ($raw -eq $null) { return "" }
		return $raw.Replace("ControlType.", "")
	} catch {
		return ""
	}
}

function Convert-Element($element, $level, $maxDepth, $maxChildren) {
	if ($null -eq $element) {
		return $null
	}

	$rect = $element.Current.BoundingRectangle

	$item = [ordered]@{
		name = $element.Current.Name
		control_type = Get-ControlTypeName $element
		automation_id = $element.Current.AutomationId
		class_name = $element.Current.ClassName
		bounds = [ordered]@{
			x = $rect.X
			y = $rect.Y
			width = $rect.Width
			height = $rect.Height
		}
		is_enabled = $element.Current.IsEnabled
		children = @()
	}

	if ($level -ge $maxDepth) {
		return $item
	}

	$children = $element.FindAll(
		[System.Windows.Automation.TreeScope]::Children,
		[System.Windows.Automation.Condition]::TrueCondition
	)

	$count = 0
	foreach ($child in $children) {
		if ($count -ge $maxChildren) {
			break
		}

		$converted = Convert-Element $child ($level + 1) $maxDepth $maxChildren
		if ($null -ne $converted) {
			$item.children += $converted
			$count++
		}
	}

	return $item
}

$hwnd = [WinAPI]::GetForegroundWindow()
$pidValue = 0
[WinAPI]::GetWindowThreadProcessId($hwnd, [ref]$pidValue) | Out-Null
$proc = Get-Process -Id $pidValue -ErrorAction SilentlyContinue
$root = [System.Windows.Automation.AutomationElement]::FromHandle($hwnd)

$result = [ordered]@{
	window_title = ""
	process_name = ""
	pid = $pidValue
	root = $null
}

if ($proc -ne $null) {
	$result.window_title = $proc.MainWindowTitle
	$result.process_name = $proc.ProcessName + ".exe"
}

if ($root -ne $null) {
	$result.root = Convert-Element $root 0 %d %d
}

$result | ConvertTo-Json -Depth 20 -Compress
`, depth, maxChildren)

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
		return UISnapshot{}, fmt.Errorf("%w: %s", err, string(out))
	}

	clean := strings.TrimSpace(string(out))
	if clean == "" {
		return UISnapshot{}, fmt.Errorf("empty UI snapshot")
	}

	var snapshot UISnapshot
	if err := json.Unmarshal([]byte(clean), &snapshot); err != nil {
		return UISnapshot{}, fmt.Errorf("cannot parse UI snapshot: %w. raw=%s", err, clean)
	}

	snapshot.CapturedAt = time.Now()

	if u.state != nil {
		u.state.SetAction("ui_snapshot", snapshot.ProcessName, "ok")
	}

	return snapshot, nil
}

func FormatUISnapshot(snapshot UISnapshot) string {
	var b strings.Builder

	b.WriteString("UI активного окна:\n")
	b.WriteString(fmt.Sprintf("- window: %s\n", snapshot.WindowTitle))
	b.WriteString(fmt.Sprintf("- process: %s pid=%d\n\n", snapshot.ProcessName, snapshot.PID))

	if snapshot.Root == nil {
		b.WriteString("UI tree не найден.\n")
		return b.String()
	}

	writeUIElement(&b, *snapshot.Root, 0, 120)

	return b.String()
}

func writeUIElement(b *strings.Builder, el UIElement, level int, remaining int) int {
	if remaining <= 0 {
		return 0
	}

	indent := strings.Repeat("  ", level)

	label := strings.TrimSpace(el.Name)
	if label == "" {
		label = strings.TrimSpace(el.AutomationID)
	}
	if label == "" {
		label = strings.TrimSpace(el.ClassName)
	}
	if label == "" {
		label = "(no name)"
	}

	b.WriteString(fmt.Sprintf(
		"%s- %s | type=%s | bounds=%.0f,%.0f %.0fx%.0f\n",
		indent,
		label,
		el.ControlType,
		el.Bounds.X,
		el.Bounds.Y,
		el.Bounds.Width,
		el.Bounds.Height,
	))

	remaining--

	for _, child := range el.Children {
		if remaining <= 0 {
			break
		}

		used := writeUIElement(b, child, level+1, remaining)
		remaining -= used
	}

	return 1
}
