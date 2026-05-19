package screen

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"jarvis/internal/world"
)

func NewSensor(opts Options) *Sensor {
	return &Sensor{
		state: opts.State,
	}
}

func (s *Sensor) CaptureWindow(ctx context.Context, target world.WindowRef, opts CaptureOptions) (world.ScreenCaptureRef, error) {
	if target.HWND == 0 || target.Width <= 0 || target.Height <= 0 {
		return world.ScreenCaptureRef{}, fmt.Errorf("invalid target window")
	}

	dir := filepath.Join(os.TempDir(), "jarvis", "screens")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return world.ScreenCaptureRef{}, err
	}

	path := filepath.Join(dir, fmt.Sprintf("window_%d_%d.png", target.HWND, time.Now().UnixNano()))

	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$x = %d
$y = %d
$w = %d
$h = %d

$bitmap = New-Object System.Drawing.Bitmap $w, $h
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)

$graphics.CopyFromScreen(
	$x,
	$y,
	0,
	0,
	[System.Drawing.Size]::new($w, $h)
)

$bitmap.Save('%s', [System.Drawing.Imaging.ImageFormat]::Png)

$graphics.Dispose()
$bitmap.Dispose()
`,
		target.X,
		target.Y,
		target.Width,
		target.Height,
		escapePowerShellPath(path),
	)

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
		return world.ScreenCaptureRef{}, fmt.Errorf("%w: %s", err, string(out))
	}

	capture := world.ScreenCaptureRef{
		Path:      path,
		Window:    target,
		Width:     target.Width,
		Height:    target.Height,
		CreatedAt: time.Now(),
		KeepFile:  opts.KeepFile,
	}

	if s.state != nil {
		s.state.SetScreenCapture(capture)
	}

	if !opts.KeepFile {
		_ = os.Remove(path)
		capture.Path = ""
	}

	return capture, nil
}

func escapePowerShellPath(path string) string {
	return strings.ReplaceAll(filepath.Clean(path), "'", "''")
}
