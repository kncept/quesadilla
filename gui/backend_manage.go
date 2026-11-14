package gui

import (
	"fmt"
	"net/url"
	"runtime"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/quesadilla/app/jobs"
	backendDefinitions "github.com/kncept/quesadilla/backend/definitions"
)

// backendManager is the state behind one backend's Manage window: the
// installed and available runtime versions and the widgets that show them.
// Versions are fetched in the background, and installs run as jobs, so the
// window stays responsive while the network and disk are busy.
type backendManager struct {
	gui     *QGUI
	backend backendDefinitions.Backend
	win     fyne.Window

	installed []string
	available []string

	status           *widget.Label
	installedSummary *widget.Label
	availableSummary *widget.Label
	spinner          *widget.ProgressBarInfinite
	refreshButton    *widget.Button
	installLatest    *widget.Button
	openReleases     *widget.Button
	installedTable   *widget.Table
	availableTable   *widget.Table

	fetchingAvailable bool
	closed            bool
}

// newBackendManager builds the content of a backend's Manage window: a header
// with the install-latest and refresh actions, then the installed versions
// (each with a Remove button) above the versions available to install (each
// with an Install/Reinstall button). The available versions are fetched in the
// background when the window opens. onClosed (when set) runs once, after the
// window closes.
func newBackendManager(gui *QGUI, backend backendDefinitions.Backend, win fyne.Window, onClosed func()) (fyne.CanvasObject, *backendManager) {
	m := &backendManager{gui: gui, backend: backend, win: win}

	title := headingLabel("Manage " + backend.Id())
	description := wrappedLabel(fmt.Sprintf(
		"Install and remove %s runtime versions. Versions are downloaded for %s/%s and are ready to use once installed.",
		backend.Id(), runtime.GOOS, runtime.GOARCH,
	))

	m.status = wrappedLabel("Ready.")
	m.installedSummary = wrappedLabel("Loading installed versions...")
	m.availableSummary = wrappedLabel("Loading available versions...")

	m.installLatest = widget.NewButtonWithIcon("Install Latest", theme.DownloadIcon(), func() {
		m.installLatestVersion()
	})
	m.refreshButton = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		m.refreshAvailable()
	})
	m.openReleases = widget.NewButtonWithIcon("Open Releases", theme.NavigateNextIcon(), func() {
		m.openReleasesPage()
	})
	m.spinner = widget.NewProgressBarInfinite()
	m.spinner.Hide()

	m.installedTable = m.newInstalledTable()
	m.availableTable = m.newAvailableTable()

	installedSection := container.NewBorder(
		container.NewVBox(
			headingLabel("Installed Versions"),
			m.installedSummary,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		m.installedTable,
	)
	availableSection := container.NewBorder(
		container.NewVBox(
			headingLabel("Available Versions"),
			m.availableSummary,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		m.availableTable,
	)
	sections := container.NewVSplit(installedSection, availableSection)
	sections.SetOffset(0.35)

	content := container.NewPadded(container.NewBorder(
		container.NewVBox(
			title,
			description,
			container.NewHBox(m.installLatest, m.refreshButton, m.openReleases, m.spinner),
			m.status,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		sections,
	))

	win.SetOnClosed(func() {
		m.closed = true
		if onClosed != nil {
			onClosed()
		}
	})

	m.reloadInstalled()

	return content, m
}

// newInstalledTable builds the table of installed versions, with Remove
// and Open actions on each row.
func (m *backendManager) newInstalledTable() *widget.Table {
	table := widget.NewTable(
		func() (int, int) {
			return len(m.installed), 2
		},
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewButtonWithIcon("Remove", theme.DeleteIcon(), nil),
				widget.NewButtonWithIcon("Open", theme.NavigateNextIcon(), nil),
			)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			m.updateInstalledCell(id, cell.(*fyne.Container))
		},
	)
	table.ShowHeaderRow = true
	table.CreateHeader = func() fyne.CanvasObject {
		return widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	table.UpdateHeader = func(id widget.TableCellID, cell fyne.CanvasObject) {
		switch id.Col {
		case 0:
			cell.(*widget.Label).SetText("Version")
		case 1:
			cell.(*widget.Label).SetText("Action")
		}
	}
	table.SetColumnWidth(0, 420)
	table.SetColumnWidth(1, 200)
	return table
}

// newAvailableTable builds the table of installable versions, with an
// Install (or Reinstall) action and Open action on each row.
func (m *backendManager) newAvailableTable() *widget.Table {
	table := widget.NewTable(
		func() (int, int) {
			return len(m.available), 2
		},
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewButtonWithIcon("Install", theme.DownloadIcon(), nil),
				widget.NewButtonWithIcon("Open", theme.NavigateNextIcon(), nil),
			)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			m.updateAvailableCell(id, cell.(*fyne.Container))
		},
	)
	table.ShowHeaderRow = true
	table.CreateHeader = func() fyne.CanvasObject {
		return widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	table.UpdateHeader = func(id widget.TableCellID, cell fyne.CanvasObject) {
		switch id.Col {
		case 0:
			cell.(*widget.Label).SetText("Version")
		case 1:
			cell.(*widget.Label).SetText("Action")
		}
	}
	table.SetColumnWidth(0, 420)
	table.SetColumnWidth(1, 200)
	return table
}

// updateInstalledCell fills one cell of the installed-versions table.
func (m *backendManager) updateInstalledCell(id widget.TableCellID, cell *fyne.Container) {
	cell.RemoveAll()
	if id.Row < 0 || id.Row >= len(m.installed) {
		cell.Refresh()
		return
	}
	version := m.installed[id.Row]
	switch id.Col {
	case 0:
		label := widget.NewLabel(version)
		label.Wrapping = fyne.TextWrapOff
		cell.Add(label)
	case 1:
		cell.Add(widget.NewButtonWithIcon("Remove", theme.DeleteIcon(), func() {
			m.confirmRemoveVersion(version)
		}))
		cell.Add(widget.NewButtonWithIcon("Open", theme.NavigateNextIcon(), func() {
			m.openVersionReleasePage(version)
		}))
	}
	cell.Refresh()
}

// updateAvailableCell fills one cell of the available-versions table. A
// version that is already installed offers Reinstall; the rest offer Install.
// Both have an Open button to view the specific release on GitHub.
func (m *backendManager) updateAvailableCell(id widget.TableCellID, cell *fyne.Container) {
	cell.RemoveAll()
	if id.Row < 0 || id.Row >= len(m.available) {
		cell.Refresh()
		return
	}
	version := m.available[id.Row]
	switch id.Col {
	case 0:
		label := widget.NewLabel(version)
		label.Wrapping = fyne.TextWrapOff
		cell.Add(label)
	case 1:
		action := "Install"
		if m.isInstalled(version) {
			action = "Reinstall"
		}
		cell.Add(widget.NewButtonWithIcon(action, theme.DownloadIcon(), func() {
			m.installVersion(version)
		}))
		cell.Add(widget.NewButtonWithIcon("Open", theme.NavigateNextIcon(), func() {
			m.openVersionReleasePage(version)
		}))
	}
	cell.Refresh()
}

// isInstalled reports whether the given version is in the installed list.
func (m *backendManager) isInstalled(version string) bool {
	return slices.Contains(m.installed, version)
}

// reloadInstalled re-reads the backend's installed versions from disk and
// refreshes both tables (the available table's actions depend on what is
// installed).
func (m *backendManager) reloadInstalled() {
	versions, err := safeInstalledVersions(m.backend)
	if err != nil {
		m.installedSummary.SetText("Could not list installed versions: " + err.Error())
		return
	}
	m.installed = versions
	switch len(m.installed) {
	case 0:
		m.installedSummary.SetText("No versions installed.")
	case 1:
		m.installedSummary.SetText("1 version installed.")
	default:
		m.installedSummary.SetText(fmt.Sprintf("%d versions installed.", len(m.installed)))
	}
	m.installedTable.Refresh()
	m.availableTable.Refresh()
	if m.gui != nil && m.gui.backendsTable != nil {
		m.gui.backendsTable.Refresh()
	}
}

// updateAvailableSummary shows a message, or the current available-version
// count when message is empty.
func (m *backendManager) updateAvailableSummary(message string) {
	if message != "" {
		m.availableSummary.SetText(message)
		return
	}
	switch len(m.available) {
	case 0:
		m.availableSummary.SetText("No versions available.")
	case 1:
		m.availableSummary.SetText("1 version available.")
	default:
		m.availableSummary.SetText(fmt.Sprintf("%d versions available.", len(m.available)))
	}
}

// refreshAvailable fetches the backend's installable versions in the
// background and updates the available-versions table when they arrive. Only
// one fetch runs at a time.
func (m *backendManager) refreshAvailable() {
	if m.fetchingAvailable || m.closed {
		return
	}
	m.fetchingAvailable = true
	m.available = nil
	m.availableTable.Refresh()
	m.updateAvailableSummary("Fetching available versions...")
	m.refreshButton.Disable()
	m.spinner.Show()

	go func() {
		versions, err := safeInstallableVersions(m.backend)
		fyne.Do(func() {
			m.fetchingAvailable = false
			if m.closed {
				return
			}
			m.refreshButton.Enable()
			m.spinner.Hide()
			if err != nil {
				m.updateAvailableSummary("Could not list available versions: " + err.Error())
				return
			}
			m.available = versions
			m.availableTable.Refresh()
			m.updateAvailableSummary("")
		})
	}()
}

// installLatestVersion resolves the backend's latest version in the
// background, then installs it.
func (m *backendManager) installLatestVersion() {
	if m.closed {
		return
	}
	m.setStatus("Looking up the latest version...")
	m.installLatest.Disable()

	go func() {
		version, err := safeLatestVersion(m.backend)
		fyne.Do(func() {
			if m.closed {
				return
			}
			m.installLatest.Enable()
			if err != nil {
				m.setStatus("Could not find the latest version: " + err.Error())
				return
			}
			if version == "" {
				m.setStatus("Could not determine the latest version.")
				return
			}
			m.installVersion(version)
		})
	}()
}

// installVersion installs the given version as a background job, then
// refreshes the installed list. Reinstalling an installed version replaces it,
// like the CLI's install command.
func (m *backendManager) installVersion(version string) {
	if version == "" || m.closed {
		return
	}
	m.setStatus("Installing " + version + "...")
	err := m.gui.jobManager.StartJob(
		fmt.Sprintf("Install %s %s", m.backend.Id(), version),
		jobs.JobTypeDownload,
		fmt.Sprintf("Install %s version %s", m.backend.Id(), version),
		func() error {
			return safeInstallVersion(m.backend, version)
		},
		func(err error) {
			fyne.Do(func() {
				if m.closed {
					return
				}
				m.reloadInstalled()
				if err != nil {
					message := fmt.Sprintf("Installing %s %s failed: %v", m.backend.Id(), version, err)
					m.setStatus(message)
					m.gui.notifyError(message)
					return
				}
				m.setStatus("Installed " + version + ".")
			})
		},
		nil,
	)
	if err != nil {
		m.setStatus(fmt.Sprintf("Could not start installing %s: %v", version, err))
	}
}

// confirmRemoveVersion asks for confirmation before removing an installed
// version.
func (m *backendManager) confirmRemoveVersion(version string) {
	message := fmt.Sprintf("Remove %s version %s?", m.backend.Id(), version)
	dialog.NewConfirm("Remove Version", message, func(confirmed bool) {
		if confirmed {
			m.removeVersion(version)
		}
	}, m.win).Show()
}

// removeVersion removes an installed version and refreshes the installed list.
func (m *backendManager) removeVersion(version string) {
	m.setStatus("Removing " + version + "...")
	if err := safeRemoveVersion(m.backend, version); err != nil {
		message := fmt.Sprintf("Removing %s %s failed: %v", m.backend.Id(), version, err)
		m.setStatus(message)
		m.gui.notifyError(message)
		return
	}
	m.reloadInstalled()
	m.setStatus("Removed " + version + ".")
}

// setStatus updates the window's status line.
func (m *backendManager) setStatus(message string) {
	m.status.SetText(message)
}

// openReleasesPage opens the GitHub releases page for the backend in the browser.
func (m *backendManager) openReleasesPage() {
	var urlStr string
	switch m.backend.Id() {
	case "llama-binary":
		urlStr = "https://github.com/ggml-org/llama.cpp/releases"
	default:
		urlStr = "https://github.com/ggml-org/llama.cpp/releases" // fallback
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return
	}
	if m.gui.openURL != nil {
		_ = m.gui.openURL(u)
		return
	}
	if a := fyne.CurrentApp(); a != nil {
		_ = a.OpenURL(u)
	}
}

// openVersionReleasePage opens the specific release page for the given version.
func (m *backendManager) openVersionReleasePage(version string) {
	var urlStr string
	switch m.backend.Id() {
	case "llama-binary":
		urlStr = fmt.Sprintf("https://github.com/ggml-org/llama.cpp/releases/tag/%s", version)
	default:
		urlStr = fmt.Sprintf("https://github.com/ggml-org/llama.cpp/releases/tag/%s", version)
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return
	}
	if m.gui.openURL != nil {
		_ = m.gui.openURL(u)
		return
	}
	if a := fyne.CurrentApp(); a != nil {
		_ = a.OpenURL(u)
	}
}

// showBackendManager opens a backend's Manage window. Each backend has at most
// one Manage window: opening it again (from the Manage button) brings the
// existing window to the front instead of opening a second one. It returns the
// window (or nil if the app is not ready), which is handy for tests.
func (this *QGUI) showBackendManager(backend backendDefinitions.Backend) fyne.Window {
	if backend == nil {
		return nil
	}
	id := backend.Id()

	this.mu.Lock()
	app := this.app
	if win := this.manageWindows[id]; win != nil {
		this.mu.Unlock()
		win.Show()
		win.RequestFocus()
		return win
	}
	this.mu.Unlock()

	if app == nil {
		return nil
	}

	win := app.NewWindow("Manage " + id)
	content, manager := newBackendManager(this, backend, win, func() { this.forgetManageWindow(id, win) })
	win.SetContent(content)
	win.Resize(fyne.NewSize(760, 600))

	this.mu.Lock()
	if this.manageWindows == nil {
		this.manageWindows = map[string]fyne.Window{}
	}
	if this.manageManagers == nil {
		this.manageManagers = map[string]*backendManager{}
	}
	this.manageWindows[id] = win
	this.manageManagers[id] = manager
	this.mu.Unlock()

	win.Show()
	// Fetch the available versions once the window is built, so the first
	// render does not race the background refresh.
	manager.refreshAvailable()
	return win
}

// forgetManageWindow drops the reference to a backend's Manage window once it
// has closed, so opening it again creates a fresh window. It only clears the
// entry when it still points at win, so a window that closed after being
// replaced by a newer one does not evict the newer window.
func (this *QGUI) forgetManageWindow(id string, win fyne.Window) {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.manageWindows[id] == win {
		delete(this.manageWindows, id)
		delete(this.manageManagers, id)
	}
}

// The backend interface reports versions as plain values, so the GitHub-backed
// implementation reports network failures with a panic. The helpers below
// turn those panics into errors at the GUI boundary: a failed network call
// shows up in the Manage window instead of taking the whole application down.

func safeInstalledVersions(backend backendDefinitions.Backend) (versions []string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = backendCallError(backend, "listing installed versions", recovered)
		}
	}()
	return backend.InstalledVersions(), nil
}

func safeInstallableVersions(backend backendDefinitions.Backend) (versions []string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = backendCallError(backend, "listing installable versions", recovered)
		}
	}()
	return backend.InstallableVersions(), nil
}

func safeLatestVersion(backend backendDefinitions.Backend) (version string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = backendCallError(backend, "finding the latest version", recovered)
		}
	}()
	return backend.LatestVersion(), nil
}

func safeInstallVersion(backend backendDefinitions.Backend, version string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = backendCallError(backend, "installing "+version, recovered)
		}
	}()
	// Mirror the CLI: replace an existing install rather than writing over it.
	if slices.Contains(backend.InstalledVersions(), version) {
		if err := backend.RemoveVersion(version); err != nil {
			return err
		}
	}
	return backend.InstallVersion(version)
}

func safeRemoveVersion(backend backendDefinitions.Backend, version string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = backendCallError(backend, "removing "+version, recovered)
		}
	}()
	return backend.RemoveVersion(version)
}

func backendCallError(backend backendDefinitions.Backend, action string, recovered any) error {
	return fmt.Errorf("%s: %s: %v", backend.Id(), action, recovered)
}
