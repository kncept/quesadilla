package gui

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
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
	if err := s.Lock(); errors.Is(err, single.ErrAlreadyRunning) {
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
		this.mainWin.SetContent(widget.NewLabel("Quesadilla Control Suite"))
	}
	this.mainWin.Show()
}
