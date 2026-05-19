package ocr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	screensensor "jarvis/internal/sensors/screen"
	"jarvis/internal/world"
)

func NewSensor(opts Options) *Sensor {
	return &Sensor{
		state:        opts.State,
		screenSensor: opts.ScreenSensor,
	}
}

func (s *Sensor) RecognizeWindow(ctx context.Context, target world.WindowRef, opts OCRCaptureOptions) (world.OCRSnapshot, error) {
	if s.screenSensor == nil {
		return world.OCRSnapshot{}, fmt.Errorf("screen sensor is nil")
	}

	capture, err := s.screenSensor.CaptureWindow(ctx, target, screensensor.CaptureOptions{
		KeepFile: true,
	})
	if err != nil {
		return world.OCRSnapshot{}, err
	}

	ocrResult, err := recognizeImageWithWindowsOCR(ctx, capture.Path)
	if err != nil {
		if !opts.KeepDebugScreenshot {
			_ = os.Remove(capture.Path)
		}
		return world.OCRSnapshot{}, err
	}

	snapshot := world.OCRSnapshot{
		Window:     target,
		SourcePath: capture.Path,
		FullText:   strings.TrimSpace(ocrResult.FullText),
		Lines:      ocrResult.Lines,
		Words:      ocrResult.Words,
		CapturedAt: time.Now(),
	}

	if !opts.KeepDebugScreenshot {
		_ = os.Remove(capture.Path)
		snapshot.SourcePath = ""
	}

	if s.state != nil {
		s.state.SetOCRSnapshot(snapshot)
	}

	return snapshot, nil
}

type windowsOCRResult struct {
	FullText string          `json:"full_text"`
	Lines    []world.OCRLine `json:"lines"`
	Words    []world.OCRWord `json:"words"`
}

func recognizeImageWithWindowsOCR(ctx context.Context, path string) (windowsOCRResult, error) {
	script := fmt.Sprintf(`
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)

Add-Type -AssemblyName System.Runtime.WindowsRuntime

[Windows.Storage.StorageFile, Windows.Storage, ContentType = WindowsRuntime] | Out-Null
[Windows.Storage.FileAccessMode, Windows.Storage, ContentType = WindowsRuntime] | Out-Null
[Windows.Storage.Streams.IRandomAccessStream, Windows.Storage.Streams, ContentType = WindowsRuntime] | Out-Null
[Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics.Imaging, ContentType = WindowsRuntime] | Out-Null
[Windows.Graphics.Imaging.SoftwareBitmap, Windows.Graphics.Imaging, ContentType = WindowsRuntime] | Out-Null
[Windows.Media.Ocr.OcrEngine, Windows.Media.Ocr, ContentType = WindowsRuntime] | Out-Null

$asTaskGeneric = ([System.WindowsRuntimeSystemExtensions].GetMethods() |
	Where-Object {
		$_.Name -eq 'AsTask' -and
		$_.IsGenericMethod -and
		$_.GetParameters().Count -eq 1
	} | Select-Object -First 1)

function Await($operation, $resultType) {
	$asTask = $asTaskGeneric.MakeGenericMethod($resultType)
	$task = $asTask.Invoke($null, @($operation))
	$task.Wait()
	return $task.Result
}

$imagePath = '%s'

$file = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($imagePath)) ([Windows.Storage.StorageFile])
$stream = Await ($file.OpenAsync([Windows.Storage.FileAccessMode]::Read)) ([Windows.Storage.Streams.IRandomAccessStream])
$decoder = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
$bitmap = Await ($decoder.GetSoftwareBitmapAsync()) ([Windows.Graphics.Imaging.SoftwareBitmap])

$engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()

if ($engine -eq $null) {
	throw "Windows OCR engine is not available. Install OCR language pack in Windows language settings."
}

$result = Await ($engine.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])

$lines = @()
$wordsAll = @()

foreach ($line in $result.Lines) {
	$lineWords = @()

	foreach ($word in $line.Words) {
		$rect = $word.BoundingRect

		$wordObj = [ordered]@{
			text = $word.Text
			x = $rect.X
			y = $rect.Y
			width = $rect.Width
			height = $rect.Height
			confidence = 0
		}

		$lineWords += $wordObj
		$wordsAll += $wordObj
	}

	$lines += [ordered]@{
		text = $line.Text
		words = $lineWords
	}
}

[ordered]@{
	full_text = $result.Text
	lines = $lines
	words = $wordsAll
} | ConvertTo-Json -Depth 20 -Compress

$stream.Dispose()
`, escapePowerShellSingleQuoted(path))

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
		return windowsOCRResult{}, fmt.Errorf("%w: %s", err, string(out))
	}

	clean := strings.TrimSpace(string(out))
	if clean == "" {
		return windowsOCRResult{}, fmt.Errorf("empty OCR result")
	}

	var result windowsOCRResult
	if err := json.Unmarshal([]byte(clean), &result); err != nil {
		return windowsOCRResult{}, fmt.Errorf("cannot parse OCR json: %w. raw=%s", err, clean)
	}

	return result, nil
}

func escapePowerShellSingleQuoted(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
