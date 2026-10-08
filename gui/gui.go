package gui

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/postfinance/single"

	qapp "github.com/kncept/quesadilla/app"
	"github.com/kncept/quesadilla/backend/running"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	"github.com/kncept/quesadilla/utils/qenv"
)

// SINGLE Q GUI
// DO NOT instantiate - use the 'Create' func
type QGUI struct {
	SysLink SystemAgnosticOperations
	QApp    *qapp.QApp

	mu      sync.Mutex
	started bool
	app     fyne.App
	tray    *fyne.Menu
	mainWin fyne.Window

	// The pages whose content changes over time (running status, uptime);
	// kept up to date by the status ticker in [QGUI.Start]. Nil until the
	// main window is built.
	modelsTable        *widget.Table
	runningModelsTable *widget.Table
	noRunningLabel     *widget.Label

	// The model-search screen's widgets and its last scan's results; the
	// search runs in the background and updates them when it finishes. Nil
	// until the main window is built.
	searchRunning      bool
	searchScannersList *widget.List
	searchSpinner      *widget.ProgressBarInfinite
	searchStatusLabel  *widget.Label
	searchOverview     *widget.Label
	searchResultsTable *widget.Table
	searchResults      []modelDefinitions.RemoteModel
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

	// Keep the dynamic pages (running status, uptime) up to date.
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			this.refreshDynamicPages()
		}
	}()

	this.app.Run()
}

func (this *QGUI) Quit() {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.mainWin != nil {
		this.mainWin.Close()
		this.mainWin = nil
	}

	this.QApp.StopAll()

	// close tray
	this.app.Quit()
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

func CreateGui(SysLink SystemAgnosticOperations, qApp *qapp.QApp) *QGUI {
	return &QGUI{SysLink: SysLink, QApp: qApp}
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
	quit := fyne.NewMenuItem("Quit", func() { this.Quit() })
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

// refreshDynamicPages re-renders the pages whose content changes over time:
// the installed-models table (running status and start/stop buttons) and the
// Overview's running-models section. Safe to call from any goroutine; widgets
// that are not attached to a window are no-ops.
func (this *QGUI) refreshDynamicPages() {
	fyne.Do(func() {
		// this.mu.Lock()
		// defer this.mu.Unlock()
		if this.modelsTable != nil {
			fyne.Do(func() { this.modelsTable.Refresh() })
		}
		if this.runningModelsTable != nil {
			fyne.Do(func() { this.runningModelsTable.Refresh() })
			this.updateRunningModelsVisibility()
		}
	})
}

// notifyError reports a problem to the user: an error dialog on the main
// window when it is open, plus a log line either way.
func (this *QGUI) notifyError(message string) {
	log.Printf("gui: %s", message)
	this.mu.Lock()
	win := this.mainWin
	this.mu.Unlock()
	if win != nil {
		dialog.NewInformation("Error", message, win).Show()
	}
}

// showMainWindow opens (or focuses) the main control suite window.
func (this *QGUI) showMainWindow() {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.mainWin == nil {
		this.mainWin = this.app.NewWindow("Quesadilla Control Suite")
		// A closed window is destroyed by the driver and can never be
		// shown again (Show is a no-op on it), so drop the reference and
		// create a fresh window the next time the control suite is opened.
		this.mainWin.SetOnClosed(func() {
			this.mu.Lock()
			defer this.mu.Unlock()
			this.mainWin = nil
		})
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
	sidebarItems := []string{"Overview", "Models", "Backends", "Model Search"}

	// One content page per sidebar item; only the selected page is shown.
	pages := container.NewStack(this.overviewPage(), this.modelsPage(), this.backendsPage(), this.modelSearchPage())

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

// headingLabel is a bold, non-wrapping label used for titles and headings.
func headingLabel(text string) *widget.Label {
	label := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	label.Wrapping = fyne.TextWrapOff
	return label
}

// wrappedLabel is a word-wrapping body label.
func wrappedLabel(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	return label
}

// overviewPage is the Overview screen: stats and the running models section,
// which lists every running model with stats (including uptime) and a stop
// button per model.
func (this *QGUI) overviewPage() fyne.CanvasObject {
	header := container.NewVBox(
		headingLabel("Overview"),
		wrappedLabel("Overview stats would go here."),
		widget.NewSeparator(),
		headingLabel("Running Models"),
		widget.NewSeparator(),
	)
	return container.NewPadded(container.NewBorder(header, nil, nil, nil, this.runningModelsContent()))
}
