package main

import (
	"embed"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/colecarlson/stepthrough/orchestrator"
	"github.com/colecarlson/stepthrough/runner"
	"github.com/colecarlson/stepthrough/service"
	"github.com/colecarlson/stepthrough/session"
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

	var app *application.App
	sysEmit := func(event string, data any) {
		if app != nil {
			app.Event.Emit(event, data)
		}
	}

	watcherSvc := service.NewWatcherService(pipelineSvc, sysEmit)
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
		// CLI arg mode: open one file, ignore saved session.
		pipelineSvc.SetPendingFile(pipelineFile)
		go func() {
			// Give the frontend time to mount and register event listeners.
			time.Sleep(400 * time.Millisecond)
			if err := watcherSvc.AddWatch(pipelineFile); err != nil {
				log.Printf("watcher error: %v", err)
			}
		}()
	} else {
		// Session restore mode: validate saved files after the frontend mounts.
		go func() {
			time.Sleep(400 * time.Millisecond)
			sess := session.Load()
			for _, file := range sess.TabOrder {
				// RestoreTab creates the orchestrator and emits pipeline:loaded or pipeline:missing.
				// It does NOT auto-run — the user decides when to run.
				pipelineSvc.RestoreTab(file)

				// For files that exist, start the hot-reload watcher.
				// AddWatch is a no-op for the initial load (EnsureTab returns false since
				// RestoreTab already created the orchestrator).
				if _, err := os.Stat(file); err == nil {
					if err := watcherSvc.AddWatch(file); err != nil {
						log.Printf("watcher error for %s: %v", file, err)
					}
				}
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
