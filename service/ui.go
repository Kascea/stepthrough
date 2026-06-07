package service

// UIService handles UI-layer interactions that require platform dialogs.
// The openFile function is injected by main.go so this package stays free
// of any Wails or CGo dependency and can be tested in CI without GTK headers.
type UIService struct {
	openFile func(title string) (string, error)
	pipeline *PipelineService
}

func NewUIService(pipeline *PipelineService, openFile func(title string) (string, error)) *UIService {
	return &UIService{pipeline: pipeline, openFile: openFile}
}

// SelectAndAdd opens a file dialog and adds the chosen file as a new pipeline tab.
func (u *UIService) SelectAndAdd() string {
	file, err := u.openFile("Select azure-pipelines.yml")
	if err != nil || file == "" {
		return ""
	}
	go u.pipeline.AddTab(file)
	return file
}

// RelocateAndWatch opens a file dialog to locate a moved pipeline file.
// Relocates the old tab to the new file path and starts watching it.
func (u *UIService) RelocateAndWatch(oldFile string) string {
	file, err := u.openFile("Locate pipeline file")
	if err != nil || file == "" {
		return ""
	}
	go u.pipeline.RelocatePipeline(oldFile, file)
	return file
}
