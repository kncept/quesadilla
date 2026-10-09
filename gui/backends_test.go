package gui

import (
	"path/filepath"
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/quesadilla/app"
	"github.com/kncept/quesadilla/backend"
	backendDefinitions "github.com/kncept/quesadilla/backend/definitions"
	"github.com/kncept/quesadilla/config"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// fakeBackend is a definitions.Backend with canned version lists, used to
// test the backends screen without touching the network.
type fakeBackend struct {
	id          string
	description string
	modelTypes  []string
	installed   []string
	installable []string

	// installableCalls counts calls to InstallableVersions, so tests can
	// assert the GUI does not fetch from the network.
	installableCalls int
}

func (f *fakeBackend) Id() string                  { return f.id }
func (f *fakeBackend) Description() string         { return f.description }
func (f *fakeBackend) ModelTypes() []string        { return f.modelTypes }
func (f *fakeBackend) InstalledVersions() []string { return f.installed }
func (f *fakeBackend) InstallableVersions() []string {
	f.installableCalls++
	return f.installable
}
func (f *fakeBackend) LatestVersion() string       { return "" }
func (f *fakeBackend) InstallVersion(string) error { return nil }
func (f *fakeBackend) RemoveVersion(string) error  { return nil }
func (f *fakeBackend) Start(*modelDefinitions.Model) (runnerDefinitions.RunningModel, error) {
	return nil, nil
}

var _ backendDefinitions.Backend = (*fakeBackend)(nil)

// newBackendsGui builds a QGUI whose backend repository holds the given
// backends, with the order persisted to a temporary config file.
func newBackendsGui(t *testing.T, backends []backendDefinitions.Backend) (*QGUI, *backend.Repository) {
	t.Helper()
	repo := backend.NewRepositoryWithConfig(backends, filepath.Join(t.TempDir(), "config.json"))
	return CreateGui(nil, &app.QApp{Backends: repo}), repo
}

func TestFormatVersionList(t *testing.T) {
	if got := formatVersionList(nil, noneInstalled); got != noneInstalled {
		t.Errorf("empty versions = %q, want fallback", got)
	}
	got := formatVersionList([]string{"b1", "b2"}, noneInstalled)
	if want := "b1, b2"; got != want {
		t.Errorf("versions = %q, want %q", got, want)
	}
}

func TestBackendCell(t *testing.T) {
	b := &fakeBackend{
		id:         "llama",
		modelTypes: []string{"gguf"},
		installed:  []string{"b11401"},
	}
	entry := config.BackendOrderEntry{ModelType: "gguf", BackendId: "llama"}

	cases := []struct {
		isDefault bool
		col       int
		want      string
	}{
		{false, 0, "gguf"},
		{false, 1, "llama"},
		{true, 1, "llama (default)"},
		{false, 2, "b11401"},
	}
	for _, c := range cases {
		if got := backendCell(entry, b, c.isDefault, c.col); got != c.want {
			t.Errorf("backendCell col %d = %q, want %q", c.col, got, c.want)
		}
	}
}

// TestBackendCellNilBackend verifies a row whose backend id no longer resolves
// still renders, with the no-versions fallback.
func TestBackendCellNilBackend(t *testing.T) {
	entry := config.BackendOrderEntry{ModelType: "gguf", BackendId: "gone"}
	if got := backendCell(entry, nil, false, 2); got != noneInstalled {
		t.Errorf("versions = %q, want %q", got, noneInstalled)
	}
}

// TestBackendsTableListsOrder verifies the table is sized from, and lists in
// order, the repository's (model type, backend) entries: model types grouped,
// a multi-type backend once per type, and the topmost backend of each group
// marked as the default.
func TestBackendsTableListsOrder(t *testing.T) {
	test.NewApp()

	g, repo := newBackendsGui(t, []backendDefinitions.Backend{
		&fakeBackend{id: "llama", modelTypes: []string{"gguf"}},
		&fakeBackend{id: "other", modelTypes: []string{"gguf", "onnx"}},
	})

	want := []config.BackendOrderEntry{
		{ModelType: "gguf", BackendId: "llama"},
		{ModelType: "gguf", BackendId: "other"},
		{ModelType: "onnx", BackendId: "other"},
	}
	if got := repo.BackendOrder(); !reflect.DeepEqual(got, want) {
		t.Fatalf("repository order = %v, want %v", got, want)
	}

	table := g.newBackendsTable()
	rows, cols := table.Length()
	if rows != len(want) {
		t.Fatalf("table rows = %d, want %d", rows, len(want))
	}
	if cols != len(backendColumns) {
		t.Fatalf("table cols = %d, want %d", cols, len(backendColumns))
	}
	if !table.ShowHeaderRow {
		t.Error("expected the table to show a header row")
	}

	for i, e := range want {
		if got := tableCellText(table, i, 0); got != e.ModelType {
			t.Errorf("row %d model type = %q, want %q", i, got, e.ModelType)
		}
	}
	if got := tableCellText(table, 0, 1); got != "llama (default)" {
		t.Errorf("row 0 backend = %q, want %q", got, "llama (default)")
	}
	if got := tableCellText(table, 1, 1); got != "other" {
		t.Errorf("row 1 backend = %q, want %q", got, "other")
	}
	if got := tableCellText(table, 2, 1); got != "other (default)" {
		t.Errorf("row 2 backend = %q, want %q", got, "other (default)")
	}
}

// TestBackendsTableDoesNotFetchInstallable verifies the GUI never calls the
// network-backed InstallableVersions while building or rendering the table.
func TestBackendsTableDoesNotFetchInstallable(t *testing.T) {
	test.NewApp()

	fake := &fakeBackend{id: "llama", modelTypes: []string{"gguf"}}
	g, _ := newBackendsGui(t, []backendDefinitions.Backend{fake})
	table := g.newBackendsTable()

	// Render every cell in the table.
	rows, cols := table.Length()
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			_ = tableCellText(table, r, c)
		}
	}

	if fake.installableCalls != 0 {
		t.Errorf("InstallableVersions called %d times, want 0", fake.installableCalls)
	}
}

// TestBackendsTableMoveButtonsDisabledAtGroupBounds verifies the move up/down
// buttons are only enabled between rows of the same model type group.
func TestBackendsTableMoveButtonsDisabledAtGroupBounds(t *testing.T) {
	test.NewApp()

	g, _ := newBackendsGui(t, []backendDefinitions.Backend{
		&fakeBackend{id: "llama", modelTypes: []string{"gguf"}},
		&fakeBackend{id: "gogguf", modelTypes: []string{"gguf"}},
		&fakeBackend{id: "other", modelTypes: []string{"onnx"}},
	})
	table := g.newBackendsTable()
	actionCol := len(backendColumns) - 1

	cases := []struct {
		row          int
		upDisabled   bool
		downDisabled bool
	}{
		{0, true, false}, // top of the gguf group
		{1, false, true}, // bottom of the gguf group
		{2, true, true},  // the only onnx row
	}
	for _, c := range cases {
		btns := tableCellButtons(table, c.row, actionCol)
		if len(btns) != 2 {
			t.Fatalf("row %d has %d action buttons, want 2", c.row, len(btns))
		}
		if btns[0].Disabled() != c.upDisabled {
			t.Errorf("row %d up disabled = %v, want %v", c.row, btns[0].Disabled(), c.upDisabled)
		}
		if btns[1].Disabled() != c.downDisabled {
			t.Errorf("row %d down disabled = %v, want %v", c.row, btns[1].Disabled(), c.downDisabled)
		}
	}
}

// TestBackendsTableMoveUpdatesOrderAndView verifies tapping move up reorders
// the repository's order (persisting it to the config) and re-renders the
// table, with the new topmost backend of the group marked as the default.
func TestBackendsTableMoveUpdatesOrderAndView(t *testing.T) {
	test.NewApp()

	g, repo := newBackendsGui(t, []backendDefinitions.Backend{
		&fakeBackend{id: "llama", modelTypes: []string{"gguf"}},
		&fakeBackend{id: "gogguf", modelTypes: []string{"gguf"}},
		&fakeBackend{id: "other", modelTypes: []string{"onnx"}},
	})
	table := g.newBackendsTable()
	actionCol := len(backendColumns) - 1

	// Move gogguf (row 1) up, over llama (row 0).
	btns := tableCellButtons(table, 1, actionCol)
	btns[0].Tapped(nil)

	want := []config.BackendOrderEntry{
		{ModelType: "gguf", BackendId: "gogguf"},
		{ModelType: "gguf", BackendId: "llama"},
		{ModelType: "onnx", BackendId: "other"},
	}
	if got := repo.BackendOrder(); !reflect.DeepEqual(got, want) {
		t.Fatalf("repository order after move = %v, want %v", got, want)
	}

	// The tap refreshed the table: the row is re-rendered from the new order.
	if got := tableCellText(table, 0, 1); got != "gogguf (default)" {
		t.Errorf("row 0 backend after move = %q, want %q", got, "gogguf (default)")
	}
	if got := tableCellText(table, 1, 1); got != "llama" {
		t.Errorf("row 1 backend after move = %q, want %q", got, "llama")
	}

	// Moving gogguf up again is a no-op: it tops its group.
	if got := tableCellButtons(table, 0, actionCol)[0].Disabled(); !got {
		t.Error("expected the up button to be disabled on the group's new top row")
	}
}

// tableCellButtons renders one data cell and returns the buttons of its
// action box.
func tableCellButtons(table *widget.Table, row, col int) []*widget.Button {
	obj := tableCellObject(table, row, col)
	box, ok := obj.(*fyne.Container)
	if !ok {
		return nil
	}
	var buttons []*widget.Button
	for _, o := range box.Objects {
		if b, ok := o.(*widget.Button); ok {
			buttons = append(buttons, b)
		}
	}
	return buttons
}
