package tray

import (
	"fmt"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"

	running "github.com/kncept/quesadilla/backend/running"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

type QAppTray struct {
	mu                    sync.Mutex
	trayApp               desktop.App
	trayMenu              *fyne.Menu
	runningModelsRegistry *running.Registry
	openControlSuite      func()
}

func SupportsTray(app fyne.App) bool {
	if _, ok := app.(desktop.App); ok {
		return true
	}
	return false
}

func InitializeTray(
	app fyne.App,
	openControlSuite func(),
	runningModelsRegistry *running.Registry,
) *QAppTray {
	if desktopApp, ok := app.(desktop.App); ok {
		this := &QAppTray{
			trayApp:               desktopApp,
			trayMenu:              fyne.NewMenu("Quesadilla"),
			openControlSuite:      openControlSuite,
			runningModelsRegistry: runningModelsRegistry,
		}

		complete := new(sync.WaitGroup)
		complete.Add(1)
		fyne.Do(func() {
			// the menu must exist before the icon (the tray item is created by it)
			this.trayApp.SetSystemTrayMenu(this.trayMenu)
			this.trayApp.SetSystemTrayIcon(theme.ComputerIcon())
			complete.Done()
		})
		complete.Wait()

		return this
	} else {
		panic("System Tray not supported")
	}

}

// TODO: this should be Object based, not Procedural - this function should manage RefreshTrayMenu
func (this *QAppTray) Refresh() {
	if this == nil || this.trayMenu == nil {
		return
	}
	// RefreshTrayMenu()
}

// todo: remove
// RefreshTrayMenu rebuilds the tray menu from the current state.
// trayMenu is the existing tray menu to update, mu protects concurrent access,
// runningModels are the current model names, and the callback functions handle
// user actions (open control suite, model selected, quit).
func RefreshTrayMenu(
	tray *QAppTray,
	onModelSelected func(runnerDefinitions.RunningModel),
	onQuit func()) {

	if tray == nil {
		return
	}
	trayMenu := tray.trayMenu
	if trayMenu == nil {
		return
	}
	tray.mu.Lock()
	defer tray.mu.Unlock()

	items := []*fyne.MenuItem{
		fyne.NewMenuItem("Open Control Suite", func() {
			tray.openControlSuite()
		}),
		fyne.NewMenuItemSeparator(),
	}

	runningModels := tray.runningModelsRegistry.RunningModels()

	if len(runningModels) == 0 {
		items = append(items, disabledItem("No models running"))
	} else {
		items = append(items, disabledItem("Models running:"))
		for _, runningModel := range runningModels {
			// the model name opens (or focuses) that model's log window
			items = append(items, fyne.NewMenuItem("  "+runningModel.ModelName(), func() {
				onModelSelected(runningModel)
			}))
		}
	}

	items = append(items, fyne.NewMenuItemSeparator())
	items = append(items, fyne.NewMenuItem("Quit", func() { onQuit() }))

	if !trayItemsAreEqualItems(trayMenu.Items, items) {
		fyne.Do(func() {
			fmt.Printf("Updating system tray\n")
			trayMenu.Items = items
			trayMenu.Refresh()
		})
	}
}

// trayItemsAreEqualItems checks if two slices of menu items have the same labels.
func trayItemsAreEqualItems(items, compare []*fyne.MenuItem) bool {
	if len(items) != len(compare) {
		return false
	}
	for idx, item := range items {
		if item.Label != compare[idx].Label {
			return false
		}
	}
	return true
}

// disabledItem creates a disabled menu item with the given label.
func disabledItem(label string) *fyne.MenuItem {
	item := fyne.NewMenuItem(label, nil)
	item.Disabled = true
	return item
}
