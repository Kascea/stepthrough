package service

import "github.com/wailsapp/wails/v3/pkg/application"

// UIService handles UI-layer interactions that require a live Wails app.
type UIService struct {
	app      *application.App
	pipeline *PipelineService
}

func NewUIService(pipeline *PipelineService) *UIService {
	return &UIService{pipeline: pipeline}
}

func (u *UIService) SetApp(app *application.App) { u.app = app }

// SelectAndAdd opens a file dialog and adds the chosen file as a new pipeline tab.
func (u *UIService) SelectAndAdd() string {
	file, err := u.app.Dialog.OpenFile().
		SetTitle("Select azure-pipelines.yml").
		AddFilter("YAML files (*.yml, *.yaml)", "*.yml;*.yaml").
		AddFilter("All files", "*").
		PromptForSingleSelection()
	if err != nil || file == "" {
		return ""
	}
	go u.pipeline.AddTab(file)
	return file
}

// RelocateAndWatch opens a file dialog to locate a moved pipeline file.
// Relocates the old tab to the new file path and starts watching it.
func (u *UIService) RelocateAndWatch(oldFile string) string {
	file, err := u.app.Dialog.OpenFile().
		SetTitle("Locate pipeline file").
		AddFilter("YAML files (*.yml, *.yaml)", "*.yml;*.yaml").
		AddFilter("All files", "*").
		PromptForSingleSelection()
	if err != nil || file == "" {
		return ""
	}
	go u.pipeline.RelocatePipeline(oldFile, file)
	return file
}
