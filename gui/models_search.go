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
// results: the model's name and type, then its action buttons (between the
// type and the file, so the buttons sit next to the details they act on), and
// finally the file.
var modelSearchColumns = []string{"Name", "Type", "Action", "File"}

// The search-results table's column indices, in [modelSearchColumns] order.
const (
	searchResultNameColumn = iota
	searchResultTypeColumn
	searchResultActionColumn
	searchResultFileColumn
)

// searchResultRow is one row of the search-results table. A model whose
// variants are listed individually (a HuggingFace repository holding several
// GGUF files) becomes one row per variant, so a specific quantization can be
// downloaded; the model's name and type are shown only on the group's first
// row, so it reads as one model. A row with variant == -1 stands for the model
// as a whole (the scanners that list one row per model).
type searchResultRow struct {
	model   modelDefinitions.RemoteModel
	variant int
}

// showsModel reports whether the row shows the model's name and type: only the
// first row of the model's group does, so a multi-file model is not repeated.
func (r searchResultRow) showsModel() bool {
	return r.variant <= 0
}

// variantFile returns the file name this row stands for: its own variant when
// it has one, otherwise the model's first variant.
func (r searchResultRow) variantFile() string {
	if r.variant >= 0 && r.variant < len(r.model.Variants) {
		return r.model.Variants[r.variant].FileName
	}
	if len(r.model.Variants) > 0 {
		return r.model.Variants[0].FileName
	}
	return ""
}

// buildSearchResultRows flattens the scanned models into the table's rows.
// Models from scanners that list their files individually (HuggingFace, so a
// specific GGUF quantization can be picked) become one row per variant; every
// other model becomes a single whole-model row.
func buildSearchResultRows(models []modelDefinitions.RemoteModel) []searchResultRow {
	rows := make([]searchResultRow, 0, len(models))
	for _, m := range models {
		if variantsAreRows(m) {
			for i := range m.Variants {
				rows = append(rows, searchResultRow{model: m, variant: i})
			}
			continue
		}
		rows = append(rows, searchResultRow{model: m, variant: -1})
	}
	return rows
}

// variantsAreRows reports whether each of the model's variants gets its own
// row. The HuggingFace scanner finds repositories that usually hold several
// GGUF quantizations of one model, so its files are listed individually; the
// other scanners group a model's files into a single row.
func variantsAreRows(m modelDefinitions.RemoteModel) bool {
	return m.ModelSource == remoterepository.HuggingFaceScannerName && len(m.Variants) > 0
}

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
			this.searchQueryScanner = nil
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

	// The search box for a searchable scanner (HuggingFace). It is built
	// lazily - when a searchable scanner is picked - so it adds no hidden
	// entry and button widgets to the page until it is actually needed.
	this.searchQueryBar = container.NewVBox()
	this.searchQueryBar.Hide()

	right := container.NewBorder(
		container.NewVBox(
			container.NewHBox(this.searchSpinner, this.searchStatusLabel), // top: search status
			this.searchQueryBar, // top: the searchable scanner's search box
		),
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
// model's name and type, then its action buttons, then the file.
func newSearchResultsTable(g *QGUI) *widget.Table {
	table := widget.NewTable(
		func() (int, int) {
			return len(g.searchResultRows), len(modelSearchColumns)
		},
		searchResultsCellFactory,
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			if id.Row < 0 || id.Row >= len(g.searchResultRows) {
				return
			}
			g.updateSearchResultCell(g.searchResultRows[id.Row], id, cell.(*fyne.Container))
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

	table.SetColumnWidth(searchResultNameColumn, 210)   // Name
	table.SetColumnWidth(searchResultTypeColumn, 80)    // Type
	table.SetColumnWidth(searchResultActionColumn, 220) // Action
	table.SetColumnWidth(searchResultFileColumn, 320)   // File

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
// model's actions in the action column, and a label for the other columns.
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
func (this *QGUI) updateSearchResultCell(row searchResultRow, id widget.TableCellID, cell *fyne.Container) {
	cell.RemoveAll()
	if id.Col == searchResultActionColumn {
		this.addSearchResultActions(row, cell)
		cell.Refresh()
		return
	}

	label := widget.NewLabel(searchResultCellText(row, id.Col))
	label.Wrapping = fyne.TextWrapOff
	cell.Add(label)
	cell.Refresh()
}

// searchResultCellText returns the display value for one data cell of the
// search-results table. The name and type are shown only on the first row of
// a model's group (so a multi-file model reads as one model), while the file
// column shows the row's own file.
func searchResultCellText(row searchResultRow, col int) string {
	m := row.model
	switch col {
	case searchResultNameColumn:
		if row.showsModel() {
			return m.FullModelName()
		}
	case searchResultTypeColumn:
		if row.showsModel() {
			return m.ModelType
		}
	case searchResultFileColumn:
		return row.variantFile()
	}
	return ""
}

// addSearchResultActions fills the action cell for a search result's row: the
// buttons that install the row's model (or, for a searchable scanner, the
// specific variant the row lists) into local storage.
func (this *QGUI) addSearchResultActions(row searchResultRow, cell *fyne.Container) {
	m := row.model
	switch {
	case m.ModelSource == remoterepository.FilesystemScannerName:
		cell.Add(this.modelLinkButton(m))
		cell.Add(this.modelImportButton(m))
	case m.ModelSource == remoterepository.LocalAiScannerName:
		this.addLocalScannerActions(m, cell)
	default:
		cell.Add(this.modelDownloadButton(row))
		if m.ModelURL != "" {
			cell.Add(this.modelCardButton(m))
		}
	}
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
		this.installModel(m, "", installInstall)
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
// "Download" when the row's file is not installed locally yet, and
// "Redownload" when it already is.
func (this *QGUI) modelDownloadButton(row searchResultRow) *widget.Button {
	if this.variantInstalled(row) {
		return widget.NewButtonWithIcon("Redownload", theme.ViewRefreshIcon(), func() {
			this.downloadModelRow(row)
		})
	}
	return widget.NewButtonWithIcon("Download", theme.DownloadIcon(), func() {
		this.downloadModelRow(row)
	})
}

// variantInstalled reports whether the row's file is already installed
// locally: a whole-model row counts as installed when the model is present,
// and a variant row when the model holds a variant with that file name.
func (this *QGUI) variantInstalled(row searchResultRow) bool {
	local := this.QApp.LocalModels.GetModel(row.model.FullModelName())
	if local == nil {
		return false
	}
	if row.variant < 0 {
		return true
	}
	fileName := row.variantFile()
	for _, v := range local.Variants {
		if v.FileName == fileName {
			return true
		}
	}
	return false
}

// downloadModelRow downloads the row's file into local storage: the single
// variant when the row stands for one file (so a specific HuggingFace
// quantization can be picked), or the whole model when the row stands for all
// of it.
func (this *QGUI) downloadModelRow(row searchResultRow) {
	variantFile := ""
	if row.variant >= 0 {
		variantFile = row.variantFile()
	}
	this.installModel(row.model, variantFile, installDownload)
}

// showModelSearchOverview shows the overview text in the results area.
func (this *QGUI) showModelSearchOverview() {
	this.searchOverview.Show()
	this.searchResultsTable.Hide()
	if this.searchFilesystemForm != nil {
		this.searchFilesystemForm.Hide()
	}
	this.setSearchQueryBarVisible(false)
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

// runModelSearch runs the named scanner's default scan (see
// [QGUI.runModelSearchQuery]).
func (this *QGUI) runModelSearch(scanner modelDefinitions.ModelScanner) {
	this.runModelSearchQuery(scanner, "")
}

// runModelSearchQuery runs a scanner's scan in the background: the spinner is
// shown while it is active, and the results (or the error) are listed when it
// finishes. A non-empty query is used when the scanner supports searching
// (see [definitions.QueryModelScanner]); such scanners also get the search box
// above their results. Picking a scanner while a scan is in progress is
// ignored.
func (this *QGUI) runModelSearchQuery(scanner modelDefinitions.ModelScanner, query string) {
	if this.searchRunning {
		return
	}
	this.searchRunning = true

	// a searchable scanner (HuggingFace) shows the search box above its
	// results; the other scanners keep it hidden
	if queryable, ok := scanner.(modelDefinitions.QueryModelScanner); ok {
		this.searchQueryScanner = queryable
	} else {
		this.searchQueryScanner = nil
	}
	this.setSearchQueryBarVisible(this.searchQueryScanner != nil)

	this.searchSpinner.Show()
	this.searchStatusLabel.Show()
	if query != "" {
		this.searchStatusLabel.SetText(fmt.Sprintf("Searching %s for %q...", scanner.ScannerName(), query))
	} else {
		this.searchStatusLabel.SetText(fmt.Sprintf("Searching with %s...", scanner.ScannerName()))
	}
	if this.searchFilesystemForm != nil {
		this.searchFilesystemForm.Hide()
	}

	go func() {
		var models []modelDefinitions.RemoteModel
		var err error
		if query != "" {
			if queryable, ok := scanner.(modelDefinitions.QueryModelScanner); ok {
				models, err = queryable.ScanForQuery(query)
			} else {
				models, err = scanner.ScanForModels()
			}
		} else {
			models, err = scanner.ScanForModels()
		}
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

// runHuggingFaceSearch runs the scanner behind the search box with the query
// the user typed (an empty query lists the scanner's default models), filtered
// by the selected model types.
func (this *QGUI) runHuggingFaceSearch() {
	scanner := this.searchQueryScanner
	if scanner == nil {
		scanner = this.QApp.RemoteModels.GetScanner(remoterepository.HuggingFaceScannerName)
	}
	if scanner == nil {
		this.searchStatusLabel.Show()
		this.searchStatusLabel.SetText("HuggingFace scanner is not available")
		return
	}

	// Collect selected model types from checkboxes
	var selectedTypes []string
	for modelType, check := range this.searchModelTypeChecks {
		if check.Checked {
			selectedTypes = append(selectedTypes, modelType)
		}
	}

	// Use the HuggingFace scanner's type-filtered search method
	if hfScanner, ok := scanner.(*remoterepository.HuggingFaceScanner); ok {
		query := strings.TrimSpace(this.searchQueryEntry.Text)
		this.runHuggingFaceSearchWithTypes(hfScanner, query, selectedTypes)
	} else {
		// Fallback for other queryable scanners
		this.runModelSearchQuery(scanner, strings.TrimSpace(this.searchQueryEntry.Text))
	}
}

// runHuggingFaceSearchWithTypes runs the HuggingFace scanner with the given
// query and model type filters.
func (this *QGUI) runHuggingFaceSearchWithTypes(scanner *remoterepository.HuggingFaceScanner, query string, modelTypes []string) {
	if this.searchRunning {
		return
	}
	this.searchRunning = true

	this.searchQueryScanner = scanner
	this.setSearchQueryBarVisible(true)

	this.searchSpinner.Show()
	this.searchStatusLabel.Show()
	if query != "" {
		this.searchStatusLabel.SetText(fmt.Sprintf("Searching %s for %q (types: %v)...", scanner.ScannerName(), query, modelTypes))
	} else {
		this.searchStatusLabel.SetText(fmt.Sprintf("Searching with %s (types: %v)...", scanner.ScannerName(), modelTypes))
	}
	if this.searchFilesystemForm != nil {
		this.searchFilesystemForm.Hide()
	}

	go func() {
		models, err := scanner.ScanForQueryWithTypes(query, modelTypes)
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

// newSearchQueryBar builds the search box shown above a searchable scanner's
// results: a text entry whose submissions run the search, a Search button,
// and checkboxes for filtering by model type (driven from the backends we support).
func (this *QGUI) newSearchQueryBar() {
	this.searchQueryEntry = widget.NewEntry()
	this.searchQueryEntry.SetPlaceHolder("Search HuggingFace models...")
	this.searchQueryEntry.OnSubmitted = func(string) {
		this.runHuggingFaceSearch()
	}
	this.searchQueryButton = widget.NewButtonWithIcon("Search", theme.SearchIcon(), func() {
		this.runHuggingFaceSearch()
	})

	// Build model type filter checkboxes from supported model types
	this.searchModelTypeChecks = make(map[string]*widget.Check)
	supportedTypes := this.QApp.SupportedModelTypes()
	typeCheckBoxes := make([]fyne.CanvasObject, 0, len(supportedTypes))
	for _, modelType := range supportedTypes {
		check := widget.NewCheck(modelType, nil)
		check.SetChecked(true) // default to all checked
		this.searchModelTypeChecks[modelType] = check
		typeCheckBoxes = append(typeCheckBoxes, check)
	}

	// Create a container for the model type filters
	modelTypeFilterBox := container.NewVBox(
		widget.NewLabelWithStyle("Model Type Filter", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(typeCheckBoxes...),
	)

	// Add the search entry + button, and the model type filters
	this.searchQueryBar.Add(container.NewBorder(nil, nil, nil, this.searchQueryButton, this.searchQueryEntry))
	this.searchQueryBar.Add(modelTypeFilterBox)
}

// setSearchQueryBarVisible shows or hides the search box, building it the
// first time it is shown so the page carries no hidden search widgets until a
// searchable scanner is picked.
func (this *QGUI) setSearchQueryBarVisible(visible bool) {
	if this.searchQueryBar == nil {
		return
	}
	if !visible {
		this.searchQueryBar.Hide()
		return
	}
	if this.searchQueryEntry == nil {
		this.newSearchQueryBar()
	}
	this.searchQueryBar.Show()
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
		this.searchResultRows = nil
		this.showModelSearchOverview()
		this.searchStatusLabel.SetText(fmt.Sprintf("Search failed: %v", err))
	} else if len(models) == 0 {
		this.searchResults = nil
		this.searchResultRows = nil
		this.showModelSearchOverview()
		this.searchStatusLabel.SetText("No models found")
	} else {
		this.searchResults = models
		this.searchResultRows = buildSearchResultRows(models)
		if len(models) == 1 {
			this.searchStatusLabel.SetText("1 model found")
		} else {
			this.searchStatusLabel.SetText(fmt.Sprintf("%d models found", len(models)))
		}
		this.searchOverview.Hide()
		this.searchResultsTable.Show()
		this.searchResultsTable.Refresh()
	}

	// a searchable scanner keeps its search box above the results (even on a
	// failed or empty search, so the query can be adjusted)
	this.setSearchQueryBarVisible(this.searchQueryScanner != nil)

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

// importModel copies the named scanned model's files into local storage, the
// same way downloadModelRow does for models that are not local files.
func (this *QGUI) importModel(m modelDefinitions.RemoteModel) {
	this.installModel(m, "", installImport)
}

// linkModel links the named scanned model into local storage: only its
// metadata is stored, and its variants keep pointing at the source files.
func (this *QGUI) linkModel(m modelDefinitions.RemoteModel) {
	this.installModel(m, "", installLink)
}

// installModel installs the named scanned model according to action: it
// re-resolves the model through its scanner and then either links it or
// downloads (copies) it. When variantFile is set, only that variant is
// downloaded (so a single HuggingFace quantization can be installed); an empty
// variantFile installs every variant. The work runs in the background, with
// the spinner and the status label showing progress.
func (this *QGUI) installModel(m modelDefinitions.RemoteModel, variantFile string, action modelInstallAction) {
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
			} else if variantFile != "" {
				err = this.QApp.LocalModels.DownloadVariant(remote, variantFile)
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
