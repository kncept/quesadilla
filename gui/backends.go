package gui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	backendDefinitions "github.com/kncept/quesadilla/backend/definitions"
	"github.com/kncept/quesadilla/config"
)

// backendColumns are the table headers shown for the backend list. The last
// column holds each row's move up/down action.
var backendColumns = []string{"Model Type", "Backend", "Installed Versions", "Actions"}

const noneInstalled = "(none installed)"

// defaultMarker is what a group's topmost (default) backend's row shows.
const defaultMarker = " (default)"

// backendsPage renders the Backends screen: the backends broken out by the
// model types they support - one row per (model type, backend), so a backend
// with several types gets a row per type - in the persisted order, where the
// topmost backend of each type group is that type's default.
func (this *QGUI) backendsPage() fyne.CanvasObject {
	header := container.NewVBox(
		headingLabel("Backends"),
		wrappedLabel("Backends broken out by the model types they support. The topmost backend of a model type is the default for that type; use the arrows to reorder."),
		widget.NewSeparator(),
	)
	return container.NewPadded(container.NewBorder(header, nil, nil, nil, this.newBackendsTable()))
}

// newBackendsTable builds the backends table: one row per (model type,
// backend) entry of the repository's order, with a move up/down action on
// each row.
func (this *QGUI) newBackendsTable() *widget.Table {
	table := widget.NewTable(
		func() (int, int) {
			return len(this.QApp.Backends.BackendOrder()), len(backendColumns)
		},
		backendCellFactory,
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			order := this.QApp.Backends.BackendOrder()
			if id.Row < 0 || id.Row >= len(order) {
				return
			}
			this.updateBackendCell(order, id, cell.(*fyne.Container))
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

	table.SetColumnWidth(0, 110) // Model Type
	table.SetColumnWidth(1, 160) // Backend
	table.SetColumnWidth(2, 180) // Installed Versions
	table.SetColumnWidth(3, 110) // Actions

	this.backendsTable = table
	return table
}

// backendCellFactory builds a cell of the backends table. Like the model
// table's cells it needs a real layout: a bare &fyne.Container{} never lays
// out its children, which leaves them at 0x0 - invisible and untappable. It
// also needs a minimum size that fits the action buttons, because the table
// sizes its columns from the template cell's minimum size; the seeded
// buttons are only measured and never rendered (updateBackendCell replaces
// the contents).
func backendCellFactory() fyne.CanvasObject {
	up := widget.NewButtonWithIcon("", theme.MenuDropUpIcon(), nil)
	down := widget.NewButtonWithIcon("", theme.MenuDropDownIcon(), nil)
	return container.NewVBox(container.NewHBox(up, down))
}

// updateBackendCell fills one cell of the backends table: a label for the
// data columns, and the move up/down buttons in the last column.
func (this *QGUI) updateBackendCell(order []config.BackendOrderEntry, id widget.TableCellID, cell *fyne.Container) {
	entry := order[id.Row]
	cell.RemoveAll()
	if id.Col == len(backendColumns)-1 {
		cell.Add(this.backendMoveButtons(order, id.Row))
	} else {
		label := widget.NewLabel(backendCell(entry, this.QApp.Backends.Backend(entry.BackendId), isGroupTop(order, id.Row), id.Col))
		label.Wrapping = fyne.TextWrapOff
		cell.Add(label)
	}
	cell.Refresh()
}

// backendMoveButtons builds the move up/down buttons for a backends-table
// row. The buttons move the row within its model type group only - the
// topmost backend of a group is that type's default - and the buttons at the
// group's boundaries are disabled.
func (this *QGUI) backendMoveButtons(order []config.BackendOrderEntry, row int) fyne.CanvasObject {
	entry := order[row]
	up := widget.NewButtonWithIcon("", theme.MenuDropUpIcon(), func() { this.moveBackend(entry, true) })
	down := widget.NewButtonWithIcon("", theme.MenuDropDownIcon(), func() { this.moveBackend(entry, false) })
	if !canMoveUp(order, row) {
		up.Disable()
	}
	if !canMoveDown(order, row) {
		down.Disable()
	}
	return container.NewHBox(up, down)
}

// moveBackend moves the given row one step within its model type group and
// refreshes the table when the order changed. The move persists in the
// config, so the new order (and the new default) survives a restart.
func (this *QGUI) moveBackend(entry config.BackendOrderEntry, up bool) {
	if this.QApp.Backends.MoveBackend(entry.ModelType, entry.BackendId, up) {
		this.backendsTable.Refresh()
	}
}

// canMoveUp reports whether the given row has a row of the same model type
// above it.
func canMoveUp(order []config.BackendOrderEntry, row int) bool {
	return row > 0 && order[row-1].ModelType == order[row].ModelType
}

// canMoveDown reports whether the given row has a row of the same model type
// below it.
func canMoveDown(order []config.BackendOrderEntry, row int) bool {
	return row < len(order)-1 && order[row+1].ModelType == order[row].ModelType
}

// isGroupTop reports whether the given row is the topmost row of its model
// type group - that backend is the type's default.
func isGroupTop(order []config.BackendOrderEntry, row int) bool {
	return row == 0 || order[row-1].ModelType != order[row].ModelType
}

// backendCell returns the display value for one data cell of the backends
// table.
func backendCell(entry config.BackendOrderEntry, backend backendDefinitions.Backend, isDefault bool, col int) string {
	switch col {
	case 0:
		return entry.ModelType
	case 1:
		if isDefault {
			return entry.BackendId + defaultMarker
		}
		return entry.BackendId
	case 2:
		if backend == nil {
			return noneInstalled
		}
		return formatVersionList(backend.InstalledVersions(), noneInstalled)
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
