package gui

import (
	"fmt"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	"github.com/kncept/quesadilla/model/remoterepository"
)

// modelSearchColumns are the table headers shown for the model search
// results. The first column holds each model's action buttons.
var modelSearchColumns = []string{"Action", "Name", "Type", "File"}

// modelSearchOverviewText is what the search screen's overview explains:
// that the screen is for finding models to install.
const modelSearchOverviewText = `This screen is for finding models to install.

Pick a scanner on the left to search for models installed or hosted elsewhere. Each scanner looks in a different source, and selecting one runs a search and lists what it finds here.

Selecting a scanner again re-runs its search.`

// modelSearchPage renders the Model Search screen: an Overview plus the
// available model scanners as a list on the left, and the results of the
// last scan as a table on the right. Selecting a scanner runs its scan,
// showing a spinner while it is active, and then lists what it found.
func (this *QGUI) modelSearchPage() fyne.CanvasObject {
	header := container.NewVBox(
		headingLabel("Model Search"),
		wrappedLabel("Find models to install, from the sources below."),
		widget.NewSeparator(),
	)

	scanners := this.QApp.RemoteModels.Scanners()
	rowNames := make([]string, 0, len(scanners)+1)
	rowNames = append(rowNames, "Overview")
	for _, s := range scanners {
		rowNames = append(rowNames, s.ScannerName())
	}

	// The list sizes itself to its template item, so use the longest row
	// name (padded for breathing room) as the template to guarantee the
	// column is wide enough for every row.
	longest := "Overview"
	for _, name := range rowNames {
		if len(name) > len(longest) {
			longest = name
		}
	}

	this.searchScannersList = widget.NewList(
		func() int {
			return len(rowNames)
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel(longest)
			label.Wrapping = fyne.TextWrapOff
			return container.NewPadded(label)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*fyne.Container).Objects[0].(*widget.Label).SetText(rowNames[id])
		},
	)
	this.searchScannersList.OnSelected = func(id widget.ListItemID) {
		if id == 0 { // the overview
			this.showModelSearchOverview()
			return
		}
		this.selectModelScanner(scanners[id-1])
	}

	// the search status: a spinner while a scan is active, and a label with
	// the current state (searching, done, failed)
	this.searchSpinner = widget.NewProgressBarInfinite()
	this.searchStatusLabel = widget.NewLabel("")
	this.searchSpinner.Hide()
	this.searchStatusLabel.Hide()

	// the results area: the overview text, the last scan's results, or (once
	// picked) the filesystem scanner's configuration form
	this.searchOverview = widget.NewLabel(modelSearchOverviewText)
	this.searchOverview.Wrapping = fyne.TextWrapWord
	this.searchResultsTable = newSearchResultsTable(this)
	this.searchResultsTable.Hide()

	this.searchResultsStack = container.NewStack(this.searchOverview, this.searchResultsTable)

	right := container.NewBorder(
		container.NewHBox(this.searchSpinner, this.searchStatusLabel), // top: search status
		nil, nil, nil,
		this.searchResultsStack,
	)

	left := container.NewPadded(this.searchScannersList)
	content := container.NewPadded(container.NewBorder(
		header, // top
		nil,    // bottom
		left,   // leading (left): the overview and the scanners
		nil,    // trailing (right)
		right,  // center: the results
	))

	// start on the overview
	this.searchScannersList.Select(0)
	return content
}

// newSearchResultsTable builds the table of the last scan's results: the
// model's action buttons in the first column, then the model's name, type and
// file.
func newSearchResultsTable(g *QGUI) *widget.Table {
	table := widget.NewTable(
		func() (int, int) {
			return len(g.searchResults), len(modelSearchColumns)
		},
		searchResultsCellFactory,
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			if id.Row < 0 || id.Row >= len(g.searchResults) {
				return
			}
			g.updateSearchResultCell(g.searchResults[id.Row], id, cell.(*fyne.Container))
		},
	)

	table.ShowHeaderRow = true
	table.CreateHeader = func() fyne.CanvasObject {
		return widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	table.UpdateHeader = func(id widget.TableCellID, cell fyne.CanvasObject) {
		if id.Col >= 0 && id.Col < len(modelSearchColumns) {
			cell.(*widget.Label).SetText(modelSearchColumns[id.Col])
		}
	}

	table.SetColumnWidth(0, 220) // Action
	table.SetColumnWidth(1, 210) // Name
	table.SetColumnWidth(2, 80)  // Type
	table.SetColumnWidth(3, 320) // File

	return table
}

// searchResultsCellFactory builds a cell of the search-results table. The
// cell needs a real layout: a bare &fyne.Container{} never lays out its
// children, which leaves them at 0x0 - invisible and untappable. It also
// needs a minimum size that fits the widest row's action buttons, because the
// table sizes its rows from the template cell's minimum size; the seeded
// buttons are only measured and never rendered (updateSearchResultCell
// replaces the contents).
func searchResultsCellFactory() fyne.CanvasObject {
	return container.NewHBox(
		widget.NewButtonWithIcon("Link", theme.ContentCopyIcon(), nil),
		widget.NewButtonWithIcon("Install", theme.DownloadIcon(), nil),
	)
}

// updateSearchResultCell fills one cell of the search-results table: the
// model's actions in the first column, and a label for the data columns.
//
// The actions differ by where the model came from:
//
//   - the filesystem scanner lists local files that can be linked into local
//     storage or imported (copied) into it, so its rows carry both buttons;
//   - the LocalAI scanner lists another application's local files, so its
//     rows follow the link/install state machine (see addLocalScannerActions):
//     a new model can be linked or installed, a linked one unlinked or
//     installed, and an installed one uninstalled;
//   - the remaining scanners list models to download, so their rows carry a
//     download (or re-download) button, plus a button opening the model's
//     page at its source when the scanner knows of one.
func (this *QGUI) updateSearchResultCell(m modelDefinitions.RemoteModel, id widget.TableCellID, cell *fyne.Container) {
	cell.RemoveAll()
	switch {
	case id.Col != 0:
		label := widget.NewLabel(searchResultCell(m, id.Col))
		label.Wrapping = fyne.TextWrapOff
		cell.Add(label)
	case m.ModelSource == remoterepository.FilesystemScannerName:
		cell.Add(this.modelLinkButton(m))
		cell.Add(this.modelImportButton(m))
	case m.ModelSource == remoterepository.LocalAiScannerName:
		this.addLocalScannerActions(m, cell)
	default:
		cell.Add(this.modelDownloadButton(m))
		if m.ModelURL != "" {
			cell.Add(this.modelCardButton(m))
		}
	}
	cell.Refresh()
}

// addLocalScannerActions fills the action cell for a model found by a scanner
// whose models are local files owned by another application (LocalAI). A model
// that is not installed locally can be linked into local storage or installed
// (copied into it); a linked model can be unlinked, or installed to turn the
// link into a local copy; an installed model can only be uninstalled.
func (this *QGUI) addLocalScannerActions(m modelDefinitions.RemoteModel, cell *fyne.Container) {
	local := this.QApp.LocalModels.GetModel(m.FullModelName())
	switch {
	case local == nil:
		cell.Add(this.modelLinkButton(m))
		cell.Add(this.modelInstallButton(m))
	case local.IsLinked():
		cell.Add(this.modelUnlinkButton(*local))
		cell.Add(this.modelInstallButton(m))
	default:
		cell.Add(this.modelUninstallButton(*local))
	}
}

// modelLinkButton builds the "Link" button for a filesystem search result's
// row: it links the model into local storage without moving its files.
func (this *QGUI) modelLinkButton(m modelDefinitions.RemoteModel) *widget.Button {
	return widget.NewButtonWithIcon("Link", theme.ContentCopyIcon(), func() {
		this.linkModel(m)
	})
}

// modelImportButton builds the "Import" button for a filesystem search
// result's row: it copies the model's files into local storage.
func (this *QGUI) modelImportButton(m modelDefinitions.RemoteModel) *widget.Button {
	return widget.NewButtonWithIcon("Import", theme.DownloadIcon(), func() {
		this.importModel(m)
	})
}

// modelInstallButton builds the "Install" button for a model found as local
// files (LocalAI): it copies the model's files into local storage, turning a
// new model or an existing link into a local copy.
func (this *QGUI) modelInstallButton(m modelDefinitions.RemoteModel) *widget.Button {
	return widget.NewButtonWithIcon("Install", theme.DownloadIcon(), func() {
		this.installModel(m, installInstall)
	})
}

// modelUnlinkButton builds the "Unlink" button for a linked LocalAI model: it
// drops the model from local storage without touching the source files.
func (this *QGUI) modelUnlinkButton(m modelDefinitions.Model) *widget.Button {
	return widget.NewButtonWithIcon("Unlink", theme.DeleteIcon(), func() {
		this.removeLocalModel(m, installUnlink)
	})
}

// modelUninstallButton builds the "Uninstall" button for a LocalAI model whose
// files are stored locally: it removes the model and its files.
func (this *QGUI) modelUninstallButton(m modelDefinitions.Model) *widget.Button {
	return widget.NewButtonWithIcon("Uninstall", theme.DeleteIcon(), func() {
		this.removeLocalModel(m, installUninstall)
	})
}

// modelCardButton builds the button, to the right of a remote model's download
// button, that opens the model's page (eg its card on a model hub) in the
// system browser.
func (this *QGUI) modelCardButton(m modelDefinitions.RemoteModel) *widget.Button {
	return widget.NewButtonWithIcon("Open", theme.ComputerIcon(), func() {
		this.openModelCard(m.ModelURL)
	})
}

// openModelCard opens the model's page in the system browser. The URL is
// routed through the openURL hook when set (the tests use it), and otherwise
// through the app, which launches the system browser.
func (this *QGUI) openModelCard(rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		this.notifyError(fmt.Sprintf("opening the model card: %v", err))
		return
	}
	if this.openURL != nil {
		_ = this.openURL(u)
		return
	}
	if a := fyne.CurrentApp(); a != nil {
		_ = a.OpenURL(u)
	}
}

// modelDownloadButton builds the download button for a search result's row:
// "Download" when the model is not installed locally yet, and "Redownload"
// when it already is (matched by name).
func (this *QGUI) modelDownloadButton(m modelDefinitions.RemoteModel) *widget.Button {
	if this.QApp.LocalModels.GetModel(m.FullModelName()) != nil {
		return widget.NewButtonWithIcon("Redownload", theme.ViewRefreshIcon(), func() {
			this.downloadModel(m)
		})
	}
	return widget.NewButtonWithIcon("Download", theme.DownloadIcon(), func() {
		this.downloadModel(m)
	})
}

// searchResultCell returns the display value for one data cell of the
// search-results table. Column 0 holds the download button, not a label. The
// file column shows the model's first variant (a model's variants are the
// individual files it can be run from).
func searchResultCell(m modelDefinitions.RemoteModel, col int) string {
	switch col {
	case 1:
		return m.FullModelName()
	case 2:
		return m.ModelType
	case 3:
		if len(m.Variants) > 0 {
			return m.Variants[0].FileName
		}
		return ""
	default:
		return ""
	}
}

// showModelSearchOverview shows the overview text in the results area.
func (this *QGUI) showModelSearchOverview() {
	this.searchOverview.Show()
	this.searchResultsTable.Hide()
	if this.searchFilesystemForm != nil {
		this.searchFilesystemForm.Hide()
	}
}

// selectModelScanner handles picking a scanner from the list. The filesystem
// scanner cannot scan until the user supplies a directory, so it shows its
// configuration form instead; every other scanner runs its scan immediately.
func (this *QGUI) selectModelScanner(scanner modelDefinitions.ModelScanner) {
	if _, ok := scanner.(*remoterepository.FilesystemScanner); ok {
		this.showFilesystemSearchForm()
		return
	}
	this.runModelSearch(scanner)
}

// runModelSearch runs the named scanner's scan in the background: the
// spinner is shown while it is active, and the results (or the error) are
// listed when it finishes. Picking a scanner while a scan is in progress is
// ignored.
func (this *QGUI) runModelSearch(scanner modelDefinitions.ModelScanner) {
	if this.searchRunning {
		return
	}
	this.searchRunning = true
	this.searchSpinner.Show()
	this.searchStatusLabel.Show()
	this.searchStatusLabel.SetText(fmt.Sprintf("Searching with %s...", scanner.ScannerName()))
	if this.searchFilesystemForm != nil {
		this.searchFilesystemForm.Hide()
	}

	go func() {
		models, err := scanner.ScanForModels()
		// the screen is the source of truth for where its results came from,
		// so stamp the scanner name on every result; the download action
		// looks the scanner back up by that name
		for i := range models {
			models[i].ModelSource = scanner.ScannerName()
		}
		fyne.Do(func() {
			this.finishModelSearch(models, err)
		})
	}()
}

// finishModelSearch applies the outcome of a scan to the screen: it hides
// the spinner and shows the results (or the error and the overview) in the
// results area. Last it clears the scanner selection and marks the search as
// finished, so the same scanner can be picked again to re-run a scan.
func (this *QGUI) finishModelSearch(models []modelDefinitions.RemoteModel, err error) {
	// the search and the downloads share the spinner, so only hide it when
	// no download is still in progress
	if this.downloadsInFlight == 0 {
		this.searchSpinner.Hide()
	}
	if this.searchFilesystemForm != nil {
		this.searchFilesystemForm.Hide()
	}

	if err != nil {
		this.searchResults = nil
		this.showModelSearchOverview()
		this.searchStatusLabel.SetText(fmt.Sprintf("Search failed: %v", err))
	} else if len(models) == 0 {
		this.searchResults = models
		this.showModelSearchOverview()
		this.searchStatusLabel.SetText("No models found")
	} else {
		this.searchResults = models
		if len(models) == 1 {
			this.searchStatusLabel.SetText("1 model found")
		} else {
			this.searchStatusLabel.SetText(fmt.Sprintf("%d models found", len(models)))
		}
		this.searchOverview.Hide()
		this.searchResultsTable.Show()
		this.searchResultsTable.Refresh()
	}

	// lastly release the scanner selection and mark the search finished
	this.searchScannersList.UnselectAll()
	this.searchRunning = false
}

// modelInstallAction names what acting on a scanned model will do, so the
// status messages ("Downloading...", "Downloaded ...", "Download failed: ...")
// can be built for each action.
type modelInstallAction struct {
	// verb is the action's present-tense name: "Download", "Import", "Link",
	// "Install", "Unlink" or "Uninstall".
	verb string
	// link installs the model by pointing at its source files, rather than
	// copying them into local storage.
	link bool
}

var (
	installDownload  = modelInstallAction{verb: "Download"}
	installImport    = modelInstallAction{verb: "Import"}
	installInstall   = modelInstallAction{verb: "Install"}
	installLink      = modelInstallAction{verb: "Link", link: true}
	installUnlink    = modelInstallAction{verb: "Unlink"}
	installUninstall = modelInstallAction{verb: "Uninstall"}
)

// downloadModel downloads (or copies, for local sources) the named scanned
// model into local storage. The work runs in the background, with the spinner
// and the status label showing progress; when it finishes the search-results
// table and the installed-models table are refreshed, so the button reads
// "Redownload" and the model shows up on the Models screen.
func (this *QGUI) downloadModel(m modelDefinitions.RemoteModel) {
	this.installModel(m, installDownload)
}

// importModel copies the named scanned model's files into local storage, the
// same way downloadModel does for models that are not local files.
func (this *QGUI) importModel(m modelDefinitions.RemoteModel) {
	this.installModel(m, installImport)
}

// linkModel links the named scanned model into local storage: only its
// metadata is stored, and its variants keep pointing at the source files.
func (this *QGUI) linkModel(m modelDefinitions.RemoteModel) {
	this.installModel(m, installLink)
}

// installModel installs the named scanned model according to action: it
// re-resolves the model through its scanner and then either links it or
// downloads (copies) it. The work runs in the background, with the spinner
// and the status label showing progress.
func (this *QGUI) installModel(m modelDefinitions.RemoteModel, action modelInstallAction) {
	this.downloadsInFlight++
	this.searchSpinner.Show()
	this.searchStatusLabel.Show()
	this.searchStatusLabel.SetText(fmt.Sprintf("%sing %s...", action.verb, m.FullModelName()))

	go func() {
		var err error
		scanner := this.QApp.RemoteModels.GetScanner(m.ModelSource)
		if scanner == nil {
			err = fmt.Errorf("scanner %s is not available", m.ModelSource)
		} else {
			remote := scanner.GetModel(m.FullModelName())
			if remote == nil {
				err = fmt.Errorf("%s model %s not found", m.ModelSource, m.FullModelName())
			} else if action.link {
				err = this.QApp.LocalModels.LinkScannedModel(remote)
			} else {
				err = this.QApp.LocalModels.DownloadModel(remote)
			}
		}
		fyne.Do(func() {
			this.finishModelAction(action, m.FullModelName(), err)
		})
	}()
}

// removeLocalModel drops an already-installed (or linked) local model,
// according to action: unlinking keeps the source files and only removes the
// stored link, while uninstalling removes the model's local files too. The
// work runs in the background, with the spinner and the status label showing
// progress.
func (this *QGUI) removeLocalModel(m modelDefinitions.Model, action modelInstallAction) {
	this.downloadsInFlight++
	this.searchSpinner.Show()
	this.searchStatusLabel.Show()
	this.searchStatusLabel.SetText(fmt.Sprintf("%sing %s...", action.verb, m.FullModelName()))

	go func() {
		err := this.QApp.LocalModels.RemoveModel(m.FullModelName())
		fyne.Do(func() {
			this.finishModelAction(action, m.FullModelName(), err)
		})
	}()
}

// finishModelAction applies the outcome of an install or removal to the
// screen: the status label, and a refresh of the search-results table (so the
// row's buttons reflect the new state) and of the installed-models table (so
// the model appears on the Models screen).
func (this *QGUI) finishModelAction(action modelInstallAction, modelName string, err error) {
	if this.downloadsInFlight > 0 {
		this.downloadsInFlight--
	}
	if this.downloadsInFlight == 0 {
		this.searchSpinner.Hide()
	}
	if err != nil {
		this.searchStatusLabel.SetText(fmt.Sprintf("%s failed: %v", action.verb, err))
		this.notifyError(fmt.Sprintf("%sing %s: %v", strings.ToLower(action.verb), modelName, err))
	} else {
		this.searchStatusLabel.SetText(fmt.Sprintf("%sed %s", action.verb, modelName))
	}
	this.searchResultsTable.Refresh()
	if this.modelsTable != nil {
		this.modelsTable.Refresh()
	}
}
