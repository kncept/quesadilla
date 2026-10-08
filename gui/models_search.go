package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
)

// modelSearchColumns are the table headers shown for the model search
// results.
var modelSearchColumns = []string{"Name", "Type", "File"}

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
		this.runModelSearch(scanners[id-1])
	}

	// the search status: a spinner while a scan is active, and a label with
	// the current state (searching, done, failed)
	this.searchSpinner = widget.NewProgressBarInfinite()
	this.searchStatusLabel = widget.NewLabel("")
	this.searchSpinner.Hide()
	this.searchStatusLabel.Hide()

	// the results area: the overview text, or the last scan's results
	this.searchOverview = widget.NewLabel(modelSearchOverviewText)
	this.searchOverview.Wrapping = fyne.TextWrapWord
	this.searchResultsTable = newSearchResultsTable(this)
	this.searchResultsTable.Hide()

	right := container.NewBorder(
		container.NewHBox(this.searchSpinner, this.searchStatusLabel), // top: search status
		nil, nil, nil,
		container.NewStack(this.searchOverview, this.searchResultsTable),
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

// newSearchResultsTable builds the table of the last scan's results.
func newSearchResultsTable(g *QGUI) *widget.Table {
	table := widget.NewTable(
		func() (int, int) {
			return len(g.searchResults), len(modelSearchColumns)
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapOff
			return label
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			if id.Row < 0 || id.Row >= len(g.searchResults) {
				return
			}
			cell.(*widget.Label).SetText(searchResultCell(g.searchResults[id.Row], id.Col))
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

	table.SetColumnWidth(0, 240) // Name
	table.SetColumnWidth(1, 80)  // Type
	table.SetColumnWidth(2, 380) // File

	return table
}

// searchResultCell returns the display value for one data cell of the
// search-results table.
func searchResultCell(m modelDefinitions.RemoteModel, col int) string {
	switch col {
	case 0:
		return m.ModelName
	case 1:
		return m.ModelType
	case 2:
		return m.ModelFile
	default:
		return ""
	}
}

// showModelSearchOverview shows the overview text in the results area.
func (this *QGUI) showModelSearchOverview() {
	this.searchOverview.Show()
	this.searchResultsTable.Hide()
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

	go func() {
		models, err := scanner.ScanForModels()
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
	this.searchSpinner.Hide()

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
