package remoterepository

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// hfTestServer serves canned HuggingFace API responses. The listing endpoint
// returns a different model set per filter/sort combination (so the scanner's
// filter options return overlapping, mergeable results); the single-model
// endpoints serve one GGUF release per model, and the tree endpoints serve
// each model's files (one model holds no GGUF files at all).
func hfTestServer(t *testing.T, requests *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			*requests = append(*requests, r.URL.RequestURI())
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/models":
			filter := r.URL.Query().Get("filter")
			sort := r.URL.Query().Get("sort")
			search := r.URL.Query().Get("search")
			switch {
			case search != "":
				w.Write([]byte(`[
					{"id":"acme/Searched","downloads":1,"tags":["gguf"],"license":"mit"},
					{"id":"acme/Plain","downloads":9,"tags":["transformers"]}
				]`))
			case filter == "text-generation":
				w.Write([]byte(`[
					{"id":"acme/Texty","downloads":5,"tags":["gguf","text-generation"],"pipeline_tag":"text-generation","license":"mit"},
					{"id":"acme/Plain","downloads":9,"tags":["transformers","text-generation"],"pipeline_tag":"text-generation"}
				]`))
			case filter == "gguf" && sort == "downloads":
				w.Write([]byte(`[
					{"id":"acme/Ggufy","downloads":7,"tags":["gguf"],"license":"apache-2.0"},
					{"id":"acme/Texty","downloads":5,"tags":["gguf","text-generation"],"license":"mit"}
				]`))
			case filter == "gguf" && sort == "likes":
				w.Write([]byte(`[
					{"id":"acme/Liked","likes":3,"tags":["gguf"]},
					{"id":"acme/Ggufy","likes":2,"tags":["gguf"],"license":"apache-2.0"}
				]`))
			default:
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"unexpected filter"}`))
			}
		case r.URL.Path == "/api/models/acme/Texty":
			w.Write([]byte(`{"id":"acme/Texty","downloads":5,"tags":["gguf","text-generation"],"license":"mit"}`))
		case r.URL.Path == "/api/models/acme/Texty/tree/main":
			w.Write([]byte(`[
				{"type":"file","path":"README.md","size":10},
				{"type":"file","path":"LICENSE","size":20},
				{"type":"file","path":"Texty-Q4.gguf","size":100}
			]`))
		case r.URL.Path == "/api/models/acme/Plain":
			w.Write([]byte(`{"id":"acme/Plain","downloads":9,"tags":["transformers","text-generation"],"pipeline_tag":"text-generation"}`))
		case r.URL.Path == "/api/models/acme/Plain/tree/main":
			w.Write([]byte(`[
				{"type":"file","path":"model.safetensors","size":300}
			]`))
		case r.URL.Path == "/api/models/acme/Ggufy":
			w.Write([]byte(`{"id":"acme/Ggufy","downloads":7,"likes":2,"tags":["gguf"],"license":"apache-2.0"}`))
		case r.URL.Path == "/api/models/acme/Ggufy/tree/main":
			w.Write([]byte(`[
				{"type":"file","path":"README.md","size":10},
				{"type":"file","path":"LICENSE","size":20},
				{"type":"file","path":"Ggufy-Q4.gguf","size":100},
				{"type":"file","path":"Ggufy-Q8.gguf","size":200}
			]`))
		case r.URL.Path == "/api/models/acme/Searched":
			w.Write([]byte(`{"id":"acme/Searched","downloads":1,"tags":["gguf"],"license":"mit"}`))
		case r.URL.Path == "/api/models/acme/Searched/tree/main":
			w.Write([]byte(`[
				{"type":"file","path":"LICENSE","size":20},
				{"type":"file","path":"Searched-Q4.gguf","size":100}
			]`))
		case r.URL.Path == "/api/models/acme/Liked":
			w.Write([]byte(`{"id":"acme/Liked","likes":3,"tags":["gguf"]}`))
		case r.URL.Path == "/api/models/acme/Liked/tree/main":
			w.Write([]byte(`[
				{"type":"file","path":"Nested/Liked-Q4.gguf","size":50}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"model not found"}`))
		}
	}))
}

// TestHuggingFaceScanForModels verifies the scanner runs every hard coded
// filter option, merges the results in option order, drops models that more
// than one option returns, and trims models with no runnable (GGUF)
// variants.
func TestHuggingFaceScanForModels(t *testing.T) {
	server := hfTestServer(t, nil)
	defer server.Close()
	hfBaseURL = server.URL

	models, err := NewHuggingFaceScanner().ScanForModels()
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, m := range models {
		names = append(names, m.FullModelName())
	}
	// acme/Plain holds no GGUF files, so it is trimmed
	want := []string{"acme/Texty", "acme/Ggufy", "acme/Liked"}
	if len(names) != len(want) {
		t.Fatalf("scanned %d models %v, want %d %v", len(names), names, len(want), want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("model %d = %q, want %q", i, names[i], want[i])
		}
	}

	for _, m := range models {
		if m.ModelSource != "HuggingFace" {
			t.Errorf("scanner name = %q, want HuggingFace", m.ModelSource)
		}
		if m.ModelType != "gguf" {
			t.Errorf("type of %s = %q, want gguf", m.FullModelName(), m.ModelType)
		}
		if len(m.Variants) == 0 {
			t.Errorf("model %s has no variants", m.FullModelName())
		}
		if want := server.URL + "/" + m.FullModelName(); m.ModelURL != want {
			t.Errorf("model URL of %s = %q, want %q", m.FullModelName(), m.ModelURL, want)
		}
	}

	byName := make(map[string]struct {
		family   string
		name     string
		variants []string
		licence  string
	})
	for _, m := range models {
		vars := make([]string, 0, len(m.Variants))
		for _, v := range m.Variants {
			vars = append(vars, v.FileName)
		}
		byName[m.FullModelName()] = struct {
			family   string
			name     string
			variants []string
			licence  string
		}{m.ModelFamily, m.ModelName, vars, m.LicenceName}
	}
	if got := byName["acme/Ggufy"].variants; len(got) != 2 {
		t.Errorf("variants of acme/Ggufy = %v, want both GGUF files", got)
	}
	if got := byName["acme/Texty"].licence; got != "mit" {
		t.Errorf("licence of acme/Texty = %q, want mit", got)
	}
	if got := byName["acme/Ggufy"].licence; got != "apache-2.0" {
		t.Errorf("licence of acme/Ggufy = %q, want apache-2.0", got)
	}
}

// TestHuggingFaceScanForModelsError verifies a failed hub query is reported
// rather than silently dropped.
func TestHuggingFaceScanForModelsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	}))
	defer server.Close()
	hfBaseURL = server.URL

	if _, err := NewHuggingFaceScanner().ScanForModels(); err == nil {
		t.Error("expected an error when the hub query fails")
	}
}

// TestHuggingFaceScanForQuery verifies a query runs a single hub search
// (limited to GGUF files), lists what it finds, and trims models without a
// runnable GGUF file.
func TestHuggingFaceScanForQuery(t *testing.T) {
	requests := &[]string{}
	server := hfTestServer(t, requests)
	defer server.Close()
	hfBaseURL = server.URL

	models, err := NewHuggingFaceScanner().ScanForQuery("llama")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].FullModelName() != "acme/Searched" {
		t.Fatalf("models = %+v, want just acme/Searched", models)
	}
	if models[0].ModelSource != HuggingFaceScannerName {
		t.Errorf("source = %q, want %q", models[0].ModelSource, HuggingFaceScannerName)
	}

	var searched bool
	for _, request := range *requests {
		if strings.Contains(request, "search=llama") {
			searched = true
			if !strings.Contains(request, "filter=gguf") {
				t.Errorf("search request %q is not limited to gguf", request)
			}
		}
	}
	if !searched {
		t.Errorf("requests = %v, want a search for llama", *requests)
	}
}

// TestHuggingFaceScanForQueryEmpty verifies an empty (or blank) query falls
// back to the scanner's default listing.
func TestHuggingFaceScanForQueryEmpty(t *testing.T) {
	server := hfTestServer(t, nil)
	defer server.Close()
	hfBaseURL = server.URL

	models, err := NewHuggingFaceScanner().ScanForQuery("   ")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 {
		t.Errorf("scanned %d models, want the default listing's 3", len(models))
	}
}

// TestHuggingFaceGetModel verifies GetModel verifies the model exists, splits
// its id into family and name, and lists its variants (its GGUF files, with
// their sizes and download URLs) and its licence.
func TestHuggingFaceGetModel(t *testing.T) {
	server := hfTestServer(t, nil)
	defer server.Close()
	hfBaseURL = server.URL

	m := NewHuggingFaceScanner().GetModel("acme/Ggufy")
	if m == nil {
		t.Fatal("expected GetModel to find acme/Ggufy")
	}
	if m.ModelFamily != "acme" {
		t.Errorf("model family = %q, want acme", m.ModelFamily)
	}
	if m.ModelName != "Ggufy" {
		t.Errorf("model name = %q, want Ggufy", m.ModelName)
	}
	if m.ModelType != "gguf" {
		t.Errorf("model type = %q, want gguf", m.ModelType)
	}
	if m.ModelSource != "HuggingFace" {
		t.Errorf("scanner name = %q, want HuggingFace", m.ModelSource)
	}
	if m.LicenceName != "apache-2.0" {
		t.Errorf("licence name = %q, want apache-2.0", m.LicenceName)
	}
	if m.LicenceFile != server.URL+"/acme/Ggufy/resolve/main/LICENSE" {
		t.Errorf("licence file = %q, want the repository's licence", m.LicenceFile)
	}

	want := map[string]int64{
		"Ggufy-Q4.gguf": 100,
		"Ggufy-Q8.gguf": 200,
	}
	if len(m.Variants) != len(want) {
		t.Fatalf("variants = %d, want %d", len(m.Variants), len(want))
	}
	for _, v := range m.Variants {
		wantSize, ok := want[v.FileName]
		if !ok {
			t.Errorf("unexpected variant %q", v.FileName)
			continue
		}
		if v.FileSize != wantSize {
			t.Errorf("size of %s = %d, want %d", v.FileName, v.FileSize, wantSize)
		}
		wantURL := server.URL + "/acme/Ggufy/resolve/main/" + v.FileName
		if v.Location != wantURL {
			t.Errorf("location of %s = %q, want %q", v.FileName, v.Location, wantURL)
		}
	}
}

// TestHuggingFaceGetModelNestedVariants verifies GGUF files nested in
// subdirectories of the repository are listed as variants too (under their
// file name).
func TestHuggingFaceGetModelNestedVariants(t *testing.T) {
	server := hfTestServer(t, nil)
	defer server.Close()
	hfBaseURL = server.URL

	m := NewHuggingFaceScanner().GetModel("acme/Liked")
	if m == nil {
		t.Fatal("expected GetModel to find acme/Liked")
	}
	if len(m.Variants) != 1 {
		t.Fatalf("variants = %d, want 1", len(m.Variants))
	}
	if m.Variants[0].FileName != "Liked-Q4.gguf" {
		t.Errorf("variant file name = %q, want Liked-Q4.gguf", m.Variants[0].FileName)
	}
	if m.LicenceName != "" {
		t.Errorf("licence name = %q, want none", m.LicenceName)
	}
	if m.LicenceFile != "" {
		t.Errorf("licence file = %q, want none", m.LicenceFile)
	}
}

// TestHuggingFaceGetModelNoVariants verifies GetModel returns nil for a model
// that holds no runnable (GGUF) variants.
func TestHuggingFaceGetModelNoVariants(t *testing.T) {
	server := hfTestServer(t, nil)
	defer server.Close()
	hfBaseURL = server.URL

	if m := NewHuggingFaceScanner().GetModel("acme/Plain"); m != nil {
		t.Errorf("expected nil for a model with no runnable variants, got %+v", m)
	}
}

// TestHuggingFaceGetModelNotFound verifies GetModel returns nil when the hub
// does not know the model.
func TestHuggingFaceGetModelNotFound(t *testing.T) {
	server := hfTestServer(t, nil)
	defer server.Close()
	hfBaseURL = server.URL

	if m := NewHuggingFaceScanner().GetModel("acme/Missing"); m != nil {
		t.Errorf("expected nil for a missing model, got %+v", m)
	}
}
