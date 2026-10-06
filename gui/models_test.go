package gui

import (
	"os"
	"path"
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
		{0, "tiny-llama"},
		{1, "gguf"},
		{2, ""}, // not running
		{3, "/models/gguf/tiny-llama/tiny-llama.gguf"},
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
		names[tableCellText(table, i, 0)] = true
	}
	for _, want := range []string{"tiny-llama", "big-llama"} {
		if !names[want] {
			t.Errorf("table missing model %q (got %v)", want, names)
		}
	}
}

// TestModelsTableRunningColumn verifies the Running column reflects the
// process-wide running registry.
func TestModelsTableRunningColumn(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t, "tiny-llama")

	g := CreateGui(nil, qApp)
	table := g.newModelsTable()

	if got := tableCellText(table, 0, 2); got != "" {
		t.Errorf("running column = %q, want empty", got)
	}

	stop := running.Default().Track(&fakeRunningModel{name: "tiny-llama"})
	defer stop()

	if got := tableCellText(table, 0, 2); got != "running" {
		t.Errorf("running column = %q, want %q", got, "running")
	}
}

// TestModelsTableActionButton verifies the action column shows a Start button
// for models that are not running, a Stop button for models that are, and
// that tapping Stop asks the running model to quit.
func TestModelsTableActionButton(t *testing.T) {
	test.NewApp()
	qApp := newTestQApp(t, "tiny-llama")
	g := CreateGui(nil, qApp)
	table := g.newModelsTable()

	actionCol := len(modelColumns) - 1

	// not running: a Start button
	button := tableCellButton(table, 0, actionCol)
	if button == nil {
		t.Fatal("expected a button in the action column")
	}
	if got := button.Text; got != "Start" {
		t.Errorf("action button = %q, want %q", got, "Start")
	}

	// running: a Stop button that signals the model
	fake := &fakeRunningModel{name: "tiny-llama"}
	stop := running.Default().Track(fake)
	defer stop()

	button = tableCellButton(table, 0, actionCol)
	if button == nil {
		t.Fatal("expected a button in the action column")
	}
	if got := button.Text; got != "Stop" {
		t.Errorf("action button = %q, want %q", got, "Stop")
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

	button := tableCellButton(table, 0, len(modelColumns)-1)
	if button == nil {
		t.Fatal("expected a button in the action column")
	}
	if got := button.Text; got != "Start" {
		t.Fatalf("action button = %q, want %q", got, "Start")
	}

	// must not panic, and must not have started anything
	button.Tapped(nil)
	if got := g.QApp.RunningModels(); len(got) != 0 {
		t.Errorf("running models = %v, want none", got)
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
	name  string
	quits int
}

// ModelName implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) ModelName() string { return f.name }

// ProviderName implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) ProviderName() string { return "" }

// RuntimeVersion implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) RuntimeVersion() string { return "" }

// Uptime implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) Uptime() time.Duration { return 7 * time.Minute }

// Wait implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) Wait() {}

// SendSigQuit implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) SendSigQuit() { f.quits++ }

// SendSigKill implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) SendSigKill() {}

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
