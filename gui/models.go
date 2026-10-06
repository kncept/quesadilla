package gui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/quesadilla/backend/running"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// modelColumns are the table headers shown for the model list. The last
// column holds each model's start/stop action.
var modelColumns = []string{"Name", "Type", "Status", "File", "Actions"}

// modelsPage renders the Models screen: the installed models, as a table with
// a row per model. The table reads from the repository on refresh, so it
// always reflects the current set (eg after a link or remove).
func (this *QGUI) modelsPage() fyne.CanvasObject {
	header := container.NewVBox(
		headingLabel("Models"),
		wrappedLabel("Installed models, including config and running stats."),
		widget.NewSeparator(),
	)
	return container.NewPadded(container.NewBorder(header, nil, nil, nil, this.newModelsTable()))
}

// newModelsTable builds the installed-models table: one row per model, with
// a start/stop action button on the right.
func (this *QGUI) newModelsTable() *widget.Table {
	table := widget.NewTable(
		func() (int, int) {
			return len(this.QApp.Models.Models()), len(modelColumns)
		},
		modelCellFactory,
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			models := this.QApp.Models.Models()
			if id.Row < 0 || id.Row >= len(models) {
				return
			}
			this.updateModelCell(models[id.Row], id, cell.(*fyne.Container))
		},
	)

	table.ShowHeaderRow = true
	table.CreateHeader = func() fyne.CanvasObject {
		return widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	table.UpdateHeader = func(id widget.TableCellID, cell fyne.CanvasObject) {
		if id.Col >= 0 && id.Col < len(modelColumns) {
			// fyne.Do(func() {
			cell.(*widget.Label).SetText(modelColumns[id.Col])
			// })
		}
	}

	table.SetColumnWidth(0, 220) // Name
	table.SetColumnWidth(1, 80)  // Type
	table.SetColumnWidth(2, 90)  // Running
	table.SetColumnWidth(3, 400) // File
	table.SetColumnWidth(4, 110) // Action

	this.modelsTable = table
	return table
}

// modelCellFactory builds a cell of the model (and running-models) tables.
// The cell needs a real layout: a bare &fyne.Container{} never lays out its
// children, which leaves them at 0x0 - invisible and untappable. It also
// needs a minimum size that fits the action button, because the table sizes
// its rows from the template cell's minimum size; the seeded button is only
// measured and never rendered (updateModelCell replaces the contents).
func modelCellFactory() fyne.CanvasObject {
	return container.NewVBox(widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), nil))
}

// updateModelCell fills one cell of the model table: a label for the data
// columns, and the start/stop button in the last column.
func (this *QGUI) updateModelCell(m modelDefinitions.Model, id widget.TableCellID, cell *fyne.Container) {
	cell.RemoveAll()
	if id.Col == len(modelColumns)-1 {
		cell.Add(this.modelActionButton(m))
	} else {
		label := widget.NewLabel(modelCell(m, id.Col))
		label.Wrapping = fyne.TextWrapOff
		cell.Add(label)
	}
	cell.Refresh()
}

// modelActionButton builds the start/stop button for a model's table row.
func (this *QGUI) modelActionButton(m modelDefinitions.Model) *widget.Button {
	if isRunning(m.ModelName) {
		return widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), func() {
			this.stopModel(m.ModelName)
		})
	}
	return widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), func() {
		this.startModel(&m)
	})
}

// startModel runs the given model on the first installed backend that can
// serve its type. The run is tracked in the process-wide registry, so the
// pages and the tray pick it up on their next refresh.
func (this *QGUI) startModel(m *modelDefinitions.Model) {
	fmt.Printf("startModel %s\n", m.ModelName)
	backend := this.QApp.Backends.BackendForModelType(m.ModelType)
	if backend == nil {
		this.notifyError(fmt.Sprintf("no installed backend can run %s models - install one first", m.ModelType))
		return
	}
	this.QApp.Start(backend, m)
}

// stopModel asks the named running model to quit, if it is running.
func (this *QGUI) stopModel(modelName string) {
	fmt.Printf("stopModel %s\n", modelName)
	this.QApp.Stop(modelName)
}

// modelCell returns the display value for one data cell of the model table.
func modelCell(m modelDefinitions.Model, col int) string {
	switch col {
	case 0:
		return m.ModelName
	case 1:
		return m.ModelType
	case 2:
		if isRunning(m.ModelName) {
			return "running"
		}
		return "idle"
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

// runningModelColumns are the table headers for the Overview's running-models
// section. The last column holds the stop action.
var runningModelColumns = []string{"Model", "Backend", "Version", "Uptime", "Action"}

// runningModelsContent builds the Overview's running-models section: a table
// with one row per running model (name, backend, version, uptime and a stop
// button), or a placeholder label when nothing is running.
func (this *QGUI) runningModelsContent() fyne.CanvasObject {
	table := widget.NewTable(
		func() (int, int) {
			return len(running.Default().RunningModels()), len(runningModelColumns)
		},
		modelCellFactory,
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			models := running.Default().RunningModels()
			if id.Row < 0 || id.Row >= len(models) {
				return
			}
			this.updateRunningModelCell(models[id.Row], id, cell.(*fyne.Container))
		},
	)

	table.ShowHeaderRow = true
	table.CreateHeader = func() fyne.CanvasObject {
		return widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	table.UpdateHeader = func(id widget.TableCellID, cell fyne.CanvasObject) {
		if id.Col >= 0 && id.Col < len(runningModelColumns) {
			cell.(*widget.Label).SetText(runningModelColumns[id.Col])
		}
	}

	table.SetColumnWidth(0, 200) // Model
	table.SetColumnWidth(1, 140) // Backend
	table.SetColumnWidth(2, 100) // Version
	table.SetColumnWidth(3, 100) // Uptime
	table.SetColumnWidth(4, 110) // Action

	empty := widget.NewLabel("No models running")
	this.runningModelsTable = table
	this.noRunningLabel = empty
	this.updateRunningModelsVisibility()
	return container.NewStack(table, empty)
}

// updateRunningModelCell fills one cell of the running-models table: a label
// for the data columns, and the stop button in the last column.
func (this *QGUI) updateRunningModelCell(m runnerDefinitions.RunningModel, id widget.TableCellID, cell *fyne.Container) {
	cell.RemoveAll()
	if id.Col == len(runningModelColumns)-1 {
		cell.Add(widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), func() {
			this.stopModel(m.ModelName())
		}))
	} else {
		label := widget.NewLabel(runningModelCell(m, id.Col))
		label.Wrapping = fyne.TextWrapOff
		cell.Add(label)
	}
	cell.Refresh()
}

// runningModelCell returns the display value for one data cell of the
// running-models table.
func runningModelCell(m runnerDefinitions.RunningModel, col int) string {
	switch col {
	case 0:
		return m.ModelName()
	case 1:
		return m.ProviderName()
	case 2:
		return m.RuntimeVersion()
	case 3:
		return formatUptime(m.Uptime())
	default:
		return ""
	}
}

// updateRunningModelsVisibility shows the table when models are running, and
// the placeholder otherwise.
func (this *QGUI) updateRunningModelsVisibility() {
	hasRunning := len(running.Default().RunningModels()) > 0
	if this.runningModelsTable != nil {
		if hasRunning {
			this.runningModelsTable.Show()
		} else {
			this.runningModelsTable.Hide()
		}
	}
	if this.noRunningLabel != nil {
		if hasRunning {
			this.noRunningLabel.Hide()
		} else {
			this.noRunningLabel.Show()
		}
	}
}

// formatUptime renders a duration in h/m/s for the running-models table.
func formatUptime(d time.Duration) string {
	d = d.Round(time.Second)
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	switch {
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}
