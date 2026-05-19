package computer

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type ActionEngine struct {
	state   *RuntimeState
	observe *Observer
	resolve *Resolver
}

func NewActionEngine(state *RuntimeState, observer *Observer, resolver *Resolver) *ActionEngine {
	return &ActionEngine{
		state:   state,
		observe: observer,
		resolve: resolver,
	}
}

func (a *ActionEngine) Open(ctx context.Context, input string) (ResolveResult, error) {
	if a.observe != nil {
		_, _ = a.observe.Observe(ctx)
	}

	resolved := a.resolve.Resolve(input)
	return a.OpenResolved(ctx, resolved)
}

func (a *ActionEngine) OpenResolved(ctx context.Context, resolved ResolveResult) (ResolveResult, error) {
	if !resolved.Resolved {
		return resolved, nil
	}

	switch resolved.Kind {
	case ResolveKindWebsite:
		err := openURL(ctx, resolved.Target)
		if a.state != nil {
			a.state.SetAction("open", resolved.Target, errorToResult(err))
		}
		return resolved, err

	case ResolveKindApp:
		err := openAppGeneric(ctx, resolved.Target)
		if a.state != nil {
			a.state.SetAction("open_app", resolved.Target, errorToResult(err))
		}
		return resolved, err

	case ResolveKindWindow, ResolveKindProcess:
		if a.state != nil {
			a.state.SetAction("open", resolved.Target, "already_visible_or_running")
		}
		return resolved, nil

	default:
		return resolved, errors.New("unsupported open target")
	}
}

func (a *ActionEngine) Close(ctx context.Context, input string, force bool) (ResolveResult, error) {
	if a.observe != nil {
		_, _ = a.observe.Observe(ctx)
	}

	resolved := a.resolve.Resolve(input)
	return a.CloseResolved(ctx, resolved, force)
}

func (a *ActionEngine) CloseResolved(ctx context.Context, resolved ResolveResult, force bool) (ResolveResult, error) {
	if !resolved.Resolved {
		return resolved, nil
	}

	switch resolved.Kind {
	case ResolveKindWindow, ResolveKindProcess, ResolveKindApp:
		var err error

		if force {
			err = terminateProcess(ctx, resolved.Target)
		} else {
			err = gracefulClose(ctx, resolved.Target)
		}

		time.Sleep(700 * time.Millisecond)

		if a.observe != nil {
			_, _ = a.observe.Observe(ctx)
		}

		if a.state != nil {
			mode := "close"
			if force {
				mode = "terminate"
			}
			a.state.SetAction(mode, resolved.Target, errorToResult(err))
		}

		return resolved, err

	default:
		return resolved, errors.New("unsupported close target")
	}
}

func openURL(ctx context.Context, target string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.CommandContext(ctx, "cmd", "/c", "start", "", target).Start()
	case "darwin":
		return exec.CommandContext(ctx, "open", target).Start()
	default:
		return exec.CommandContext(ctx, "xdg-open", target).Start()
	}
}

func openAppGeneric(ctx context.Context, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return errors.New("empty app target")
	}

	switch runtime.GOOS {
	case "windows":
		return exec.CommandContext(ctx, "cmd", "/c", "start", "", target).Start()
	case "darwin":
		return exec.CommandContext(ctx, "open", "-a", target).Start()
	default:
		return exec.CommandContext(ctx, target).Start()
	}
}

func gracefulClose(ctx context.Context, processName string) error {
	processName = strings.TrimSpace(processName)
	if processName == "" {
		return errors.New("empty process name")
	}

	switch runtime.GOOS {
	case "windows":
		processName = strings.TrimSuffix(processName, ".exe")

		script := `
$procName = "` + processName + `"
Get-Process -Name $procName -ErrorAction SilentlyContinue | ForEach-Object {
	if ($_.MainWindowHandle -ne 0) {
		$_.CloseMainWindow() | Out-Null
	}
}
`
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script).Run()

	default:
		return exec.CommandContext(ctx, "pkill", "-TERM", "-x", processName).Run()
	}
}

func terminateProcess(ctx context.Context, processName string) error {
	processName = strings.TrimSpace(processName)
	if processName == "" {
		return errors.New("empty process name")
	}

	switch runtime.GOOS {
	case "windows":
		return exec.CommandContext(ctx, "taskkill", "/IM", processName, "/T", "/F").Run()
	default:
		return exec.CommandContext(ctx, "pkill", "-KILL", "-x", processName).Run()
	}
}

func errorToResult(err error) string {
	if err != nil {
		return err.Error()
	}

	return "ok"
}
