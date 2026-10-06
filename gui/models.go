package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/quesadilla/backend/running"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
)

// modelColumns are the table headers shown for the model list.
var modelColumns = []string{"Name", "Type", "Running", "File"}

// modelsPage renders the Models screen: the installed models, as a table with
// a row per model. The table reads from the repository on refresh, so it
// always reflects the current set (eg after a link or remove).
func (this *QGUI) modelsPage() fyne.CanvasObject {
	header := container.NewVBox(
		headingLabel("Models"),
		wrappedLabel("Installed models, including config and running stats."),
		widget.NewSeparator(),
	)
	return container.NewPadded(container.NewBorder(header, nil, nil, nil, this.modelsTable()))
}

// modelsTable builds the installed-models table.
func (this *QGUI) modelsTable() *widget.Table {
	table := widget.NewTable(
		func() (int, int) {
			return len(this.QApp.Models.Models()), len(modelColumns)
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapOff
			return label
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			models := this.QApp.Models.Models()
			if id.Row < 0 || id.Row >= len(models) {
				return
			}
			cell.(*widget.Label).SetText(modelCell(models[id.Row], id.Col))
		},
	)

	table.ShowHeaderRow = true
	table.CreateHeader = func() fyne.CanvasObject {
		return widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	table.UpdateHeader = func(id widget.TableCellID, cell fyne.CanvasObject) {
		if id.Col >= 0 && id.Col < len(modelColumns) {
			cell.(*widget.Label).SetText(modelColumns[id.Col])
		}
	}

	table.SetColumnWidth(0, 220) // Name
	table.SetColumnWidth(1, 80)  // Type
	table.SetColumnWidth(2, 80)  // Running
	table.SetColumnWidth(3, 400) // File

	return table
}

// modelCell returns the display value for one cell of the model table.
func modelCell(m modelDefinitions.Model, col int) string {
	switch col {
	case 0:
		return m.ModelName
	case 1:
		return m.ModelType
	case 2:
		if isRunning(m.ModelName) {
			return "yes"
		}
		return ""
	case 3:
		return m.ModelFile
	default:
		return ""
	}
}

// isRunning reports whether a model with the given name is currently running.
func isRunning(modelName string) bool {
	for _, m := range running.Default().RunningModels() {
		if m.ModelName() == modelName {
			return true
		}
	}
	return false
}
