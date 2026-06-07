package main

import (
	"embed"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/kascea/stepthrough/orchestrator"
	"github.com/kascea/stepthrough/runner"
	"github.com/kascea/stepthrough/service"
	"github.com/kascea/stepthrough/session"
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

	uiSvc := service.NewUIService(pipelineSvc)

	app = application.New(application.Options{
		Name:        "stepthrough",
		Description: "Azure Pipelines local runner with hot-refresh",
		Services: []application.Service{
			application.NewService(pipelineSvc),
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
			pipelineSvc.AddTab(pipelineFile)
		}()
	} else {
		// Session restore mode: validate saved files after the frontend mounts.
		// RestoreTab creates the orchestrator, starts the watcher if the file exists,
		// and emits pipeline:loaded or pipeline:missing.
		go func() {
			time.Sleep(400 * time.Millisecond)
			sess := session.Load()
			for _, file := range sess.TabOrder {
				pipelineSvc.RestoreTab(file)
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
