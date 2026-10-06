package gui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	backendDefinitions "github.com/kncept/quesadilla/backend/definitions"
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

	cases := []struct {
		col  int
		want string
	}{
		{0, "llama"},
		{1, "gguf"},
		{2, "b11401"},
	}
	for _, c := range cases {
		if got := backendCell(b, c.col); got != c.want {
			t.Errorf("backendCell col %d = %q, want %q", c.col, got, c.want)
		}
	}
}

// TestBackendsTableListsBackends verifies the table is sized from, and lists,
// the backends passed to it.
func TestBackendsTableListsBackends(t *testing.T) {
	test.NewApp()

	backends := []backendDefinitions.Backend{
		&fakeBackend{id: "llama", modelTypes: []string{"gguf"}},
		&fakeBackend{id: "other", modelTypes: []string{"onnx"}},
	}

	table := newBackendsTable(backends)
	rows, cols := table.Length()
	if rows != 2 {
		t.Fatalf("table rows = %d, want 2", rows)
	}
	if cols != len(backendColumns) {
		t.Fatalf("table cols = %d, want %d", cols, len(backendColumns))
	}
	if !table.ShowHeaderRow {
		t.Error("expected the table to show a header row")
	}

	ids := map[string]bool{}
	for i := 0; i < rows; i++ {
		ids[tableCellText(table, i, 0)] = true
	}
	for _, want := range []string{"llama", "other"} {
		if !ids[want] {
			t.Errorf("table missing backend %q (got %v)", want, ids)
		}
	}
}

// TestBackendsTableDoesNotFetchInstallable verifies the GUI never calls the
// network-backed InstallableVersions while building or rendering the table.
func TestBackendsTableDoesNotFetchInstallable(t *testing.T) {
	test.NewApp()

	fake := &fakeBackend{id: "llama", modelTypes: []string{"gguf"}}
	table := newBackendsTable([]backendDefinitions.Backend{fake})

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
