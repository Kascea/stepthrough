package main

import (
	"embed"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/colecarlson/stepthrough/orchestrator"
	"github.com/colecarlson/stepthrough/runner"
	"github.com/colecarlson/stepthrough/service"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	pipelineFile := ""
	if len(os.Args) > 1 {
		pipelineFile = os.Args[1]
	}

	factory := func(workDir string) orchestrator.Executor {
		return runner.NewEngine(workDir)
	}

	pipelineSvc := service.NewPipelineService(factory)

	// emit is defined before the app so it can be injected into services,
	// but it captures app by pointer so calls made at runtime see the live instance.
	var app *application.App
	emit := func(event string, data any) {
		if app != nil {
			app.Event.Emit(event, data)
		}
	}

	watcherSvc := service.NewWatcherService(pipelineSvc, emit)
	uiSvc := service.NewUIService(watcherSvc)

	app = application.New(application.Options{
		Name:        "stepthrough",
		Description: "Azure Pipelines local runner with hot-refresh",
		Services: []application.Service{
			application.NewService(pipelineSvc),
			application.NewService(watcherSvc),
			application.NewService(uiSvc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	pipelineSvc.SetApp(app)
	uiSvc.SetApp(app)

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "stepthrough",
		Width:  1400,
		Height: 900,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(255, 255, 255),
		URL:              "/",
	})

	if pipelineFile != "" {
		go func() {
			if err := watcherSvc.Watch(pipelineFile); err != nil {
				log.Printf("watcher error: %v", err)
			}
		}()
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		pipelineSvc.Cleanup()
		app.Quit()
	}()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
	pipelineSvc.Cleanup()
}
