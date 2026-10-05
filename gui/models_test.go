package gui

import (
	"os"
	"path"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/quesadilla/app"
	"github.com/kncept/quesadilla/backend/running"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
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

	table := g.modelsTable()
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

	stop := running.Default().Track(modelDefinitions.Model{ModelName: "tiny-llama", ModelType: "gguf"})
	defer stop()

	g := CreateGui(nil, qApp)
	table := g.modelsTable()

	if got := tableCellText(table, 0, 2); got != "yes" {
		t.Errorf("running column = %q, want %q", got, "yes")
	}
}

// tableCellText renders one data cell and returns its label text.
func tableCellText(table *widget.Table, row, col int) string {
	cell := table.CreateCell()
	table.UpdateCell(widget.TableCellID{Row: row, Col: col}, cell)
	return cell.(*widget.Label).Text
}
