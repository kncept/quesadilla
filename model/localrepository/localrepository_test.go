package localrepository

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/kncept/quesadilla/model/definitions"
)

// withHome points HOME at a fresh temporary directory and returns its path,
// so the repository scans (and downloads into) a controlled location.
func withHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// servingModelServer starts a test server that serves the given bytes as the
// model file.
func servingModelServer(t *testing.T, contents string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(contents))
	}))
}

// TestDownloadModelFromURL verifies a downloaded model lands in its own
// directory under the models root, with the file the scanner resolved to,
// and shows up in the repository's model list under its scanned name.
func TestDownloadModelFromURL(t *testing.T) {
	home := withHome(t)
	server := servingModelServer(t, "gguf-bytes")
	defer server.Close()

	repo := NewLocalRepository()
	err := repo.DownloadModel(&definitions.RemoteModel{
		Model: definitions.Model{
			ModelName: "org/a",
			ModelType: "gguf",
			ModelFile: server.URL + "/a.gguf",
		},
		ScannerName: "ZTest",
	})
	if err != nil {
		t.Fatalf("DownloadModel: %v", err)
	}

	m := repo.GetModel("org/a")
	if m == nil {
		t.Fatal("expected the downloaded model to be listed")
	}
	wantFile := path.Join(home, ".quesadilla", "models", "gguf", "org", "a", "a.gguf")
	if m.ModelFile != wantFile {
		t.Errorf("ModelFile = %q, want %q", m.ModelFile, wantFile)
	}
	data, err := os.ReadFile(wantFile)
	if err != nil {
		t.Fatalf("reading the downloaded file: %v", err)
	}
	if string(data) != "gguf-bytes" {
		t.Errorf("file contents = %q, want gguf-bytes", data)
	}
}

// TestDownloadModelRedownload verifies downloading a model a second time
// replaces the file, leaving the model's directory with exactly one file and
// no stray temporary files in the models directory.
func TestDownloadModelRedownload(t *testing.T) {
	home := withHome(t)
	server := servingModelServer(t, "new-bytes")
	defer server.Close()

	repo := NewLocalRepository()
	remote := &definitions.RemoteModel{
		Model: definitions.Model{
			ModelName: "org/a",
			ModelType: "gguf",
			ModelFile: server.URL + "/a.gguf",
		},
		ScannerName: "ZTest",
	}
	if err := repo.DownloadModel(remote); err != nil {
		t.Fatalf("first download: %v", err)
	}
	if err := repo.DownloadModel(remote); err != nil {
		t.Fatalf("re-download: %v", err)
	}

	m := repo.GetModel("org/a")
	if m == nil {
		t.Fatal("expected the model to be listed after the re-download")
	}
	data, err := os.ReadFile(m.ModelFile)
	if err != nil {
		t.Fatalf("reading the re-downloaded file: %v", err)
	}
	if string(data) != "new-bytes" {
		t.Errorf("file contents = %q, want new-bytes", data)
	}

	entries, err := os.ReadDir(path.Dir(m.ModelFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("the model directory holds %d entries, want exactly 1", len(entries))
	}

	modelsDir := path.Join(home, ".quesadilla", "models", "gguf")
	entries, err = os.ReadDir(modelsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Errorf("stray temp file %q in the models directory", e.Name())
		}
	}
}

// TestDownloadModelFromLocalPath verifies a scanner that points at a local
// file (eg the LocalAI scanner) is installed by copying the file.
func TestDownloadModelFromLocalPath(t *testing.T) {
	home := withHome(t)
	srcDir := path.Join(home, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	srcFile := path.Join(srcDir, "local.gguf")
	if err := os.WriteFile(srcFile, []byte("local-bytes"), 0644); err != nil {
		t.Fatal(err)
	}

	repo := NewLocalRepository()
	err := repo.DownloadModel(&definitions.RemoteModel{
		Model: definitions.Model{
			ModelName: "local-model",
			ModelType: "gguf",
			ModelFile: srcFile,
		},
		ScannerName: "LocalAI",
	})
	if err != nil {
		t.Fatalf("DownloadModel: %v", err)
	}

	m := repo.GetModel("local-model")
	if m == nil {
		t.Fatal("expected the copied model to be listed")
	}
	data, err := os.ReadFile(m.ModelFile)
	if err != nil {
		t.Fatalf("reading the copied file: %v", err)
	}
	if string(data) != "local-bytes" {
		t.Errorf("file contents = %q, want local-bytes", data)
	}
	if _, err := os.Stat(srcFile); err != nil {
		t.Errorf("the source file should be untouched: %v", err)
	}
}

// TestDownloadModelUnknownType verifies only gguf models can be installed.
func TestDownloadModelUnknownType(t *testing.T) {
	withHome(t)
	repo := NewLocalRepository()
	err := repo.DownloadModel(&definitions.RemoteModel{
		Model: definitions.Model{
			ModelName: "org/b",
			ModelType: "hf",
			ModelFile: "https://x.org/org/b",
		},
		ScannerName: "ZTest",
	})
	if err == nil {
		t.Fatal("expected an error for an unsupported model type")
	}
	if repo.GetModel("org/b") != nil {
		t.Error("expected nothing to be installed")
	}
}

// TestDownloadModelFailureLeavesNoModel verifies a failed download installs
// nothing: the model is not listed and its directory holds no partial file.
func TestDownloadModelFailureLeavesNoModel(t *testing.T) {
	home := withHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer server.Close()

	repo := NewLocalRepository()
	err := repo.DownloadModel(&definitions.RemoteModel{
		Model: definitions.Model{
			ModelName: "org/a",
			ModelType: "gguf",
			ModelFile: server.URL + "/a.gguf",
		},
		ScannerName: "ZTest",
	})
	if err == nil {
		t.Fatal("expected an error from the failed download")
	}
	if repo.GetModel("org/a") != nil {
		t.Error("expected the failed model not to be listed")
	}
	modelDir := path.Join(home, ".quesadilla", "models", "gguf", "org", "a")
	entries, _ := os.ReadDir(modelDir)
	if len(entries) != 0 {
		t.Errorf("the model directory holds %d entries after a failed download, want 0", len(entries))
	}
}

// TestListModelsNestedName verifies models whose names contain a path
// separator (eg huggingface ids like "org/a") are listed under their full
// name, both for plain-file models and for linked models whose single file
// is their model.json metadata.
func TestListModelsNestedName(t *testing.T) {
	home := withHome(t)
	modelsDir := path.Join(home, ".quesadilla", "models", "gguf")

	// a plain-file model, stored nested
	if err := os.MkdirAll(path.Join(modelsDir, "org", "a"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(modelsDir, "org", "a", "a.gguf"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// a linked model, stored nested with its metadata file
	if err := os.MkdirAll(path.Join(modelsDir, "org", "b"), 0755); err != nil {
		t.Fatal(err)
	}
	metadata := `{"ModelName": "org/b", "ModelType": "gguf", "ModelFile": "https://x.org/org/b"}`
	if err := os.WriteFile(path.Join(modelsDir, "org", "b", "model.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}
	// a flat model, to make sure the nested handling does not change flat names
	if err := os.MkdirAll(path.Join(modelsDir, "flat"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(modelsDir, "flat", "flat.gguf"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	repo := NewLocalRepository()

	if m := repo.GetModel("org/a"); m == nil || m.ModelFile != path.Join(modelsDir, "org", "a", "a.gguf") {
		t.Errorf("org/a = %+v, want the nested plain-file model", m)
	}
	if m := repo.GetModel("org/b"); m == nil || m.ModelFile != "https://x.org/org/b" {
		t.Errorf("org/b = %+v, want the linked model from its metadata", m)
	}
	if m := repo.GetModel("flat"); m == nil {
		t.Error("the flat model is missing from the scan")
	}
	if got := len(repo.Models()); got != 3 {
		t.Errorf("scan found %d models, want 3", got)
	}
}
