package gui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	backendDefinitions "github.com/kncept/quesadilla/backend/definitions"
)

// backendColumns are the table headers shown for the backend list.
var backendColumns = []string{"ID", "Model Types", "Installed Versions"}

const noneInstalled = "(none installed)"

// backendsPage renders the Backends screen: one table row per backend,
// listing what model types it supports and which versions it has installed.
func (this *QGUI) backendsPage() fyne.CanvasObject {
	header := container.NewVBox(
		headingLabel("Backends"),
		wrappedLabel("Backends, the model types they support, and their installed versions."),
		widget.NewSeparator(),
	)
	return container.NewPadded(container.NewBorder(header, nil, nil, nil, newBackendsTable(this.QApp.Backends.Backends())))
}

// newBackendsTable builds the table of backends.
func newBackendsTable(backends []backendDefinitions.Backend) *widget.Table {
	table := widget.NewTable(
		func() (int, int) {
			return len(backends), len(backendColumns)
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapOff
			return label
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			if id.Row < 0 || id.Row >= len(backends) {
				return
			}
			cell.(*widget.Label).SetText(backendCell(backends[id.Row], id.Col))
		},
	)

	table.ShowHeaderRow = true
	table.CreateHeader = func() fyne.CanvasObject {
		return widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	table.UpdateHeader = func(id widget.TableCellID, cell fyne.CanvasObject) {
		if id.Col >= 0 && id.Col < len(backendColumns) {
			cell.(*widget.Label).SetText(backendColumns[id.Col])
		}
	}

	table.SetColumnWidth(0, 80)  // ID
	table.SetColumnWidth(1, 120) // Model Types
	table.SetColumnWidth(2, 200) // Installed Versions

	return table
}

// backendCell returns the display value for one cell of the backend table.
func backendCell(b backendDefinitions.Backend, col int) string {
	switch col {
	case 0:
		return b.Id()
	case 1:
		return strings.Join(b.ModelTypes(), ", ")
	case 2:
		return formatVersionList(b.InstalledVersions(), noneInstalled)
	default:
		return ""
	}
}

// formatVersionList renders a version list on a single line, or the given
// fallback when there are none.
func formatVersionList(versions []string, fallback string) string {
	if len(versions) == 0 {
		return fallback
	}
	return strings.Join(versions, ", ")
}
