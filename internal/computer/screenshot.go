package computer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type ScreenEngine struct {
	state *RuntimeState
}

type ScreenshotResult struct {
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}

func NewScreenEngine(state *RuntimeState) *ScreenEngine {
	return &ScreenEngine{state: state}
}

func (s *ScreenEngine) Screenshot(ctx context.Context) (ScreenshotResult, error) {
	if runtime.GOOS != "windows" {
		return ScreenshotResult{}, fmt.Errorf("screenshot is currently implemented only for Windows")
	}

	dir := filepath.Join(os.TempDir(), "jarvis")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ScreenshotResult{}, err
	}

	path := filepath.Join(dir, fmt.Sprintf("screenshot_%d.png", time.Now().UnixNano()))

	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$bounds = [System.Windows.Forms.SystemInformation]::VirtualScreen

$bitmap = New-Object System.Drawing.Bitmap $bounds.Width, $bounds.Height
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)

$graphics.CopyFromScreen(
	$bounds.Left,
	$bounds.Top,
	0,
	0,
	$bounds.Size
)

$bitmap.Save('%s', [System.Drawing.Imaging.ImageFormat]::Png)

$graphics.Dispose()
$bitmap.Dispose()
`, escapePowerShellPath(path))

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
		return ScreenshotResult{}, fmt.Errorf("%w: %s", err, string(out))
	}

	result := ScreenshotResult{
		Path:      path,
		CreatedAt: time.Now(),
	}

	if s.state != nil {
		s.state.SetAction("screenshot", path, "ok")
	}

	return result, nil
}

func escapePowerShellPath(path string) string {
	return filepath.Clean(path)
}
