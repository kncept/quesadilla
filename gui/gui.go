package gui

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/postfinance/single"

	"github.com/kncept/quesadilla/backend/running"
	"github.com/kncept/quesadilla/utils/qenv"
)

// SINGLE Q GUI
// DO NOT instantiate - use the 'Create' func
type QGUI struct {
	SysLink SystemAgnosticOperations

	mu      sync.Mutex
	started bool
	app     fyne.App
	tray    *fyne.Menu
	mainWin fyne.Window
}

// Start launches the lightweight system-tray indicator. Blocks until quit.
// Panics if called more than once.
func (this *QGUI) Start() {
	this.mu.Lock()
	if this.started {
		this.mu.Unlock()
		panic("gui: QGUI.Start called more than once")
	}

	release, ok := acquireInstanceLock(qenv.QDir())
	if !ok {
		this.mu.Unlock()
		return
	}

	this.started = true
	this.mu.Unlock()
	defer release()

	this.app = app.NewWithID("com.github.kncept.quesadilla")

	if trayApp, ok := this.app.(desktop.App); ok {
		this.setupTray(trayApp)
	} else {
		// no system tray on this platform, fall back to the control suite window
		this.showMainWindow()
	}

	this.app.Run()
}

// setupTray installs the lightweight indicator: a tray icon whose menu
// launches the main control suite and lists the running models.
func (this *QGUI) setupTray(trayApp desktop.App) {
	this.tray = fyne.NewMenu("Quesadilla")
	this.refreshTrayMenu()
	// the menu must exist before the icon (the tray item is created by it)
	trayApp.SetSystemTrayMenu(this.tray)
	trayApp.SetSystemTrayIcon(theme.ComputerIcon())

	// Keep the model status in the tray menu up to date.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			this.refreshTrayMenu()
		}
	}()
}

// singleInstanceName is the lock file (without extension) that ensures
// only one instance of the GUI is running at a time.
const singleInstanceName = "quesadilla-gui"

// acquireInstanceLock takes the single-instance lock in lockDir. On success
// it returns a release function (safe to defer) and true. If an instance is
// already running, it prints an error message to the console and returns
// false.
func acquireInstanceLock(lockDir string) (func(), bool) {
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		panic(fmt.Sprintf("gui: creating lock directory: %v", err))
	}
	s, err := single.New(singleInstanceName, single.WithLockPath(lockDir))
	if err != nil {
		panic(fmt.Sprintf("gui: creating single instance lock: %v", err))
	}
	if err = s.Lock(); errors.Is(err, single.ErrAlreadyRunning) {
		fmt.Fprintln(os.Stderr, "Quesadilla is already running")
		return nil, false
	}
	if err != nil {
		panic(fmt.Sprintf("gui: acquiring single instance lock: %v", err))
	}
	return func() { _ = s.Unlock() }, true
}

func CreateGui(SysLink SystemAgnosticOperations) *QGUI {
	return &QGUI{SysLink: SysLink}
}

// RunningModelNames returns the names of the models currently running.
func (this *QGUI) RunningModelNames() []string {
	return running.Default().Names()
}

// refreshTrayMenu rebuilds the system tray menu from the current state.
func (this *QGUI) refreshTrayMenu() {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.tray == nil {
		return
	}

	models := this.RunningModelNames()

	items := []*fyne.MenuItem{
		fyne.NewMenuItem("Open Control Suite", this.showMainWindow),
		fyne.NewMenuItemSeparator(),
	}

	if len(models) == 0 {
		items = append(items, disabledItem("No models running"))
	} else {
		items = append(items, disabledItem("Models running:"))
		for _, name := range models {
			items = append(items, disabledItem("  "+name))
		}
	}

	items = append(items, fyne.NewMenuItemSeparator())
	quit := fyne.NewMenuItem("Quit", func() { this.app.Quit() })
	quit.IsQuit = true
	items = append(items, quit)

	this.tray.Items = items
	// re-triggers the tray driver to rebuild the native menu
	this.tray.Refresh()
}

func disabledItem(label string) *fyne.MenuItem {
	item := fyne.NewMenuItem(label, nil)
	item.Disabled = true
	return item
}

// showMainWindow opens (or focuses) the main control suite window.
func (this *QGUI) showMainWindow() {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.mainWin == nil {
		this.mainWin = this.app.NewWindow("Quesadilla Control Suite")
		this.mainWin.SetContent(this.mainContent())
		this.mainWin.Resize(fyne.NewSize(800, 600))
		this.mainWin.Show()
	} else {
		this.mainWin.Show()
	}
}

// mainContent creates the split view with a sidebar menu on the left
// and a content view taking up the rest of the window.
func (this *QGUI) mainContent() *fyne.Container {
	// Sidebar menu items - one per screen, per docs/GUI.md
	sidebarItems := []string{"Overview", "Models", "Backends"}

	// One content page per sidebar item; only the selected page is shown.
	pages := container.NewStack(overviewPage(), modelsPage(), backendsPage())

	// The list sizes itself to its template item, so use the longest item
	// as the template to guarantee the sidebar is wide enough for all items.
	longest := sidebarItems[0]
	for _, item := range sidebarItems {
		if len(item) > len(longest) {
			longest = item
		}
	}

	// Sidebar menu. Labels must not wrap: a wrapping label reports a tiny
	// minimum width, which collapsed the sidebar into a narrow strip of
	// wrapped text.
	sidebar := widget.NewList(
		func() int {
			return len(sidebarItems)
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel(longest)
			label.Wrapping = fyne.TextWrapOff
			return label
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*widget.Label).SetText(sidebarItems[id])
		},
	)

	// Show the matching page when a sidebar item is selected.
	sidebar.OnSelected = func(id widget.ListItemID) {
		for i, page := range pages.Objects {
			if i == int(id) {
				page.Show()
			} else {
				page.Hide()
			}
		}
	}

	// Layout using Border layout: sidebar on leading (left), content in
	// center. The padded container gives the sidebar a little breathing room.
	split := container.NewBorder(
		nil,                          // top
		nil,                          // bottom
		container.NewPadded(sidebar), // leading (left/sidebar)
		nil,                          // trailing (right)
		pages,                        // center
	)

	// Select the first item initially (also shows the first page)
	sidebar.Select(0)

	return split
}

// page builds one content page: a bold title, an optional intro, and a set
// of sections. The page is padded so it fills the content area. Sections
// are passed as alternating heading/body pairs.
func page(title, intro string, sections ...string) *fyne.Container {
	heading := func(text string) *widget.Label {
		return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	body := func(text string) *widget.Label {
		label := widget.NewLabel(text)
		label.Wrapping = fyne.TextWrapWord
		return label
	}

	objects := []fyne.CanvasObject{heading(title), widget.NewSeparator()}
	if intro != "" {
		objects = append(objects, body(intro), widget.NewSeparator())
	}
	for i := 0; i+1 < len(sections); i += 2 {
		objects = append(objects, heading(sections[i]), body(sections[i+1]))
		if i+2 < len(sections) {
			objects = append(objects, widget.NewSeparator())
		}
	}
	return container.NewPadded(container.NewVBox(objects...))
}

// overviewPage is the Overview screen: stats and the running models section.
func overviewPage() *fyne.Container {
	return page("Overview",
		"Overview stats would go here.",
		"Running Models",
		"All running models with stats, including uptime, would be listed here. Each model has a 'stop model' button.",
	)
}

// modelsPage is the Models screen: installed models, plus the scan and
// available sections.
func modelsPage() *fyne.Container {
	return page("Models",
		"Installed models, including config and running stats, would go here.",
		"Scan Models",
		"Models found by a scan can be linked or unlinked here. Shown as a table with a row per found model, listing details and operations.",
		"Available Models",
		"Model metadata, whether it is running or not, and start/stop controls. Shown as a table with a row per model, listing details and operations.",
	)
}

// backendsPage is the Backends screen: installed backends and, per backend,
// its list of running models.
func backendsPage() *fyne.Container {
	return page("Backends",
		"Installed backends, where each backend also shows a list of its running models, would go here.",
	)
}
