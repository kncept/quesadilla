package tray

import (
	"fmt"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"

	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
	running "github.com/kncept/quesadilla/backend/running"
)

// SetupTray creates the initial tray menu and registers it with the tray app.
// Returns the newly created tray menu, which the caller should store and use
// for subsequent RefreshTrayMenu calls.
func SetupTray(trayApp desktop.App) *fyne.Menu {
	complete := new(sync.WaitGroup)
	complete.Add(1)

	trayMenu := fyne.NewMenu("Quesadilla")

	fyne.Do(func() {
		// the menu must exist before the icon (the tray item is created by it)
		trayApp.SetSystemTrayMenu(trayMenu)
		trayApp.SetSystemTrayIcon(theme.ComputerIcon())
		complete.Done()
	})
	complete.Wait()

	return trayMenu
}

// RefreshTrayMenu rebuilds the tray menu from the current state.
// trayMenu is the existing tray menu to update, mu protects concurrent access,
// runningModels are the current model names, and the callback functions handle
// user actions (open control suite, model selected, quit).
func RefreshTrayMenu(trayMenu *fyne.Menu, mu *sync.Mutex, runningModels []string,
	onOpenControlSuite func(),
	onModelSelected func(string, runnerDefinitions.RunningModel),
	onQuit func()) {

	if trayMenu == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()

	items := []*fyne.MenuItem{
		fyne.NewMenuItem("Open Control Suite", func() {
			onOpenControlSuite()
		}),
		fyne.NewMenuItemSeparator(),
	}

	if len(runningModels) == 0 {
		items = append(items, disabledItem("No models running"))
	} else {
		items = append(items, disabledItem("Models running:"))
		for _, name := range runningModels {
			// the model name opens (or focuses) that model's log window
			items = append(items, fyne.NewMenuItem("  "+name, func() {
				onModelSelected(name, runningModelFor(name))
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

// runningModelFor looks up a running model by name. Returns nil when no such
// model is running.
func runningModelFor(name string) runnerDefinitions.RunningModel {
	for _, m := range running.Default().RunningModels() {
		if m.ModelName() == name {
			return m
		}
	}
	return nil
}