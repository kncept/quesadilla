package gui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/quesadilla/backend/running"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// modelColumns are the table headers shown for the model list. The first
// column holds each model's row actions: start/stop, delete and logs.
var modelColumns = []string{"Actions", "Name", "Type", "Status", "File"}

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
// the row's actions (start/stop, delete, logs) in the leftmost column.
func (this *QGUI) newModelsTable() *widget.Table {
	table := widget.NewTable(
		func() (int, int) {
			return len(this.QApp.LocalModels.Models()), len(modelColumns)
		},
		modelsCellFactory,
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			models := this.QApp.LocalModels.Models()
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

	table.SetColumnWidth(0, 280) // Actions
	table.SetColumnWidth(1, 220) // Name
	table.SetColumnWidth(2, 80)  // Type
	table.SetColumnWidth(3, 90)  // Status
	table.SetColumnWidth(4, 400) // File

	this.modelsTable = table
	return table
}

// modelsCellFactory builds a cell of the installed-models table. The cell
// needs a real layout: a bare &fyne.Container{} never lays out its children,
// which leaves them at 0x0 - invisible and untappable. It also needs a
// minimum size that fits the row's action buttons, because the table sizes
// its rows from the template cell's minimum size; the seeded buttons are only
// measured and never rendered (updateModelCell replaces the contents).
func modelsCellFactory() fyne.CanvasObject {
	return container.NewHBox(
		widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), nil),
		widget.NewButtonWithIcon("", theme.DeleteIcon(), nil),
		widget.NewButtonWithIcon("Logs", theme.FileTextIcon(), nil),
		widget.NewButtonWithIcon("Open", theme.ComputerIcon(), nil),
	)
}

// runningModelCellFactory builds a cell of the Overview's running-models
// table, sized to fit its single action button (see modelsCellFactory for why
// the template needs a layout and a minimum size).
func runningModelCellFactory() fyne.CanvasObject {
	return container.NewVBox(widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), nil))
}

// updateModelCell fills one cell of the model table: a label for the data
// columns, and the row's actions (start/stop, delete, logs, and open when the
// model's metadata records a page) in the first column.
func (this *QGUI) updateModelCell(m modelDefinitions.Model, id widget.TableCellID, cell *fyne.Container) {
	cell.RemoveAll()
	if id.Col == 0 {
		cell.Add(this.modelActionButton(m))
		cell.Add(this.modelDeleteButton(m))
		cell.Add(this.modelLogsButton(m))
		if m.ModelURL != "" {
			cell.Add(this.modelOpenButton(m))
		}
	} else {
		label := widget.NewLabel(modelCell(m, id.Col))
		label.Wrapping = fyne.TextWrapOff
		cell.Add(label)
	}
	cell.Refresh()
}

// modelActionButton builds the start/stop button for a model's table row.
func (this *QGUI) modelActionButton(m modelDefinitions.Model) *widget.Button {
	if isRunning(m.FullModelName()) {
		return widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), func() {
			this.stopModel(m.FullModelName())
		})
	}
	return widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), func() {
		this.startModel(&m)
	})
}

// modelDeleteButton builds the delete button for a model's table row. It uses
// the standard delete icon at the normal button importance, so its icon is
// drawn in the theme's foreground color rather than the alarming error (red)
// color a danger button would use. Tapping it asks for confirmation before the
// model is actually removed (see confirmDeleteModel).
func (this *QGUI) modelDeleteButton(m modelDefinitions.Model) *widget.Button {
	return widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		this.confirmDeleteModel(m)
	})
}

// modelLogsButton builds the logs button for a model's table row. It opens the
// same log viewer as the Overview's running-models table. The button is
// disabled while the model is not running, because there are no captured log
// lines to show.
func (this *QGUI) modelLogsButton(m modelDefinitions.Model) *widget.Button {
	button := widget.NewButtonWithIcon("Logs", theme.FileTextIcon(), func() {
		if running := runningModelFor(m.FullModelName()); running != nil {
			this.showLogs(running)
		}
	})
	if runningModelFor(m.FullModelName()) == nil {
		button.Disable()
	}
	return button
}

// modelOpenButton builds the open button for a model's table row: it opens
// the model's page at its source (eg its card on a model hub) in the system
// browser. It is shown only for models whose metadata records a page, with
// the same behaviour as the Open button on the Model Search screen.
func (this *QGUI) modelOpenButton(m modelDefinitions.Model) *widget.Button {
	return widget.NewButtonWithIcon("Open", theme.ComputerIcon(), func() {
		this.openModelCard(m.ModelURL)
	})
}

// confirmDeleteModel asks the user to confirm deleting the named model before
// anything is removed. The model is only deleted when the user accepts;
// cancelling leaves it untouched.
func (this *QGUI) confirmDeleteModel(m modelDefinitions.Model) {
	this.mu.Lock()
	win := this.mainWin
	this.mu.Unlock()
	if win == nil {
		return
	}
	message := fmt.Sprintf("Delete model %q?\n\nThis removes the model and its files, and cannot be undone.", m.FullModelName())
	dialog.NewConfirm("Delete Model", message, func(confirmed bool) {
		if confirmed {
			this.deleteModel(m.FullModelName())
		}
	}, win).Show()
}

// deleteModel removes the named model from local storage and refreshes the
// installed-models table so its row disappears.
func (this *QGUI) deleteModel(modelName string) {
	if err := this.QApp.LocalModels.RemoveModel(modelName); err != nil {
		this.notifyError(fmt.Sprintf("deleting %s: %v", modelName, err))
		return
	}
	if this.modelsTable != nil {
		this.modelsTable.Refresh()
	}
}

// runningModelFor returns the running model with the given name, or nil when
// no such model is running.
func runningModelFor(modelName string) runnerDefinitions.RunningModel {
	for _, m := range running.Default().RunningModels() {
		if m.ModelName() == modelName {
			return m
		}
	}
	return nil
}

// startModel runs the given model on the first installed backend that can
// serve its type. The run is tracked in the process-wide registry, so the
// pages and the tray pick it up on their next refresh.
func (this *QGUI) startModel(m *modelDefinitions.Model) {
	fmt.Printf("startModel %s\n", m.FullModelName())
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
// Column 0 holds the row actions, not a label.
func modelCell(m modelDefinitions.Model, col int) string {
	switch col {
	case 1:
		return m.FullModelName()
	case 2:
		return m.ModelType
	case 3:
		if isRunning(m.FullModelName()) {
			return "running"
		}
		return "idle"
	case 4:
		return m.ModelFile
	default:
		return ""
	}
}

// isRunning reports whether a model with the given name is currently running.
func isRunning(modelName string) bool {
	return runningModelFor(modelName) != nil
}

// runningModelColumns are the table headers for the Overview's running-models
// section. The second-to-last column holds the "view logs" action and the
// last column holds the stop action.
var runningModelColumns = []string{"Model", "Backend", "Version", "Uptime", "Logs", "Action"}

// runningModelsContent builds the Overview's running-models section: a table
// with one row per running model (name, backend, version, uptime and a stop
// button), or a placeholder label when nothing is running.
func (this *QGUI) runningModelsContent() fyne.CanvasObject {
	table := widget.NewTable(
		func() (int, int) {
			return len(running.Default().RunningModels()), len(runningModelColumns)
		},
		runningModelCellFactory,
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

	table.SetColumnWidth(0, 180) // Model
	table.SetColumnWidth(1, 120) // Backend
	table.SetColumnWidth(2, 90)  // Version
	table.SetColumnWidth(3, 90)  // Uptime
	table.SetColumnWidth(4, 110) // Logs
	table.SetColumnWidth(5, 100) // Action

	empty := widget.NewLabel("No models running")
	this.runningModelsTable = table
	this.noRunningLabel = empty
	this.updateRunningModelsVisibility()
	return container.NewStack(table, empty)
}

// updateRunningModelCell fills one cell of the running-models table: a label
// for the data columns, the "view logs" button in the second-to-last column,
// and the stop button in the last column.
func (this *QGUI) updateRunningModelCell(m runnerDefinitions.RunningModel, id widget.TableCellID, cell *fyne.Container) {
	cell.RemoveAll()
	switch id.Col {
	case len(runningModelColumns) - 1: // Action: stop the model
		cell.Add(widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), func() {
			this.stopModel(m.ModelName())
		}))
	case len(runningModelColumns) - 2: // Logs: open the log viewer
		cell.Add(this.viewLogsButton(m))
	default:
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
