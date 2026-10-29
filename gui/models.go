package gui

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/quesadilla/backend/running"
	"github.com/kncept/quesadilla/model/definitions"
	"github.com/kncept/quesadilla/model/metadata"
	"github.com/kncept/quesadilla/model/remoterepository"
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

	table.SetColumnWidth(0, 420) // Actions (5 buttons: Start, Delete, Logs, Open, Details)
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
func (this *QGUI) updateModelCell(m definitions.Model, id widget.TableCellID, cell *fyne.Container) {
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
func (this *QGUI) modelActionButton(m definitions.Model) *widget.Button {
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
func (this *QGUI) modelDeleteButton(m definitions.Model) *widget.Button {
	return widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		this.confirmDeleteModel(m)
	})
}

// modelLogsButton builds the logs button for a model's table row. It opens the
// same log viewer as the Overview's running-models table. The button is
// disabled while the model is not running, because there are no captured log
// lines to show.
func (this *QGUI) modelLogsButton(m definitions.Model) *widget.Button {
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
func (this *QGUI) modelOpenButton(m definitions.Model) *widget.Button {
	return widget.NewButtonWithIcon("Open", theme.ComputerIcon(), func() {
		this.openModelCard(m.ModelURL)
	})
}

// modelDetailsButton builds the details button for a model's table row: it
// navigates to the Model Details screen for that model.
func (this *QGUI) modelDetailsButton(m definitions.Model) *widget.Button {
	return widget.NewButtonWithIcon("Details", theme.InfoIcon(), func() {
		this.showModelDetails(m.FullModelName())
	})
}

// confirmDeleteModel asks the user to confirm deleting the named model before
// anything is removed. The model is only deleted when the user accepts;
// cancelling leaves it untouched.
func (this *QGUI) confirmDeleteModel(m definitions.Model) {
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
	this.refreshModelDetailsContent()
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
func (this *QGUI) startModel(m *definitions.Model) {
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
func modelCell(m definitions.Model, col int) string {
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

// modelDetailRow is one row of the Model Details left menu: the "Model" entry,
// the optional "Download" entry, or one of the model's variants.
type modelDetailRow struct {
	label        string
	kind         string // "model", "download" or "variant"
	variantIndex int    // only for "variant" rows
}

// modelDetailsPage renders the Model Details screen: a dropdown to select a
// model at the top, then a split view like Model Search - a left menu holding
// "Model", "Download" (for models that can fetch more variants) and one row per
// variant, beside the selected view.
func (this *QGUI) modelDetailsPage() fyne.CanvasObject {
	header := container.NewVBox(
		headingLabel("Model Details"),
		wrappedLabel("Metadata for an installed model and each of its variants."),
		widget.NewSeparator(),
	)

	this.modelDetailsSelect = widget.NewSelect(nil, func(selected string) {
		this.showModelDetailsContent(selected)
	})
	this.modelDetailsSelect.PlaceHolder = "Select a model..."

	// Refresh metadata button - appears below the dropdown
	this.modelDetailsRefreshButton = widget.NewButtonWithIcon("Refresh Metadata", theme.ViewRefreshIcon(), func() {
		this.refreshModelMetadata()
	})
	this.modelDetailsRefreshButton.Disable() // disabled until a model is selected

	this.modelDetailsList = widget.NewList(
		func() int {
			return len(this.modelDetailsRows)
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel("________________________________")
			label.Wrapping = fyne.TextWrapOff
			return container.NewPadded(label)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(this.modelDetailsRows) {
				return
			}
			obj.(*fyne.Container).Objects[0].(*widget.Label).SetText(this.modelDetailsRows[id].label)
		},
	)
	this.modelDetailsList.OnSelected = func(id widget.ListItemID) {
		this.selectModelDetailsRow(int(id))
	}

	// The three right-hand views: the model's metadata, the selected variant's
	// metadata, and the downloadable-variants table. The model and variant
	// views scroll; the download view is a stack so its table fills the space
	// (a table inside a scroll would only get its minimum height).
	this.modelDetailsModelView = container.NewVBox()
	this.modelDetailsVariantView = container.NewVBox()
	this.modelDetailsDownloadView = container.NewStack()
	this.modelDetailsModelScroll = container.NewVScroll(this.modelDetailsModelView)
	this.modelDetailsVariantScroll = container.NewVScroll(this.modelDetailsVariantView)
	this.modelDetailsStack = container.NewStack(
		this.modelDetailsModelScroll,
		this.modelDetailsVariantScroll,
		this.modelDetailsDownloadView,
	)

	split := container.NewHSplit(
		container.NewPadded(this.modelDetailsList),
		container.NewPadded(this.modelDetailsStack),
	)
	split.SetOffset(0.3)

	this.modelDetailsContent = container.NewBorder(
		container.NewVBox(
			this.modelDetailsSelect,
			widget.NewSeparator(),
			container.NewHBox(this.modelDetailsRefreshButton),
			widget.NewSeparator(),
		),
		nil, nil, nil,
		split,
	)

	this.refreshModelDetailsContent()

	return container.NewPadded(container.NewBorder(header, nil, nil, nil, this.modelDetailsContent))
}

// refreshModelDetailsContent rebuilds the dropdown's options and re-renders the
// currently selected model.
func (this *QGUI) refreshModelDetailsContent() {
	if this.modelDetailsSelect == nil {
		return
	}

	models := this.QApp.LocalModels.Models()
	modelNames := make([]string, 0, len(models))
	for _, m := range models {
		modelNames = append(modelNames, m.FullModelName())
	}

	this.modelDetailsSelect.Options = modelNames
	this.modelDetailsSelect.Refresh()

	if len(modelNames) == 0 {
		this.modelDetailsSelected = ""
		this.modelDetailsRows = nil
		this.modelDetailsList.Refresh()
		this.showModelDetailsMessage("No models installed.")
		return
	}

	// Keep the current selection when it still exists, otherwise pick the
	// first installed model.
	selected := this.modelDetailsSelected
	if this.QApp.LocalModels.GetModel(selected) == nil {
		selected = modelNames[0]
	}
	this.modelDetailsSelected = selected
	this.modelDetailsSelect.SetSelected(selected)
	this.showModelDetailsContent(selected)
}

// showModelDetailsContent rebuilds the left menu and the right-hand views for
// the selected model.
func (this *QGUI) showModelDetailsContent(modelName string) {
	this.modelDetailsSelected = modelName
	model := this.QApp.LocalModels.GetModel(modelName)
	if model == nil {
		this.modelDetailsRows = nil
		this.modelDetailsList.Refresh()
		this.showModelDetailsMessage("Model not found: " + modelName)
		if this.modelDetailsRefreshButton != nil {
			this.modelDetailsRefreshButton.Disable()
		}
		return
	}

	// Enable the refresh metadata button when a model is selected
	if this.modelDetailsRefreshButton != nil {
		this.modelDetailsRefreshButton.Enable()
	}

	// Left menu: Model, then Download when the model can fetch more variants,
	// then one row per installed variant.
	rows := []modelDetailRow{{label: "Model", kind: "model"}}
	if isHuggingFaceModel(*model) {
		rows = append(rows, modelDetailRow{label: "Download", kind: "download"})
	}
	for i, v := range model.Variants {
		rows = append(rows, modelDetailRow{label: v.FileName, kind: "variant", variantIndex: i})
	}
	this.modelDetailsRows = rows
	this.modelDetailsList.Refresh()

	// Model metadata view.
	this.buildModelDetailsModelView(*model)

	// Reset the variant view until a variant is picked.
	this.modelDetailsVariantView.RemoveAll()
	this.modelDetailsVariantView.Add(wrappedLabel("Select a variant to view its details."))
	this.modelDetailsVariantView.Refresh()

	// Download view: reset the downloadable-variants state when the selected
	// model changes. The variants are fetched the first time the Download row
	// is opened, so merely picking a model does not hit the network.
	if isHuggingFaceModel(*model) {
		if this.modelDetailsRemoteModel != modelName {
			this.modelDetailsRemoteModel = modelName
			this.modelDetailsRemoteVariants = nil
			this.modelDetailsRemoteLoaded = false
			this.modelDetailsRemoteLoading = false
		}
		this.buildModelDetailsDownloadView()
	} else {
		this.modelDetailsRemoteModel = modelName
		this.modelDetailsRemoteVariants = nil
		this.modelDetailsRemoteLoaded = true
		this.modelDetailsRemoteLoading = false
		this.modelDetailsDownloadView.RemoveAll()
		this.modelDetailsDownloadView.Refresh()
	}

	// Select the Model row by default.
	this.selectModelDetailsRow(0)
	this.modelDetailsList.Select(0)
}

// showModelDetailsMessage shows a message in place of the selected view, when
// no model (or a deleted one) is selected.
func (this *QGUI) showModelDetailsMessage(message string) {
	this.modelDetailsModelView.RemoveAll()
	this.modelDetailsModelView.Add(wrappedLabel(message))
	this.modelDetailsModelView.Refresh()
	this.showModelDetailsView(this.modelDetailsModelScroll)
	if this.modelDetailsRefreshButton != nil {
		this.modelDetailsRefreshButton.Disable()
	}
}

// selectModelDetailsRow shows the view for the given left-menu row.
func (this *QGUI) selectModelDetailsRow(id int) {
	if id < 0 || id >= len(this.modelDetailsRows) {
		return
	}
	row := this.modelDetailsRows[id]
	model := this.QApp.LocalModels.GetModel(this.modelDetailsSelected)
	if model == nil {
		this.showModelDetailsMessage("Model not found: " + this.modelDetailsSelected)
		return
	}

	switch row.kind {
	case "model":
		this.showModelDetailsView(this.modelDetailsModelScroll)
	case "download":
		this.loadModelDetailsRemoteVariants()
		this.showModelDetailsView(this.modelDetailsDownloadView)
	case "variant":
		this.buildModelDetailsVariantView(*model, row.variantIndex)
		this.showModelDetailsView(this.modelDetailsVariantScroll)
	}
}

// showModelDetailsView shows one of the right-hand views and hides the others.
func (this *QGUI) showModelDetailsView(show fyne.CanvasObject) {
	if this.modelDetailsStack == nil {
		return
	}
	for _, obj := range this.modelDetailsStack.Objects {
		if obj == show {
			obj.Show()
		} else {
			obj.Hide()
		}
	}
}

// buildModelMetadataSection creates a section showing the model's metadata.
func (this *QGUI) buildModelMetadataSection(m definitions.Model) fyne.CanvasObject {
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

// buildModelDetailsModelView fills the "Model" view with the model's metadata.
func (this *QGUI) buildModelDetailsModelView(m definitions.Model) {
	this.modelDetailsModelView.RemoveAll()
	this.modelDetailsModelView.Add(this.buildModelMetadataSection(m))
	this.modelDetailsModelView.Refresh()
}

// buildModelDetailsVariantView fills the variant view with the metadata of the
// model's variant at the given index.
func (this *QGUI) buildModelDetailsVariantView(m definitions.Model, index int) {
	this.modelDetailsVariantView.RemoveAll()
	if index < 0 || index >= len(m.Variants) {
		this.modelDetailsVariantView.Add(wrappedLabel("Variant not found."))
		this.modelDetailsVariantView.Refresh()
		return
	}

	v := m.Variants[index]
	this.modelDetailsVariantView.Add(headingLabel("Variant: " + v.FileName))
	this.modelDetailsVariantView.Add(widget.NewSeparator())

	fields := []struct {
		label  string
		value  string
		isBool bool
	}{
		{"File Name", v.FileName, false},
		{"Description", v.Description, false},
		{"Linked Model", fmt.Sprintf("%v", v.LinkedModel), true},
		{"Location", v.Location, false},
		{"File Size", formatFileSize(v.FileSize), false},
		{"Imported At", importedAtText(v.ImportedAt), false},
		{"Supports Vision", fmt.Sprintf("%v", v.SupportsVision), true},
	}

	for _, f := range fields {
		// Boolean fields are always shown (true or false); other fields are
		// hidden when empty/zero.
		if f.isBool || f.value != "" {
			this.modelDetailsVariantView.Add(this.metadataRow(f.label, f.value))
		}
	}

	// For GGUF models, read and display the raw GGUF metadata
	if m.ModelType == string(definitions.ModelTypeGGUF) {
		this.addRawGGUFMetadata(m, v)
	}

	this.modelDetailsVariantView.Refresh()
}

// addRawGGUFMetadata reads the GGUF file's raw metadata and adds it to the variant view.
func (this *QGUI) addRawGGUFMetadata(m definitions.Model, v definitions.ModelVariant) {
	variantPath := m.VariantPath(v)
	if variantPath == "" {
		return
	}

	ggufMetadata, err := metadata.ReadGGUFMetadata(variantPath)
	if err != nil {
		// Silently skip if we can't read the metadata
		return
	}

	if ggufMetadata == nil || ggufMetadata.Raw == nil || len(ggufMetadata.Raw) == 0 {
		return
	}

	this.modelDetailsVariantView.Add(widget.NewSeparator())
	this.modelDetailsVariantView.Add(headingLabel("Raw GGUF Metadata"))

	// Format the raw metadata as a string for display
	rawText := formatRawMetadata(ggufMetadata.Raw)

	// Create a scrollable text area for the raw metadata
	textWidget := widget.NewMultiLineEntry()
	textWidget.SetText(rawText)
	textWidget.Disable() // Make it read-only
	textWidget.Wrapping = fyne.TextWrapOff
	textWidget.TextStyle = fyne.TextStyle{Monospace: true}

	// Wrap in a scroll container with a fixed height
	scroll := container.NewVScroll(textWidget)
	scroll.SetMinSize(fyne.NewSize(0, 300))

	this.modelDetailsVariantView.Add(scroll)
}

// formatRawMetadata formats the raw metadata map as a readable string.
func formatRawMetadata(raw map[string]any) string {
	if raw == nil || len(raw) == 0 {
		return "(empty)"
	}

	var keys []string
	for k := range raw {
		keys = append(keys, k)
	}

	// Sort keys for consistent output
	// Simple bubble sort since we don't want to import sort just for this
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	var lines []string
	for _, k := range keys {
		v := raw[k]
		lines = append(lines, fmt.Sprintf("%s: %v", k, formatMetadataValue(v)))
	}

	return strings.Join(lines, "\n")
}

// formatMetadataValue formats a metadata value for display.
func formatMetadataValue(v any) string {
	if v == nil {
		return "<nil>"
	}
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case []byte:
		return fmt.Sprintf("<binary: %d bytes>", len(val))
	case map[string]any:
		return fmt.Sprintf("{... %d keys ...}", len(val))
	case []any:
		return fmt.Sprintf("[... %d items ...]", len(val))
	default:
		return fmt.Sprintf("%v", val)
	}
}

// importedAtText formats an import time for display, or returns "" when the
// time is unset.
func importedAtText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// buildModelDetailsDownloadView fills the "Download" view with a table of the
// model's downloadable variants. Variants already installed locally are shown
// without a download button.
func (this *QGUI) buildModelDetailsDownloadView() {
	this.modelDetailsDownloadView.RemoveAll()

	if this.modelDetailsRemoteLoading {
		this.modelDetailsDownloadView.Add(wrappedLabel("Loading downloadable variants..."))
		this.modelDetailsDownloadView.Refresh()
		return
	}

	if len(this.modelDetailsRemoteVariants) == 0 {
		this.modelDetailsDownloadView.Add(wrappedLabel("No downloadable variants found."))
		this.modelDetailsDownloadView.Refresh()
		return
	}

	// Snapshot the variants and model so the table's length and cells always
	// agree, even if the shared fields change while it is rendered.
	variants := append([]definitions.ModelVariant(nil), this.modelDetailsRemoteVariants...)
	modelName := this.modelDetailsSelected

	table := widget.NewTable(
		func() (int, int) {
			return len(variants), 3
		},
		func() fyne.CanvasObject {
			return container.NewHBox(widget.NewButtonWithIcon("Download", theme.DownloadIcon(), nil))
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			this.updateModelDetailsDownloadCell(variants, modelName, id, cell.(*fyne.Container))
		},
	)
	table.ShowHeaderRow = true
	table.CreateHeader = func() fyne.CanvasObject {
		return widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	table.UpdateHeader = func(id widget.TableCellID, cell fyne.CanvasObject) {
		switch id.Col {
		case 0:
			cell.(*widget.Label).SetText("File")
		case 1:
			cell.(*widget.Label).SetText("Size")
		case 2:
			cell.(*widget.Label).SetText("Action")
		}
	}
	table.SetColumnWidth(0, 300)
	table.SetColumnWidth(1, 100)
	table.SetColumnWidth(2, 140)

	this.modelDetailsDownloadView.Add(table)
	this.modelDetailsDownloadView.Refresh()
}

// updateModelDetailsDownloadCell fills one cell of the downloadable-variants
// table: the file name and size, and a download button for variants that are
// not installed yet (installed variants have no button).
func (this *QGUI) updateModelDetailsDownloadCell(variants []definitions.ModelVariant, modelName string, id widget.TableCellID, cell *fyne.Container) {
	cell.RemoveAll()
	if id.Row < 0 || id.Row >= len(variants) {
		cell.Refresh()
		return
	}
	v := variants[id.Row]
	switch id.Col {
	case 0:
		label := widget.NewLabel(v.FileName)
		label.Wrapping = fyne.TextWrapOff
		cell.Add(label)
	case 1:
		label := widget.NewLabel(formatFileSize(v.FileSize))
		label.Wrapping = fyne.TextWrapOff
		cell.Add(label)
	case 2:
		if !this.modelDetailsVariantInstalled(modelName, v.FileName) {
			cell.Add(widget.NewButtonWithIcon("Download", theme.DownloadIcon(), func() {
				this.downloadHuggingFaceVariant(v.FileName)
			}))
		}
	}
	cell.Refresh()
}

// modelDetailsVariantInstalled reports whether the named model already has a
// variant with the given file name installed locally.
func (this *QGUI) modelDetailsVariantInstalled(modelName string, fileName string) bool {
	model := this.QApp.LocalModels.GetModel(modelName)
	if model == nil {
		return false
	}
	for _, v := range model.Variants {
		if v.FileName == fileName {
			return true
		}
	}
	return false
}

// isHuggingFaceModel reports whether the model came from HuggingFace, whose
// files can be fetched individually.
func isHuggingFaceModel(m definitions.Model) bool {
	return m.ModelSource == remoterepository.HuggingFaceScannerName
}

// huggingFaceModelID returns the Hub id ("org/name") of a model, taken from its
// source URL when present, or from its full model name otherwise.
func huggingFaceModelID(m definitions.Model) string {
	u := strings.TrimSpace(m.ModelURL)
	for _, prefix := range []string{
		"https://huggingface.co/",
		"http://huggingface.co/",
		"https://hf.co/",
		"http://hf.co/",
	} {
		if strings.HasPrefix(u, prefix) {
			if id := strings.Trim(strings.TrimPrefix(u, prefix), "/"); id != "" {
				return id
			}
		}
	}
	return m.FullModelName()
}

// loadModelDetailsRemoteVariants fetches the downloadable variants of the
// selected HuggingFace model the first time the Download row is opened.
func (this *QGUI) loadModelDetailsRemoteVariants() {
	if this.modelDetailsRemoteLoaded || this.modelDetailsRemoteLoading {
		return
	}
	model := this.QApp.LocalModels.GetModel(this.modelDetailsSelected)
	if model == nil || !isHuggingFaceModel(*model) {
		return
	}
	this.modelDetailsRemoteLoading = true
	this.buildModelDetailsDownloadView()
	this.fetchModelDetailsRemoteVariants(*model)
}

// fetchModelDetailsRemoteVariants fetches the downloadable variants of a
// HuggingFace model in the background and then rebuilds the download view.
func (this *QGUI) fetchModelDetailsRemoteVariants(m definitions.Model) {
	modelName := m.FullModelName()
	modelID := huggingFaceModelID(m)
	go func() {
		var variants []definitions.ModelVariant
		scanner := this.QApp.RemoteModels.GetScanner(remoterepository.HuggingFaceScannerName)
		if hfScanner, ok := scanner.(*remoterepository.HuggingFaceScanner); ok {
			if remote := hfScanner.GetModel(modelID); remote != nil {
				variants = remote.Variants
			}
		}
		fyne.Do(func() {
			// Ignore the result when the user has since picked another model.
			if this.modelDetailsSelected != modelName {
				return
			}
			this.modelDetailsRemoteLoading = false
			this.modelDetailsRemoteLoaded = true
			this.modelDetailsRemoteModel = modelName
			this.modelDetailsRemoteVariants = variants
			this.buildModelDetailsDownloadView()
		})
	}()
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
