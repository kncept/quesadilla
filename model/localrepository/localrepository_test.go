package localrepository

import (
	"encoding/json"
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

// readQModel reads and parses a model directory's qmodel.json.
func readQModel(t *testing.T, modelDir string) definitions.QModel {
	t.Helper()
	data, err := os.ReadFile(path.Join(modelDir, definitions.QModelFileName))
	if err != nil {
		t.Fatalf("reading qmodel.json in %s: %v", modelDir, err)
	}
	qm := definitions.QModel{}
	if err := json.Unmarshal(data, &qm); err != nil {
		t.Fatalf("parsing qmodel.json in %s: %v", modelDir, err)
	}
	return qm
}

// TestDownloadModelFromURL verifies a downloaded model lands in its
// family/name directory under the models root, with its qmodel.json listing
// the variant, and shows up in the repository's model list under its full
// name.
func TestDownloadModelFromURL(t *testing.T) {
	home := withHome(t)
	server := servingModelServer(t, "gguf-bytes")
	defer server.Close()

	repo := NewLocalRepository()
	err := repo.DownloadModel(&definitions.RemoteModel{
		ModelFamily: "org",
		ModelName:   "a",
		ModelSource: "ZTest",
		ModelType:   "gguf",
		Variants: []definitions.ModelVariant{
			{FileName: "a.gguf", Location: server.URL + "/a.gguf"},
		},
	})
	if err != nil {
		t.Fatalf("DownloadModel: %v", err)
	}

	m := repo.GetModel("org/a")
	if m == nil {
		t.Fatal("expected the downloaded model to be listed")
	}
	wantDir := path.Join(home, ".quesadilla", "models", "gguf", "org", "a")
	wantFile := path.Join(wantDir, "a.gguf")
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

	// the model's metadata records the family, name, source and variant
	qm := readQModel(t, wantDir)
	if qm.ModelFamily != "org" {
		t.Errorf("qmodel family = %q, want org", qm.ModelFamily)
	}
	if qm.ModelName != "a" {
		t.Errorf("qmodel name = %q, want a", qm.ModelName)
	}
	if qm.ModelSource != "ZTest" {
		t.Errorf("qmodel source = %q, want ZTest", qm.ModelSource)
	}
	if qm.ModelType != "gguf" {
		t.Errorf("qmodel type = %q, want gguf", qm.ModelType)
	}
	if len(qm.Variants) != 1 {
		t.Fatalf("qmodel variants = %d, want 1", len(qm.Variants))
	}
	v := qm.Variants[0]
	if v.FileName != "a.gguf" {
		t.Errorf("variant file name = %q, want a.gguf", v.FileName)
	}
	if v.FileSize != int64(len("gguf-bytes")) {
		t.Errorf("variant file size = %d, want %d", v.FileSize, len("gguf-bytes"))
	}
	if v.LinkedModel {
		t.Error("a downloaded variant must not be marked linked")
	}
	if v.Location != "" {
		t.Errorf("a downloaded variant must not carry a location: %q", v.Location)
	}
	if v.ImportedAt.IsZero() {
		t.Error("expected the variant's import time to be set")
	}
}

// TestDownloadModelMultipleVariants verifies a model with several variants
// has every variant downloaded into its directory, and its qmodel.json lists
// them all.
func TestDownloadModelMultipleVariants(t *testing.T) {
	home := withHome(t)
	server := servingModelServer(t, "gguf-bytes")
	defer server.Close()

	repo := NewLocalRepository()
	err := repo.DownloadModel(&definitions.RemoteModel{
		ModelFamily: "org",
		ModelName:   "a",
		ModelSource: "ZTest",
		ModelType:   "gguf",
		Variants: []definitions.ModelVariant{
			{FileName: "a-q4.gguf", Location: server.URL + "/q4"},
			{FileName: "a-q8.gguf", Location: server.URL + "/q8"},
		},
	})
	if err != nil {
		t.Fatalf("DownloadModel: %v", err)
	}

	wantDir := path.Join(home, ".quesadilla", "models", "gguf", "org", "a")
	for _, fileName := range []string{"a-q4.gguf", "a-q8.gguf"} {
		if _, err := os.Stat(path.Join(wantDir, fileName)); err != nil {
			t.Errorf("variant %s not installed: %v", fileName, err)
		}
	}

	qm := readQModel(t, wantDir)
	if len(qm.Variants) != 2 {
		t.Fatalf("qmodel variants = %d, want 2", len(qm.Variants))
	}
	names := make(map[string]bool)
	for _, v := range qm.Variants {
		names[v.FileName] = true
	}
	for _, fileName := range []string{"a-q4.gguf", "a-q8.gguf"} {
		if !names[fileName] {
			t.Errorf("qmodel variants missing %s (got %v)", fileName, names)
		}
	}
}

// TestDownloadModelRedownload verifies downloading a model a second time
// replaces the files, leaving the model's directory with exactly its
// variants and metadata, and no stray temporary files in the models
// directory.
func TestDownloadModelRedownload(t *testing.T) {
	home := withHome(t)
	server := servingModelServer(t, "new-bytes")
	defer server.Close()

	repo := NewLocalRepository()
	remote := &definitions.RemoteModel{
		ModelFamily: "org",
		ModelName:   "a",
		ModelSource: "ZTest",
		ModelType:   "gguf",
		Variants: []definitions.ModelVariant{
			{FileName: "a.gguf", Location: server.URL + "/a.gguf"},
		},
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
	if len(entries) != 2 { // the variant and the qmodel.json
		t.Errorf("the model directory holds %d entries, want 2", len(entries))
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

// TestDownloadModelFromLocalPath verifies a scanner that points at local
// files (eg the LocalAI scanner) is installed by copying the files, leaving
// the source files untouched.
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
		ModelName:   "local-model",
		ModelSource: "LocalAI",
		ModelType:   "gguf",
		Variants: []definitions.ModelVariant{
			{FileName: "local.gguf", Location: srcFile},
		},
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

// TestDownloadModelLicence verifies a scanned model's licence file is fetched
// into the model's directory (when the scanner knows of one) and recorded in
// the model's metadata.
func TestDownloadModelLicence(t *testing.T) {
	home := withHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.gguf":
			_, _ = w.Write([]byte("gguf-bytes"))
		case "/LICENSE":
			_, _ = w.Write([]byte("the licence"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	repo := NewLocalRepository()
	err := repo.DownloadModel(&definitions.RemoteModel{
		ModelFamily: "org",
		ModelName:   "a",
		ModelSource: "ZTest",
		ModelType:   "gguf",
		Variants:    []definitions.ModelVariant{{FileName: "a.gguf", Location: server.URL + "/a.gguf"}},
		LicenceName: "mit",
		LicenceFile: server.URL + "/LICENSE",
	})
	if err != nil {
		t.Fatalf("DownloadModel: %v", err)
	}

	wantDir := path.Join(home, ".quesadilla", "models", "gguf", "org", "a")
	data, err := os.ReadFile(path.Join(wantDir, "LICENSE"))
	if err != nil {
		t.Fatalf("reading the downloaded licence: %v", err)
	}
	if string(data) != "the licence" {
		t.Errorf("licence contents = %q, want the licence", data)
	}

	qm := readQModel(t, wantDir)
	if qm.LicenceName != "mit" {
		t.Errorf("qmodel licence name = %q, want mit", qm.LicenceName)
	}
	if qm.LicenceFile != "LICENSE" {
		t.Errorf("qmodel licence file = %q, want LICENSE", qm.LicenceFile)
	}
}

// TestDownloadModelModelURL verifies a scanned model's page at its source
// (ModelURL) is stored in the model's metadata, so the page can be opened
// from the installed model later.
func TestDownloadModelModelURL(t *testing.T) {
	home := withHome(t)
	server := servingModelServer(t, "gguf-bytes")
	defer server.Close()

	repo := NewLocalRepository()
	err := repo.DownloadModel(&definitions.RemoteModel{
		ModelFamily: "org",
		ModelName:   "a",
		ModelSource: "ZTest",
		ModelType:   "gguf",
		ModelURL:    "https://huggingface.co/org/a",
		Variants: []definitions.ModelVariant{
			{FileName: "a.gguf", Location: server.URL + "/a.gguf"},
		},
	})
	if err != nil {
		t.Fatalf("DownloadModel: %v", err)
	}

	m := repo.GetModel("org/a")
	if m == nil {
		t.Fatal("expected the downloaded model to be listed")
	}
	if m.ModelURL != "https://huggingface.co/org/a" {
		t.Errorf("model URL = %q, want https://huggingface.co/org/a", m.ModelURL)
	}

	wantDir := path.Join(home, ".quesadilla", "models", "gguf", "org", "a")
	qm := readQModel(t, wantDir)
	if qm.ModelURL != "https://huggingface.co/org/a" {
		t.Errorf("qmodel URL = %q, want https://huggingface.co/org/a", qm.ModelURL)
	}
}

// TestDownloadModelUnknownType verifies only gguf models can be installed.
func TestDownloadModelUnknownType(t *testing.T) {
	withHome(t)
	repo := NewLocalRepository()
	err := repo.DownloadModel(&definitions.RemoteModel{
		ModelFamily: "org",
		ModelName:   "b",
		ModelSource: "ZTest",
		ModelType:   "xxx",
		Variants:    []definitions.ModelVariant{{FileName: "b.gguf", Location: "https://x.org/b.gguf"}},
	})
	if err == nil {
		t.Fatal("expected an error for an unsupported model type")
	}
	if repo.GetModel("org/b") != nil {
		t.Error("expected nothing to be installed")
	}
}

// TestDownloadModelFailureLeavesNoModel verifies a failed download installs
// nothing: the model is not listed and its directory holds no partial file
// or metadata.
func TestDownloadModelFailureLeavesNoModel(t *testing.T) {
	home := withHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer server.Close()

	repo := NewLocalRepository()
	err := repo.DownloadModel(&definitions.RemoteModel{
		ModelFamily: "org",
		ModelName:   "a",
		ModelSource: "ZTest",
		ModelType:   "gguf",
		Variants:    []definitions.ModelVariant{{FileName: "a.gguf", Location: server.URL + "/a.gguf"}},
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

// TestDownloadVariantMerges verifies downloading a single variant of a model
// installs it, and downloading a second variant later lands in the same
// model directory, merged into the model's variant list.
func TestDownloadVariantMerges(t *testing.T) {
	home := withHome(t)
	server := servingModelServer(t, "gguf-bytes")
	defer server.Close()

	remote := &definitions.RemoteModel{
		ModelFamily: "org",
		ModelName:   "a",
		ModelSource: "ZTest",
		ModelType:   "gguf",
		Variants: []definitions.ModelVariant{
			{FileName: "a-q4.gguf", Location: server.URL + "/q4"},
			{FileName: "a-q8.gguf", Location: server.URL + "/q8"},
		},
	}

	repo := NewLocalRepository()
	if err := repo.DownloadVariant(remote, "a-q4.gguf"); err != nil {
		t.Fatalf("first variant: %v", err)
	}
	m := repo.GetModel("org/a")
	if m == nil {
		t.Fatal("expected the model to be listed after the first variant")
	}
	if got := len(m.Variants); got != 1 {
		t.Fatalf("variants after the first download = %d, want 1", got)
	}

	if err := repo.DownloadVariant(remote, "a-q8.gguf"); err != nil {
		t.Fatalf("second variant: %v", err)
	}
	m = repo.GetModel("org/a")
	if m == nil {
		t.Fatal("expected the model to still be listed after the second variant")
	}
	if got := len(m.Variants); got != 2 {
		t.Fatalf("variants after the second download = %d, want 2", got)
	}

	wantDir := path.Join(home, ".quesadilla", "models", "gguf", "org", "a")
	for _, fileName := range []string{"a-q4.gguf", "a-q8.gguf"} {
		if _, err := os.Stat(path.Join(wantDir, fileName)); err != nil {
			t.Errorf("variant %s not installed: %v", fileName, err)
		}
	}
	if err := repo.DownloadVariant(remote, "a-q16.gguf"); err == nil {
		t.Error("expected an error for a variant the model does not have")
	}
}

// TestLinkScannedModel verifies linking a scanned model stores only its
// qmodel.json (the files stay where they are), with the variants marked
// linked and pointing at the source files.
func TestLinkScannedModel(t *testing.T) {
	home := withHome(t)
	srcDir := path.Join(home, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	srcFile := path.Join(srcDir, "linked.gguf")
	if err := os.WriteFile(srcFile, []byte("linked-bytes"), 0644); err != nil {
		t.Fatal(err)
	}

	repo := NewLocalRepository()
	err := repo.LinkScannedModel(&definitions.RemoteModel{
		ModelName:   "linked-model",
		ModelSource: "LocalAI",
		ModelType:   "gguf",
		ModelURL:    "https://huggingface.co/localai/linked-model",
		Variants: []definitions.ModelVariant{
			{FileName: "linked.gguf", Location: srcFile},
		},
	})
	if err != nil {
		t.Fatalf("LinkScannedModel: %v", err)
	}

	m := repo.GetModel("linked-model")
	if m == nil {
		t.Fatal("expected the linked model to be listed")
	}
	if m.ModelFile != srcFile {
		t.Errorf("ModelFile = %q, want the source file %q", m.ModelFile, srcFile)
	}
	if len(m.Variants) != 1 || !m.Variants[0].LinkedModel {
		t.Errorf("expected a single linked variant, got %+v", m.Variants)
	}

	wantDir := path.Join(home, ".quesadilla", "models", "gguf", "none", "linked-model")
	entries, err := os.ReadDir(wantDir)
	if err != nil {
		t.Fatalf("the linked model's directory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("the linked model's directory holds %d entries, want only its qmodel.json", len(entries))
	}
	qm := readQModel(t, wantDir)
	if qm.ModelFamily != "none" {
		t.Errorf("qmodel family = %q, want none", qm.ModelFamily)
	}
	if qm.ModelSource != "LocalAI" {
		t.Errorf("qmodel source = %q, want LocalAI", qm.ModelSource)
	}
	if m.ModelURL != "https://huggingface.co/localai/linked-model" {
		t.Errorf("model URL = %q, want https://huggingface.co/localai/linked-model", m.ModelURL)
	}
	if qm.ModelURL != "https://huggingface.co/localai/linked-model" {
		t.Errorf("qmodel URL = %q, want https://huggingface.co/localai/linked-model", qm.ModelURL)
	}
	if len(qm.Variants) != 1 {
		t.Fatalf("qmodel variants = %d, want 1", len(qm.Variants))
	}
	if !qm.Variants[0].LinkedModel {
		t.Error("the linked model's variant must be marked linked")
	}
	if qm.Variants[0].Location != srcFile {
		t.Errorf("variant location = %q, want the source file %q", qm.Variants[0].Location, srcFile)
	}
	if qm.Variants[0].FileSize != int64(len("linked-bytes")) {
		t.Errorf("variant file size = %d, want %d", qm.Variants[0].FileSize, len("linked-bytes"))
	}
}

// TestRemoveModel verifies removing a model removes its directory (metadata
// and files) and rescans.
func TestRemoveModel(t *testing.T) {
	home := withHome(t)
	server := servingModelServer(t, "gguf-bytes")
	defer server.Close()

	repo := NewLocalRepository()
	if err := repo.DownloadModel(&definitions.RemoteModel{
		ModelFamily: "org",
		ModelName:   "a",
		ModelSource: "ZTest",
		ModelType:   "gguf",
		Variants:    []definitions.ModelVariant{{FileName: "a.gguf", Location: server.URL + "/a.gguf"}},
	}); err != nil {
		t.Fatalf("DownloadModel: %v", err)
	}

	if err := repo.RemoveModel("org/a"); err != nil {
		t.Fatalf("RemoveModel: %v", err)
	}
	if repo.GetModel("org/a") != nil {
		t.Error("expected the model to be gone after the remove")
	}
	if _, err := os.Stat(path.Join(home, ".quesadilla", "models", "gguf", "org", "a")); !os.IsNotExist(err) {
		t.Error("expected the model's directory to be removed")
	}
	if err := repo.RemoveModel("org/a"); err == nil {
		t.Error("expected an error removing a model that is not installed")
	}
}

// TestScanMigratesLegacyModelJson verifies a legacy linked model (a directory
// whose single file is its model.json) is migrated to the current layout: it
// moves under the "none" family, gains its qmodel.json, and the legacy
// metadata file is removed.
func TestScanMigratesLegacyModelJson(t *testing.T) {
	home := withHome(t)
	srcDir := path.Join(home, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	srcFile := path.Join(srcDir, "qwen.gguf")
	if err := os.WriteFile(srcFile, []byte("qwen-bytes"), 0644); err != nil {
		t.Fatal(err)
	}

	legacyDir := path.Join(home, ".quesadilla", "models", "gguf", "qwen")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := `{"ModelName": "qwen", "ModelType": "gguf", "ModelFile": "` + srcFile + `", "ScannerName": "LocalAI", "MetadataFile": "` + path.Join(legacyDir, "model.json") + `"}`
	if err := os.WriteFile(path.Join(legacyDir, "model.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	repo := NewLocalRepository()

	m := repo.GetModel("qwen")
	if m == nil {
		t.Fatal("expected the migrated model to be listed")
	}
	if m.ModelFile != srcFile {
		t.Errorf("ModelFile = %q, want the source file %q", m.ModelFile, srcFile)
	}
	if len(m.Variants) != 1 || !m.Variants[0].LinkedModel {
		t.Errorf("expected a single linked variant, got %+v", m.Variants)
	}

	wantDir := path.Join(home, ".quesadilla", "models", "gguf", "none", "qwen")
	if _, err := os.Stat(path.Join(wantDir, definitions.QModelFileName)); err != nil {
		t.Errorf("the migrated model's qmodel.json: %v", err)
	}
	if _, err := os.Stat(path.Join(wantDir, "model.json")); !os.IsNotExist(err) {
		t.Error("the legacy model.json should have been removed")
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Error("the legacy model directory should have been moved")
	}
}

// TestScanMigratesLegacyBareFile verifies a legacy downloaded model (a
// directory holding bare model files) is migrated to the current layout: it
// moves under the "none" family and gains a qmodel.json listing its files.
func TestScanMigratesLegacyBareFile(t *testing.T) {
	home := withHome(t)
	legacyDir := path.Join(home, ".quesadilla", "models", "gguf", "flat")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(legacyDir, "flat.gguf"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	repo := NewLocalRepository()

	m := repo.GetModel("flat")
	if m == nil {
		t.Fatal("expected the migrated model to be listed")
	}
	wantDir := path.Join(home, ".quesadilla", "models", "gguf", "none", "flat")
	if m.ModelDirectory != wantDir {
		t.Errorf("model directory = %q, want %q", m.ModelDirectory, wantDir)
	}
	qm := readQModel(t, wantDir)
	if qm.ModelFamily != "none" {
		t.Errorf("qmodel family = %q, want none", qm.ModelFamily)
	}
	if len(qm.Variants) != 1 || qm.Variants[0].FileName != "flat.gguf" {
		t.Errorf("qmodel variants = %+v, want the single flat.gguf variant", qm.Variants)
	}
}

// TestScanMigratesLegacyNestedName verifies a legacy nested model (a name
// with a path separator, stored one directory per component) keeps its full
// name after migration, with its family as the first component.
func TestScanMigratesLegacyNestedName(t *testing.T) {
	home := withHome(t)
	modelsDir := path.Join(home, ".quesadilla", "models", "gguf")

	// a legacy nested plain-file model
	if err := os.MkdirAll(path.Join(modelsDir, "org", "a"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(modelsDir, "org", "a", "a.gguf"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	repo := NewLocalRepository()

	m := repo.GetModel("org/a")
	if m == nil {
		t.Fatal("expected the nested model to be listed under its full name")
	}
	if m.ModelFamily != "org" || m.ModelName != "a" {
		t.Errorf("model family/name = %q/%q, want org/a", m.ModelFamily, m.ModelName)
	}
	qm := readQModel(t, path.Join(modelsDir, "org", "a"))
	if qm.ModelFamily != "org" || qm.ModelName != "a" {
		t.Errorf("qmodel family/name = %q/%q, want org/a", qm.ModelFamily, qm.ModelName)
	}
	if got := len(repo.Models()); got != 1 {
		t.Errorf("scan found %d models, want 1", got)
	}
}

// TestScanListsVariantsOnDisk verifies a model directory holding several
// model files lists every file as a variant, whether or not the metadata
// already knew of them.
func TestScanListsVariantsOnDisk(t *testing.T) {
	home := withHome(t)
	modelDir := path.Join(home, ".quesadilla", "models", "gguf", "org", "a")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(modelDir, "a-q4.gguf"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(modelDir, "a-q8.gguf"), []byte("xy"), 0644); err != nil {
		t.Fatal(err)
	}

	repo := NewLocalRepository()

	m := repo.GetModel("org/a")
	if m == nil {
		t.Fatal("expected the model to be listed")
	}
	if len(m.Variants) != 2 {
		t.Fatalf("variants = %d, want 2", len(m.Variants))
	}
	sizes := make(map[string]int64)
	for _, v := range m.Variants {
		sizes[v.FileName] = v.FileSize
	}
	if sizes["a-q4.gguf"] != 1 || sizes["a-q8.gguf"] != 2 {
		t.Errorf("variant sizes = %v, want a-q4.gguf=1 a-q8.gguf=2", sizes)
	}
}

// TestScanIgnoresModelsWithoutVariants verifies directories with no runnable
// variants are not listed: an empty directory, and a directory holding only
// a licence file.
func TestScanIgnoresModelsWithoutVariants(t *testing.T) {
	home := withHome(t)
	modelsDir := path.Join(home, ".quesadilla", "models", "gguf")

	if err := os.MkdirAll(path.Join(modelsDir, "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(path.Join(modelsDir, "licence-only"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(modelsDir, "licence-only", "LICENSE"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	repo := NewLocalRepository()
	if got := len(repo.Models()); got != 0 {
		t.Errorf("scan found %d models, want 0", got)
	}
}

// TestGetModelByNameAndFullName verifies a model is found both by its full
// name ("family/name") and, for a model with no known family, by its plain
// name.
func TestGetModelByNameAndFullName(t *testing.T) {
	home := withHome(t)
	modelsDir := path.Join(home, ".quesadilla", "models", "gguf")

	if err := os.MkdirAll(path.Join(modelsDir, "org", "a"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(modelsDir, "org", "a", "a.gguf"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(path.Join(modelsDir, "none", "b"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(modelsDir, "none", "b", "b.gguf"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	repo := NewLocalRepository()

	if m := repo.GetModel("org/a"); m == nil {
		t.Error("expected the model to be found by its full name")
	}
	if m := repo.GetModel("a"); m == nil {
		t.Error("expected the model to be found by its name")
	}
	if m := repo.GetModel("b"); m == nil {
		t.Error("expected the family-less model to be found by its name")
	}
	if m := repo.GetModel("none/b"); m == nil {
		t.Error("expected the family-less model to be found by its full name")
	}
}
