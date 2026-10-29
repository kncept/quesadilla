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
var modelColumns = []string{"Actions", "Name", "Type", "Status"}

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
		widget.NewButtonWithIcon("Details", theme.InfoIcon(), nil),
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
		cell.Add(this.modelDetailsButton(m))
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

// modelDetailsButton builds the details button for a model's table row: it
// navigates to the Model Details screen for that model.
func (this *QGUI) modelDetailsButton(m modelDefinitions.Model) *widget.Button {
	return widget.NewButtonWithIcon("Details", theme.InfoIcon(), func() {
		this.showModelDetails(m.FullModelName())
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

// modelDetailsPage renders the Model Details screen: a dropdown to select a
// model, and the selected model's metadata and variant details.
func (this *QGUI) modelDetailsPage() fyne.CanvasObject {
	// Initialize the display container if needed
	if this.modelDetailsDisplay == nil {
		this.modelDetailsDisplay = container.NewVBox()
	}
	// This will be populated when the page is shown
	this.modelDetailsContent = container.NewVBox()
	this.refreshModelDetailsContent()
	return container.NewPadded(this.modelDetailsContent)
}

// refreshModelDetailsContent rebuilds the Model Details screen content.
func (this *QGUI) refreshModelDetailsContent() {
	if this.modelDetailsContent == nil {
		return
	}

	models := this.QApp.LocalModels.Models()
	if len(models) == 0 {
		this.modelDetailsContent.RemoveAll()
		this.modelDetailsContent.Add(wrappedLabel("No models installed."))
		this.modelDetailsContent.Refresh()
		return
	}

	// Build the dropdown options
	modelNames := make([]string, len(models))
	for i, m := range models {
		modelNames[i] = m.FullModelName()
	}

	// Create or reuse the dropdown
	if this.modelDetailsSelect == nil {
		this.modelDetailsSelect = widget.NewSelect(modelNames, func(selected string) {
			this.showModelDetailsContent(selected)
		})
		this.modelDetailsSelect.PlaceHolder = "Select a model..."
	} else {
		this.modelDetailsSelect.Options = modelNames
		this.modelDetailsSelect.Refresh()
	}

	// If no model is selected yet, select the first one
	if this.modelDetailsSelected == "" && len(modelNames) > 0 {
		this.modelDetailsSelected = modelNames[0]
		this.modelDetailsSelect.SetSelected(this.modelDetailsSelected)
	}

	// Build the header with the dropdown
	header := container.NewVBox(
		headingLabel("Model Details"),
		this.modelDetailsSelect,
		widget.NewSeparator(),
	)

	this.modelDetailsContent.RemoveAll()
	this.modelDetailsContent.Add(header)
	this.modelDetailsContent.Add(this.modelDetailsDisplay)
	this.showModelDetailsContent(this.modelDetailsSelected)
	this.modelDetailsContent.Refresh()
}

// showModelDetailsContent displays the details for the selected model.
func (this *QGUI) showModelDetailsContent(modelName string) {
	this.modelDetailsSelected = modelName
	model := this.QApp.LocalModels.GetModel(modelName)
	if model == nil {
		this.modelDetailsDisplay.RemoveAll()
		this.modelDetailsDisplay.Add(wrappedLabel("Model not found: " + modelName))
		this.modelDetailsDisplay.Refresh()
		return
	}

	this.modelDetailsDisplay.RemoveAll()

	// Model metadata section
	metadataSection := this.buildModelMetadataSection(*model)
	this.modelDetailsDisplay.Add(metadataSection)

	// Variants section
	if len(model.Variants) > 0 {
		variantsSection := this.buildVariantsSection(*model)
		this.modelDetailsDisplay.Add(widget.NewSeparator())
		this.modelDetailsDisplay.Add(variantsSection)
	}

	this.modelDetailsDisplay.Refresh()
}

// buildModelMetadataSection creates a section showing the model's metadata.
func (this *QGUI) buildModelMetadataSection(m modelDefinitions.Model) fyne.CanvasObject {
	items := []fyne.CanvasObject{
		headingLabel("Model Metadata"),
	}

	fields := []struct {
		label string
		value string
	}{
		{"Full Name", m.FullModelName()},
		{"Family", m.ModelFamily},
		{"Name", m.ModelName},
		{"Type", m.ModelType},
		{"Directory", m.ModelDirectory},
		{"Source", m.ModelSource},
		{"URL", m.ModelURL},
		{"Licence", m.LicenceName},
		{"Licence File", m.LicenceFile},
		{"Metadata Version", m.MetadataVersion},
	}

	for _, f := range fields {
		if f.value != "" {
			items = append(items, this.metadataRow(f.label, f.value))
		}
	}

	return container.NewVBox(items...)
}

// buildVariantsSection creates a split view with a variant list on the left
// and the selected variant's details on the right (similar to Model Search).
func (this *QGUI) buildVariantsSection(m modelDefinitions.Model) fyne.CanvasObject {
	if len(m.Variants) == 0 {
		return container.NewVBox(
			headingLabel("Variants (0)"),
			wrappedLabel("No variants available."),
		)
	}

	// Variant list on the left
	variantNames := make([]string, len(m.Variants))
	for i, v := range m.Variants {
		variantNames[i] = v.FileName
	}

	// Find the longest name for the list template
	longest := variantNames[0]
	for _, name := range variantNames {
		if len(name) > len(longest) {
			longest = name
		}
	}

	variantList := widget.NewList(
		func() int {
			return len(m.Variants)
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel(longest)
			label.Wrapping = fyne.TextWrapOff
			return container.NewPadded(label)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*fyne.Container).Objects[0].(*widget.Label).SetText(variantNames[id])
		},
	)

	// Detail panel on the right - shows the selected variant's details
	detailContainer := container.NewVBox()

	// Function to update the detail panel
	updateDetail := func(variantIndex int) {
		if variantIndex < 0 || variantIndex >= len(m.Variants) {
			return
		}
		v := m.Variants[variantIndex]
		detailContainer.RemoveAll()

		fields := []struct {
			label string
			value string
			isBool bool
		}{
			{"File Name", v.FileName, false},
			{"Description", v.Description, false},
			{"Linked Model", fmt.Sprintf("%v", v.LinkedModel), true},
			{"Location", v.Location, false},
			{"File Size", formatFileSize(v.FileSize), false},
			{"Imported At", v.ImportedAt.Format(time.RFC3339), false},
			{"Supports Vision", fmt.Sprintf("%v", v.SupportsVision), true},
		}

		detailContainer.Add(headingLabel(fmt.Sprintf("Variant: %s", v.FileName)))
		detailContainer.Add(widget.NewSeparator())

		for _, f := range fields {
			// For boolean fields, always show. For others, skip empty/zero.
			if f.isBool || (f.value != "" && f.value != "0" && f.value != "0001-01-01T00:00:00Z") {
				detailContainer.Add(this.metadataRow(f.label, f.value))
			}
		}

		detailContainer.Refresh()
	}

	// Select the first variant by default
	variantList.OnSelected = func(id widget.ListItemID) {
		updateDetail(int(id))
	}
	if len(variantNames) > 0 {
		updateDetail(0)
		variantList.Select(0)
	}

	// Split view: list on left, details on right
	left := container.NewBorder(
		headingLabel(fmt.Sprintf("Variants (%d)", len(m.Variants))),
		nil, nil, nil,
		variantList,
	)

	right := container.NewPadded(
		container.NewVScroll(detailContainer),
	)

	split := container.NewHSplit(left, right)
	split.SetOffset(0.3) // 30% for the list, 70% for details

	return split
}

// metadataRow creates a label-value row for metadata display.
func (this *QGUI) metadataRow(label, value string) fyne.CanvasObject {
	labelWidget := widget.NewLabelWithStyle(label+":", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	labelWidget.Wrapping = fyne.TextWrapOff
	valueWidget := widget.NewLabel(value)
	valueWidget.Wrapping = fyne.TextWrapWord
	valueWidget.TextStyle = fyne.TextStyle{Monospace: true}
	return container.NewGridWithColumns(2, labelWidget, valueWidget)
}

// formatFileSize formats a file size in bytes to a human-readable string.
func formatFileSize(bytes int64) string {
	if bytes <= 0 {
		return ""
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// showModelDetails navigates to the Model Details screen for the given model.
// It selects the Model Details tab and shows the model's details.
func (this *QGUI) showModelDetails(modelName string) {
	// Switch to the Model Details tab (index 2 in the sidebar)
	this.mu.Lock()
	win := this.mainWin
	this.mu.Unlock()
	if win == nil {
		return
	}

	// The sidebar selection needs to happen on the main thread
	fyne.Do(func() {
		// Set the selected model so it shows up when the tab is selected
		this.modelDetailsSelected = modelName
		if this.modelDetailsSelect != nil {
			this.modelDetailsSelect.SetSelected(modelName)
		}
		// Switch to the Model Details tab (index 2)
		if this.sidebarList != nil {
			this.sidebarList.Select(2)
		}
	})
}
