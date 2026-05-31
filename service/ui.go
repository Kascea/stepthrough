package service

import "github.com/wailsapp/wails/v3/pkg/application"

// UIService handles UI-layer interactions that require a live Wails app —
// currently just native file-picker dialogs. All file-watching logic lives
// in WatcherService, which has no dependency on the UI framework.
type UIService struct {
	app     *application.App
	watcher *WatcherService
}

func NewUIService(watcher *WatcherService) *UIService {
	return &UIService{watcher: watcher}
}

func (u *UIService) SetApp(app *application.App) { u.app = app }

// SelectAndWatch opens a native file dialog and, if the user picks a file,
// starts watching it. Returns the selected path, or empty string if cancelled.
func (u *UIService) SelectAndWatch() string {
	file, err := u.app.Dialog.OpenFile().
		SetTitle("Select azure-pipelines.yml").
		AddFilter("YAML files (*.yml, *.yaml)", "*.yml;*.yaml").
		AddFilter("All files", "*").
		PromptForSingleSelection()
	if err != nil || file == "" {
		return ""
	}
	go func() {
		if err := u.watcher.Watch(file); err != nil {
			u.watcher.emit("watcher:error", err.Error())
		}
	}()
	return file
}
