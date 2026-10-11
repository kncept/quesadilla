package remoterepository

import (
	"os"
	"path/filepath"
	"testing"
)

// writeGGUF creates a stub .gguf file (with the given contents) at dir/name.
func writeGGUF(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("stub"), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestFilesystemScanDepthZero verifies a depth of 0 searches only the scan
// root: gguf files in the root form a model, subdirectories are not entered.
func TestFilesystemScanDepthZero(t *testing.T) {
	root := t.TempDir()
	writeGGUF(t, root, "root.gguf")
	modelsDir := filepath.Join(root, "sub")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeGGUF(t, modelsDir, "sub.gguf")

	scanner := NewFilesystemScanner()
	scanner.Configure(root, 0)

	models, err := scanner.ScanForModels()
	if err != nil {
		t.Fatalf("ScanForModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("models = %d, want 1 (only the root holds an .gguf at depth 0)", len(models))
	}
	if models[0].FullModelName() != filepath.Base(root) {
		t.Errorf("model name = %q, want the root's base name %q", models[0].FullModelName(), filepath.Base(root))
	}
	if len(models[0].Variants) != 1 || models[0].Variants[0].FileName != "root.gguf" {
		t.Errorf("variants = %+v, want the root's single root.gguf", models[0].Variants)
	}
}

// TestFilesystemScanDepthDescends verifies the depth controls how far below
// the root the walk descends: depth 1 finds each of the root's immediate
// subdirectories, depth 2 reaches a grandchild.
func TestFilesystemScanDepthDescends(t *testing.T) {
	root := t.TempDir()
	writeGGUF(t, root, "root.gguf")

	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	writeGGUF(t, filepath.Join(root, "a"), "a.gguf")
	writeGGUF(t, nested, "b.gguf")

	cases := []struct {
		depth int
		want  []string
	}{
		{0, nil},
		{1, []string{"a"}},
		{2, []string{"a", "a/b"}},
	}
	for _, c := range cases {
		scanner := NewFilesystemScanner()
		scanner.Configure(root, c.depth)
		t.Run("depth-"+string(rune('0'+c.depth)), func(t *testing.T) {
			models, err := scanner.ScanForModels()
			if err != nil {
				t.Fatalf("ScanForModels: %v", err)
			}
			got := map[string]bool{}
			for _, m := range models {
				got[m.FullModelName()] = true
			}
			// the root holds an .gguf, so it is a model at every depth
			if !got[filepath.Base(root)] {
				t.Errorf("missing the root model %q", filepath.Base(root))
			}
			for _, want := range c.want {
				if !got[want] {
					t.Errorf("missing %q at depth %d (have %v)", want, c.depth, got)
				}
			}
			unexpected := 0
			for have := range got {
				if have != filepath.Base(root) {
					found := false
					for _, want := range c.want {
						if have == want {
							found = true
							break
						}
					}
					if !found {
						unexpected++
					}
				}
			}
			if unexpected > 0 {
				t.Errorf("found %d unexpected models at depth %d (have %v)", unexpected, c.depth, got)
			}
		})
	}
}

// TestFilesystemScanUnlimited verifies the unlimited depth reaches every level
// below the root.
func TestFilesystemScanUnlimited(t *testing.T) {
	root := t.TempDir()
	writeGGUF(t, root, "root.gguf")
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	writeGGUF(t, filepath.Join(root, "a"), "a.gguf")
	writeGGUF(t, filepath.Join(root, "a", "b"), "b.gguf")
	writeGGUF(t, nested, "c.gguf")

	scanner := NewFilesystemScanner()
	scanner.Configure(root, FilesystemDepthUnlimited)

	models, err := scanner.ScanForModels()
	if err != nil {
		t.Fatalf("ScanForModels: %v", err)
	}
	got := map[string]bool{}
	for _, m := range models {
		got[m.FullModelName()] = true
	}
	for _, want := range []string{filepath.Base(root), "a", "a/b", "a/b/c"} {
		if !got[want] {
			t.Errorf("missing %q under unlimited depth (have %v)", want, got)
		}
	}
}

// TestFilesystemScanGroupsVariants verifies a directory holding several .gguf
// files is a single model whose variants are the files.
func TestFilesystemScanGroupsVariants(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "model")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	writeGGUF(t, dir, "model-q4.gguf")
	writeGGUF(t, dir, "model-q8.gguf")

	scanner := NewFilesystemScanner()
	scanner.Configure(root, 1)

	models, err := scanner.ScanForModels()
	if err != nil {
		t.Fatalf("ScanForModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("models = %d, want 1", len(models))
	}
	m := models[0]
	if m.FullModelName() != "model" {
		t.Errorf("model name = %q, want model", m.FullModelName())
	}
	if len(m.Variants) != 2 {
		t.Fatalf("variants = %d, want 2", len(m.Variants))
	}
	if m.Variants[0].FileName != "model-q4.gguf" || m.Variants[1].FileName != "model-q8.gguf" {
		t.Errorf("variants = %+v, want both quantizations sorted", m.Variants)
	}
}

// TestFilesystemScanDistinguishesSameNamedDirs verifies two same-named
// directories under different parents become distinct models, named by their
// relative paths.
func TestFilesystemScanDistinguishesSameNamedDirs(t *testing.T) {
	root := t.TempDir()
	for _, sub := range []string{"one", "two"} {
		dir := filepath.Join(root, sub, "model")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		writeGGUF(t, dir, sub+".gguf")
	}

	scanner := NewFilesystemScanner()
	scanner.Configure(root, FilesystemDepthUnlimited)

	models, err := scanner.ScanForModels()
	if err != nil {
		t.Fatalf("ScanForModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("models = %d, want 2", len(models))
	}
	for _, want := range []string{"one/model", "two/model"} {
		if scanner.GetModel(want) == nil {
			t.Errorf("expected the scanner to resolve %q", want)
		}
	}
}

// TestFilesystemGetModel verifies GetModel re-walks and returns the model by
// name, and nil when there is no such model.
func TestFilesystemGetModel(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "model")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	writeGGUF(t, dir, "model.gguf")

	scanner := NewFilesystemScanner()
	scanner.Configure(root, 1)

	m := scanner.GetModel("model")
	if m == nil {
		t.Fatal("expected GetModel to find the scanned model")
	}
	if m.ModelSource != FilesystemScannerName {
		t.Errorf("model source = %q, want %q", m.ModelSource, FilesystemScannerName)
	}
	if len(m.Variants) != 1 {
		t.Errorf("variants = %d, want 1", len(m.Variants))
	}
	if scanner.GetModel("missing") != nil {
		t.Error("expected nil for an unknown model")
	}
}

// TestFilesystemConfigureMakesRootAbsolute verifies a relative directory is
// stored absolute, so the locations of the models it finds stay valid
// wherever the process runs.
func TestFilesystemConfigureMakesRootAbsolute(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "model")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	writeGGUF(t, dir, "model.gguf")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(wd, root)
	if err != nil {
		t.Fatal(err)
	}

	scanner := NewFilesystemScanner()
	scanner.Configure(rel, 1)
	models, err := scanner.ScanForModels()
	if err != nil {
		t.Fatalf("ScanForModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("models = %d, want 1", len(models))
	}
	if want := filepath.Join(root, "model", "model.gguf"); models[0].Variants[0].Location != want {
		t.Errorf("variant location = %q, want the absolute %q", models[0].Variants[0].Location, want)
	}
}

// TestFilesystemExpandHome verifies a leading ~ is expanded to the user's
// home directory, and other paths are left untouched.
func TestFilesystemExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"~":          home,
		"~/models":   filepath.Join(home, "models"),
		"/abs/path":  "/abs/path",
		"relative/x": "relative/x",
	}
	for in, want := range cases {
		if got := expandHome(in); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestFilesystemScanErrors verifies the scanner reports a missing root, a root
// that is not a directory, and a missing configuration.
func TestFilesystemScanErrors(t *testing.T) {
	scanner := NewFilesystemScanner()
	if _, err := scanner.ScanForModels(); err == nil {
		t.Error("expected an error scanning with no directory configured")
	}

	scanner.Configure(t.TempDir()+"/missing", 0)
	if _, err := scanner.ScanForModels(); err == nil {
		t.Error("expected an error scanning a directory that does not exist")
	}

	file := filepath.Join(t.TempDir(), "file.gguf")
	if err := os.WriteFile(file, []byte("stub"), 0644); err != nil {
		t.Fatal(err)
	}
	scanner.Configure(file, 0)
	if _, err := scanner.ScanForModels(); err == nil {
		t.Error("expected an error scanning a file as a directory")
	}
}

// TestFilesystemScanEmpty verifies a directory holding no .gguf files yields
// no models.
func TestFilesystemScanEmpty(t *testing.T) {
	root := t.TempDir()
	scanner := NewFilesystemScanner()
	scanner.Configure(root, FilesystemDepthUnlimited)
	models, err := scanner.ScanForModels()
	if err != nil {
		t.Fatalf("ScanForModels: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("models = %d, want 0", len(models))
	}
}
