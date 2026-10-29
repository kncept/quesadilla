package gui

import (
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/quesadilla/app"
	"github.com/kncept/quesadilla/backend/running"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// newTestQApp points HOME at a temporary directory and installs the given
// models, so the model repository scans a controlled, hermetic set.
func newTestQApp(t *testing.T, modelNames ...string) *app.QApp {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	modelsDir := path.Join(home, ".quesadilla", "models", "gguf")
	for _, name := range modelNames {
		modelDir := path.Join(modelsDir, name)
		if err := os.MkdirAll(modelDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path.Join(modelDir, name+".gguf"), []byte("stub"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return app.New()
}

func TestModelCell(t *testing.T) {
	m := modelDefinitions.Model{
		ModelName: "tiny-llama",
		ModelType: "gguf",
		ModelFile: "/models/gguf/tiny-llama/tiny-llama.gguf",
	}

	cases := []struct {
		col  int
		want string
	}{
		{0, ""}, // the actions column, not a label
		{1, "tiny-llama"},
		{2, "gguf"},
		{3, "idle"}, // not running
	}
	for _, c := range cases {
		if got := modelCell(m, c.col); got != c.want {
			t.Errorf("modelCell col %d = %q, want %q", c.col, got, c.want)
		}
	}
}

// TestModelsTableListsRepository verifies the table is sized from, and lists,
// the models held by the repository.
func TestModelsTableListsRepository(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t, "tiny-llama", "big-llama")
	g := CreateGui(nil, qApp)

	table := g.newModelsTable()
	rows, cols := table.Length()
	if rows != 2 {
		t.Fatalf("table rows = %d, want 2", rows)
	}
	if cols != len(modelColumns) {
		t.Fatalf("table cols = %d, want %d", cols, len(modelColumns))
	}
	if !table.ShowHeaderRow {
		t.Error("expected the table to show a header row")
	}

	// The name column is populated from the repository.
	names := map[string]bool{}
	for i := 0; i < rows; i++ {
		names[tableCellText(table, i, 1)] = true
	}
	for _, want := range []string{"tiny-llama", "big-llama"} {
		if !names[want] {
			t.Errorf("table missing model %q (got %v)", want, names)
		}
	}
}

// TestModelsTableActionsFirst verifies the row actions live in the leftmost
// column: the start/stop, delete and logs buttons are all in column 0.
func TestModelsTableActionsFirst(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t, "tiny-llama")
	g := CreateGui(nil, qApp)
	table := g.newModelsTable()

	if got := modelColumns[0]; got != "Actions" {
		t.Fatalf("first column = %q, want %q", got, "Actions")
	}

	buttons := tableCellButtons(table, 0, 0)
	if findButtonByText(buttons, "Start") == nil {
		t.Error("expected a start/stop button in the first column")
	}
	if findButtonByIconName(buttons, "delete") == nil {
		t.Error("expected a delete button in the first column")
	}
	if findButtonByText(buttons, "Logs") == nil {
		t.Error("expected a logs button in the first column")
	}
}

// TestModelsTableRunningColumn verifies the Status column reflects the
// process-wide running registry.
func TestModelsTableRunningColumn(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t, "tiny-llama")

	g := CreateGui(nil, qApp)
	table := g.newModelsTable()

	statusCol := 3

	if got := tableCellText(table, 0, statusCol); got != "idle" {
		t.Errorf("running column = %q, want %q", got, "idle")
	}

	stop := running.Default().Track(&fakeRunningModel{name: "tiny-llama"})
	defer stop()

	if got := tableCellText(table, 0, statusCol); got != "running" {
		t.Errorf("running column = %q, want %q", got, "running")
	}
}

// TestModelsTableActionButton verifies the actions column shows a Start button
// for models that are not running, a Stop button for models that are, and
// that tapping Stop asks the running model to quit.
func TestModelsTableActionButton(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t, "tiny-llama")
	g := CreateGui(nil, qApp)
	table := g.newModelsTable()

	actionCol := 0

	// not running: a Start button
	button := findButtonByText(tableCellButtons(table, 0, actionCol), "Start")
	if button == nil {
		t.Fatal("expected a Start button in the actions column")
	}

	// running: a Stop button that signals the model
	fake := &fakeRunningModel{name: "tiny-llama"}
	stop := running.Default().Track(fake)
	defer stop()

	button = findButtonByText(tableCellButtons(table, 0, actionCol), "Stop")
	if button == nil {
		t.Fatal("expected a Stop button in the actions column")
	}

	button.Tapped(nil)
	if fake.quits != 1 {
		t.Errorf("stop tapped = %d, want 1", fake.quits)
	}
}

// TestModelsTableStartButtonWithoutBackend verifies tapping Start when no
// installed backend can run the model type reports an error instead of
// panicking. With a temp HOME, the llama backend has no installed versions.
func TestModelsTableStartButtonWithoutBackend(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t, "tiny-llama")
	g := CreateGui(nil, qApp)
	table := g.newModelsTable()

	button := findButtonByText(tableCellButtons(table, 0, 0), "Start")
	if button == nil {
		t.Fatal("expected a Start button in the actions column")
	}

	// must not panic, and must not have started anything
	button.Tapped(nil)
	if got := g.QApp.RunningModels(); len(got) != 0 {
		t.Errorf("running models = %v, want none", got)
	}
}

// TestModelsTableDeleteButtonConfirms verifies the actions column carries a
// delete button that is not a danger (bright red) button, and that it does not
// remove the model until the user accepts a confirmation dialog.
func TestModelsTableDeleteButtonConfirms(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t, "tiny-llama")
	g := CreateGui(nil, qApp)
	table := g.newModelsTable()

	win := test.NewWindow(table)
	win.Resize(fyne.NewSize(900, 600))
	defer win.Close()

	g.mu.Lock()
	g.mainWin = win
	g.mu.Unlock()

	del := findButtonByIconName(tableCellButtons(table, 0, 0), "delete")
	if del == nil {
		t.Fatal("expected a delete button in the actions column")
	}
	if del.Importance == widget.DangerImportance {
		t.Error("delete button should not use the danger (bright red) importance")
	}

	// tapping the delete button only asks for confirmation
	del.Tapped(nil)
	if qApp.LocalModels.GetModel("tiny-llama") == nil {
		t.Fatal("model was deleted before the confirmation was accepted")
	}

	// the confirmation dialog is showing over the window
	overlay := win.Canvas().Overlays().Top()
	if overlay == nil {
		t.Fatal("expected a confirmation dialog to be shown")
	}
	confirm := findButtonByText(findButtons(overlay, nil), "Yes")
	if confirm == nil {
		t.Fatal("expected a confirm button in the dialog")
	}

	// accepting the confirmation deletes the model
	confirm.Tapped(nil)
	if qApp.LocalModels.GetModel("tiny-llama") != nil {
		t.Fatal("model was not deleted after the confirmation was accepted")
	}
}

// TestModelsTableDeleteButtonCancelKeepsModel verifies cancelling the
// confirmation dialog leaves the model installed.
func TestModelsTableDeleteButtonCancelKeepsModel(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t, "tiny-llama")
	g := CreateGui(nil, qApp)
	table := g.newModelsTable()

	win := test.NewWindow(table)
	win.Resize(fyne.NewSize(900, 600))
	defer win.Close()

	g.mu.Lock()
	g.mainWin = win
	g.mu.Unlock()

	del := findButtonByIconName(tableCellButtons(table, 0, 0), "delete")
	if del == nil {
		t.Fatal("expected a delete button in the actions column")
	}
	del.Tapped(nil)

	cancel := findButtonByText(findButtons(win.Canvas().Overlays().Top(), nil), "No")
	if cancel == nil {
		t.Fatal("expected a dismiss button in the dialog")
	}
	cancel.Tapped(nil)

	if qApp.LocalModels.GetModel("tiny-llama") == nil {
		t.Fatal("model was deleted even though the confirmation was cancelled")
	}
}

// TestModelsTableLogsButtonDisabledUnlessRunning verifies the actions column's
// logs button is disabled while the model is not running and enabled once it
// is.
func TestModelsTableLogsButtonDisabledUnlessRunning(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t, "tiny-llama")
	g := CreateGui(nil, qApp)
	table := g.newModelsTable()

	// not running: disabled
	logs := findButtonByText(tableCellButtons(table, 0, 0), "Logs")
	if logs == nil {
		t.Fatal("expected a logs button in the actions column")
	}
	if !logs.Disabled() {
		t.Error("logs button should be disabled while the model is not running")
	}

	// running: enabled
	stop := running.Default().Track(&fakeRunningModel{name: "tiny-llama"})
	defer stop()

	logs = findButtonByText(tableCellButtons(table, 0, 0), "Logs")
	if logs == nil {
		t.Fatal("expected a logs button in the actions column")
	}
	if logs.Disabled() {
		t.Error("logs button should be enabled while the model is running")
	}
}

// TestModelsTableOpenButton verifies the actions column shows an Open button
// for models whose metadata records a page (and a tap on it opens that page,
// routed through the openURL hook), and no such button for models whose
// metadata does not.
func TestModelsTableOpenButton(t *testing.T) {
	test.NewApp()
	home := t.TempDir()
	t.Setenv("HOME", home)

	modelsDir := path.Join(home, ".quesadilla", "models", "gguf")

	// a model whose metadata records its page at the source
	pagedDir := path.Join(modelsDir, "org", "paged")
	if err := os.MkdirAll(pagedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(pagedDir, "paged.gguf"), []byte("stub"), 0644); err != nil {
		t.Fatal(err)
	}
	qmodel := `{
		"ModelFamily": "org",
		"ModelName": "paged",
		"ModelType": "gguf",
		"ModelURL": "https://huggingface.co/org/paged",
		"Variants": [{"FileName": "paged.gguf", "ImportedAt": "2024-01-01T00:00:00Z", "FileSize": 4}]
	}`
	if err := os.WriteFile(path.Join(pagedDir, "qmodel.json"), []byte(qmodel), 0644); err != nil {
		t.Fatal(err)
	}

	// a model whose metadata records no page
	bareDir := path.Join(modelsDir, "none", "bare")
	if err := os.MkdirAll(bareDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(bareDir, "bare.gguf"), []byte("stub"), 0644); err != nil {
		t.Fatal(err)
	}

	g := CreateGui(nil, app.New())
	opened := ""
	g.openURL = func(u *url.URL) error {
		opened = u.String()
		return nil
	}
	table := g.newModelsTable()

	rows, _ := table.Length()
	if rows != 2 {
		t.Fatalf("table rows = %d, want 2", rows)
	}

	// the paged model: start/stop, delete, logs and an Open button
	pagedRow, bareRow := -1, -1
	for i := 0; i < rows; i++ {
		switch tableCellText(table, i, 1) {
		case "org/paged":
			pagedRow = i
		case "bare":
			bareRow = i
		}
	}
	if pagedRow < 0 || bareRow < 0 {
		t.Fatalf("expected both models in the table (rows = %d)", rows)
	}

	pagedButtons := tableCellButtons(table, pagedRow, 0)
	if len(pagedButtons) != 5 {
		t.Fatalf("paged row buttons = %d, want 5 (start/stop, delete, logs, Open, Details)", len(pagedButtons))
	}
	open := findButtonByText(pagedButtons, "Open")
	if open == nil {
		t.Fatal("expected an Open button in the paged model's actions")
	}
	open.Tapped(nil)
	if opened != "https://huggingface.co/org/paged" {
		t.Errorf("opened URL = %q, want the model's page", opened)
	}

	// the model without a page: no Open button, but a Details button
	bareButtons := tableCellButtons(table, bareRow, 0)
	if len(bareButtons) != 4 {
		t.Fatalf("bare row buttons = %d, want 4 (start/stop, delete, logs, Details)", len(bareButtons))
	}
	if findButtonByText(bareButtons, "Open") != nil {
		t.Error("did not expect an Open button on a model without a page")
	}
	if findButtonByText(bareButtons, "Details") == nil {
		t.Error("expected a Details button on every model's actions")
	}
}

// TestFormatUptime verifies the uptime rendering for the running-models
// section.
func TestFormatUptime(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{7*time.Minute + 30*time.Second, "7m 30s"},
		{2*time.Hour + 5*time.Minute + 9*time.Second, "2h 5m"},
	}
	for _, c := range cases {
		if got := formatUptime(c.in); got != c.want {
			t.Errorf("formatUptime(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestOverviewRunningModelsSection verifies the Overview's running-models
// section: the placeholder shows when nothing is running, and the table (with
// its stats and stop button) shows when models are.
func TestOverviewRunningModelsSection(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t)
	g := CreateGui(nil, qApp)
	g.overviewPage()

	if g.runningModelsTable == nil || g.noRunningLabel == nil {
		t.Fatal("expected the running-models section to be built")
	}

	// nothing running: placeholder visible, table hidden
	if !g.noRunningLabel.Visible() {
		t.Error("expected the 'no models running' placeholder to be visible")
	}
	if g.runningModelsTable.Visible() {
		t.Error("expected the running-models table to be hidden")
	}

	// a model starts running: table visible with its stats and a stop button
	fake := &fakeRunningModel{name: "tiny-llama"}
	stop := running.Default().Track(fake)
	defer stop()
	g.refreshDynamicPages()

	if !g.runningModelsTable.Visible() {
		t.Error("expected the running-models table to be visible")
	}
	if g.noRunningLabel.Visible() {
		t.Error("expected the placeholder to be hidden")
	}

	rows, cols := g.runningModelsTable.Length()
	if rows != 1 {
		t.Fatalf("table rows = %d, want 1", rows)
	}
	if cols != len(runningModelColumns) {
		t.Fatalf("table cols = %d, want %d", cols, len(runningModelColumns))
	}
	if got := tableCellText(g.runningModelsTable, 0, 0); got != "tiny-llama" {
		t.Errorf("model column = %q, want %q", got, "tiny-llama")
	}
	if got := tableCellText(g.runningModelsTable, 0, 3); got != "7m 0s" {
		t.Errorf("uptime column = %q, want %q", got, "7m 0s")
	}

	actionCol := len(runningModelColumns) - 1
	button := tableCellButton(g.runningModelsTable, 0, actionCol)
	if button == nil {
		t.Fatal("expected a stop button in the action column")
	}
	if got := button.Text; got != "Stop" {
		t.Errorf("action button = %q, want %q", got, "Stop")
	}
	button.Tapped(nil)
	if fake.quits != 1 {
		t.Errorf("stop tapped = %d, want 1", fake.quits)
	}
}

// fakeRunningModel is a minimal runnerDefinitions.RunningModel for tests.
// The GUI reads its name and uptime; SendSigQuit records how often it was
// signalled so tests can verify the stop actions.
type fakeRunningModel struct {
	mu    sync.Mutex
	name  string
	quits int
	logs  []string
}

// ModelName implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) ModelName() string { return f.name }

// ProviderName implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) ProviderName() string { return "" }

// RuntimeVersion implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) RuntimeVersion() string { return "" }

// Uptime implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) Uptime() time.Duration { return 7 * time.Minute }

// Logs implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) Logs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.logs))
	copy(out, f.logs)
	return out
}

// addLog appends a line to the fake's captured logs. It exists so a test can
// feed new output while the log viewer is open; the mutex keeps it safe for
// the viewer's refresh goroutine to read concurrently.
func (f *fakeRunningModel) addLog(line string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, line)
}

// Wait implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) Wait() {}

// SendSigQuit implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) SendSigQuit() { f.quits++ }

// SendSigKill implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) SendSigKill() {}

// Log implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) Log(line string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, line)
}

var _ runnerDefinitions.RunningModel = (*fakeRunningModel)(nil)

// tableCellObject renders one data cell and returns the widget it holds. It
// handles both the plain label cells (backends table) and the container
// cells (model tables) that hold a label or a button.
func tableCellObject(table *widget.Table, row, col int) fyne.CanvasObject {
	cell := table.CreateCell()
	table.UpdateCell(widget.TableCellID{Row: row, Col: col}, cell)
	switch c := cell.(type) {
	case *fyne.Container:
		if len(c.Objects) != 1 {
			return nil
		}
		return c.Objects[0]
	default:
		return cell
	}
}

// tableCellText renders one data cell and returns its label text.
func tableCellText(table *widget.Table, row, col int) string {
	label, ok := tableCellObject(table, row, col).(*widget.Label)
	if !ok {
		return ""
	}
	return label.Text
}

// tableCellButton renders one data cell and returns its button.
func tableCellButton(table *widget.Table, row, col int) *widget.Button {
	button, ok := tableCellObject(table, row, col).(*widget.Button)
	if !ok {
		return nil
	}
	return button
}

// findButtonByText returns the button carrying the given label, or nil.
func findButtonByText(buttons []*widget.Button, text string) *widget.Button {
	for _, b := range buttons {
		if b.Text == text {
			return b
		}
	}
	return nil
}

// findButtonByIconName returns the button whose icon name contains substr
// (case-insensitively), or nil.
func findButtonByIconName(buttons []*widget.Button, substr string) *widget.Button {
	for _, b := range buttons {
		if b.Icon != nil && strings.Contains(strings.ToLower(b.Icon.Name()), substr) {
			return b
		}
	}
	return nil
}
