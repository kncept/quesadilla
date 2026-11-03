package gui

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/postfinance/single"

	qapp "github.com/kncept/quesadilla/app"
	jobs "github.com/kncept/quesadilla/app/jobs"
	tray "github.com/kncept/quesadilla/gui/tray"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
	"github.com/kncept/quesadilla/utils/qenv"
)

// SINGLE Q GUI
// DO NOT instantiate - use the 'Create' func
type QGUI struct {
	SysLink SystemAgnosticOperations
	QApp    *qapp.QApp

	// openURL opens a URL in the system browser when it is set; the search
	// screen's model-card buttons fall back to fyne.App.OpenURL otherwise.
	// It is a field so tests can capture the URL that would be opened.
	openURL func(*url.URL) error

	mu      sync.Mutex
	started bool
	app     fyne.App
	tray    *tray.QAppTray
	mainWin fyne.Window

	// logWindows holds the open log window for each running model, keyed by
	// model name, so opening a model's logs again focuses the existing window
	// instead of stacking up duplicates. Guarded by mu.
	logWindows map[string]fyne.Window

	// The pages whose content changes over time (running status, uptime,
	// the backend order); kept up to date by the status ticker in
	// [QGUI.Start]. Nil until the main window is built.
	modelsTable        *widget.Table
	runningModelsTable *widget.Table
	noRunningLabel     *widget.Label
	backendsTable      *widget.Table

	// Model Details screen widgets. The screen is laid out like Model Search:
	// a dropdown at the top, then a left menu ("Model", optionally "Download",
	// then one row per variant) beside the selected view.
	modelDetailsContent        *fyne.Container
	modelDetailsSelect         *widget.Select
	modelDetailsSelected       string
	modelDetailsList           *widget.List
	modelDetailsRows           []modelDetailRow
	modelDetailsModelView      *fyne.Container
	modelDetailsVariantView    *fyne.Container
	modelDetailsDownloadView   *fyne.Container
	modelDetailsModelScroll    *container.Scroll
	modelDetailsVariantScroll  *container.Scroll
	modelDetailsStack          *fyne.Container
	modelDetailsRemoteModel    string
	modelDetailsRemoteVariants []modelDefinitions.ModelVariant
	modelDetailsRemoteLoaded   bool
	modelDetailsRemoteLoading  bool
	modelDetailsRefreshButton  *widget.Button
	modelsRefreshAllButton     *widget.Button

	// Job management
	jobManager *jobs.JobManager
	jobsScreen *jobs.JobsScreen

	// Sidebar list for programmatic tab switching

	// Sidebar list for programmatic tab switching
	sidebarList *widget.List

	// The model-search screen's widgets and its last scan's results; the
	// search runs in the background and updates them when it finishes. Nil
	// until the main window is built. The filesystem scanner's configuration
	// form holds the chosen directory and depth, and the HuggingFace search
	// box holds the query for a scanner that supports searching.
	searchRunning          bool
	jobsInFlight           int
	searchScannersList     *widget.List
	searchSpinner          *widget.ProgressBarInfinite
	searchStatusLabel      *widget.Label
	searchOverview         *widget.Label
	searchResultsTable     *widget.Table
	searchResultsStack     *fyne.Container
	searchResults          []modelDefinitions.RemoteModel
	searchResultRows       []searchResultRow
	searchFilesystemForm   *fyne.Container
	searchDirectoryEntry   *widget.Entry
	searchDepthEntry       *widget.Entry
	searchUnlimitedCheck   *widget.Check
	searchFilesystemButton *widget.Button
	searchDepthLast        string
	searchQueryScanner     modelDefinitions.ModelScanner
	searchQueryBar         *fyne.Container
	searchQueryEntry       *widget.Entry
	searchQueryButton      *widget.Button
	searchModelTypeChecks  map[string]*widget.Check // model type -> checkbox
}

// Start launches the lightweight system-tray indicator. Blocks until quit.
// Panics if called more than once.
func (this *QGUI) Start() {
	this.mu.Lock() // Start needs slightly different lock handling
	if this.started {
		panic("a QGUI instance has already been started")
	}

	release, ok := acquireInstanceLock(qenv.QDir())
	if !ok {
		panic("a QGUI instance is already running")
	}
	this.started = true
	defer release()
	this.mu.Unlock() // Start

	this.app = app.NewWithID("com.github.kncept.quesadilla")

	if tray.SupportsTray(this.app) {
		this.tray = tray.InitializeTray(
			this.app,
			this.showMainWindow,
			this.QApp.RunningModelRegistry(),
			func(model runnerDefinitions.RunningModel) {
				this.showLogs(model)
			},
			this.Quit,
		)
	} else {
		// no system tray on this platform, fall back to the control suite window
		this.showMainWindow()
	}

	// Keep the dynamic pages (running status, uptime) up to date.
	this.jobManager.StartJob(
		"DynamicPages",
		"GUI",
		"Keep dynamic pages up to date",
		func() error {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				this.refreshDynamicPages()
				if this.tray != nil {
					this.tray.Refresh()
				}
			}
			return nil
		},
		nil,
		nil,
	)

	this.app.Run()
}

func (this *QGUI) Quit() {
	this.mu.Lock() // Quit

	this.QApp.RunningModelRegistry().StopAll()
	// this.jobManager.ActiveJobs() // stop all of these as well?

	// close tray and exit app
	defer this.app.Quit()

	if this.mainWin != nil {
		defer this.mainWin.Close() // this will acquire a lock, so it's deferred so the lock is free
	}
	defer this.mu.Unlock() // Quit - shouldn't every matter (!!)

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
	return &QGUI{
		SysLink:    SysLink,
		QApp:       qApp,
		jobManager: jobs.NewJobManager(),
	}
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
			fyne.Do(func() {
				this.runningModelsTable.Refresh()
				this.updateRunningModelsVisibility()
			})
		}
	})
}

// notifyError reports a problem to the user: an error dialog on the main
// window when it is open, plus a log line either way.
func (this *QGUI) notifyError(message string) {
	log.Printf("gui: %s", message)
	// this.mu.Lock()
	win := this.mainWin
	// this.mu.Unlock()
	if win != nil {
		dialog.NewInformation("Error", message, win).Show()
	}
}

// showMainWindow opens (or focuses) the main control suite window.
func (this *QGUI) showMainWindow() {
	this.mu.Lock()         // showMainWindow
	defer this.mu.Unlock() // showMainWindow
	if this.mainWin == nil {
		this.mainWin = this.app.NewWindow("Quesadilla Control Suite")
		// A closed window is destroyed by the driver and can never be
		// shown again (Show is a no-op on it), so drop the reference and
		// create a fresh window the next time the control suite is opened.
		this.mainWin.SetOnClosed(func() {
			this.mu.Lock()         // showMainWindow SetOnClosed
			defer this.mu.Unlock() // showMainWindow SetOnClosed
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
	sidebarItems := []string{"Overview", "Models", "Model Details", "Backends", "Model Search", "Jobs"}

	// One content page per sidebar item; only the selected page is shown.
	pages := container.NewStack(this.overviewPage(), this.modelsPage(), this.modelDetailsPage(), this.backendsPage(), this.modelSearchPage(), this.jobsPage())

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

	// Store the sidebar list for programmatic tab switching
	this.sidebarList = sidebar

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
