package main

import (
	"context"
	"strings"

	"jarvis/internal/ai"
	"jarvis/internal/config"
	"jarvis/internal/core"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx   context.Context
	agent *core.Agent
}

func NewApp() *App {
	cfg, err := config.Load("config.json")
	if err != nil {
		panic(err)
	}

	client := ai.NewClient(cfg)
	agent := core.NewAgent(client)

	return &App{
		agent: agent,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	wailsruntime.WindowSetAlwaysOnTop(ctx, true)
	wailsruntime.WindowSetPosition(ctx, 40, 40)

	a.agent.Start(ctx)
}

func (a *App) SendMessage(message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", nil
	}

	if a.ctx == nil {
		a.ctx = context.Background()
	}

	return a.agent.Handle(a.ctx, message)
}

func (a *App) MinimizeHUD() {
	if a.ctx == nil {
		return
	}

	wailsruntime.WindowSetSize(a.ctx, 360, 76)
}

func (a *App) ExpandHUD() {
	if a.ctx == nil {
		return
	}

	wailsruntime.WindowSetSize(a.ctx, 520, 360)
}

func (a *App) MoveHUD(x int, y int) {
	if a.ctx == nil {
		return
	}

	wailsruntime.WindowSetPosition(a.ctx, x, y)
}

func (a *App) CloseApp() {
	if a.ctx == nil {
		return
	}

	a.agent.Stop()
	wailsruntime.Quit(a.ctx)
}
