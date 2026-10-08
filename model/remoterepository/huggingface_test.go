package remoterepository

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// hfTestServer serves canned HuggingFace API responses. The listing endpoint
// returns a different model set per filter/sort combination (so the scanner's
// filter options return overlapping, mergeable results), and the single-model
// and tree endpoints serve one GGUF model and one non-GGUF model.
func hfTestServer(t *testing.T, requests *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			*requests = append(*requests, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/models":
			filter := r.URL.Query().Get("filter")
			sort := r.URL.Query().Get("sort")
			switch {
			case filter == "text-generation":
				w.Write([]byte(`[
					{"id":"acme/Texty","downloads":5,"tags":["gguf","text-generation"],"pipeline_tag":"text-generation"},
					{"id":"acme/Plain","downloads":9,"tags":["transformers","text-generation"],"pipeline_tag":"text-generation"}
				]`))
			case filter == "gguf" && sort == "downloads":
				w.Write([]byte(`[
					{"id":"acme/Ggufy","downloads":7,"tags":["gguf"]},
					{"id":"acme/Texty","downloads":5,"tags":["gguf","text-generation"]}
				]`))
			case filter == "gguf" && sort == "likes":
				w.Write([]byte(`[
					{"id":"acme/Liked","likes":3,"tags":["gguf"]},
					{"id":"acme/Ggufy","likes":2,"tags":["gguf"]}
				]`))
			default:
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":"unexpected filter"}`))
			}
		case r.URL.Path == "/api/models/acme/Ggufy":
			w.Write([]byte(`{"id":"acme/Ggufy","downloads":7,"likes":2,"tags":["gguf"]}`))
		case r.URL.Path == "/api/models/acme/Ggufy/tree/main":
			w.Write([]byte(`[
				{"type":"file","path":"README.md"},
				{"type":"file","path":"Ggufy-Q4.gguf"},
				{"type":"file","path":"Ggufy-Q8.gguf"}
			]`))
		case r.URL.Path == "/api/models/acme/Plain":
			w.Write([]byte(`{"id":"acme/Plain","downloads":9,"tags":["transformers","text-generation"],"pipeline_tag":"text-generation"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"model not found"}`))
		}
	}))
}

// TestHuggingFaceScanForModels verifies the scanner runs every hard coded
// filter option, merges the results in option order, and drops models that
// more than one option returns.
func TestHuggingFaceScanForModels(t *testing.T) {
	server := hfTestServer(t, nil)
	defer server.Close()
	hfBaseURL = server.URL

	models, err := NewHuggingFaceScanner().ScanForModels()
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	types := make(map[string]string)
	for _, m := range models {
		names = append(names, m.ModelName)
		types[m.ModelName] = m.ModelType
	}
	want := []string{"acme/Texty", "acme/Plain", "acme/Ggufy", "acme/Liked"}
	if len(names) != len(want) {
		t.Fatalf("scanned %d models %v, want %d %v", len(names), names, len(want), want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("model %d = %q, want %q", i, names[i], want[i])
		}
	}

	// gguf-tagged models are runnable, the rest are not (yet)
	for name, wantType := range map[string]string{
		"acme/Texty": "gguf",
		"acme/Plain": "hf",
		"acme/Ggufy": "gguf",
		"acme/Liked": "gguf",
	} {
		if types[name] != wantType {
			t.Errorf("type of %s = %q, want %q", name, types[name], wantType)
		}
	}

	for _, m := range models {
		if m.ScannerName != "HuggingFace" {
			t.Errorf("scanner name = %q, want HuggingFace", m.ScannerName)
		}
		if m.ModelFile != server.URL+"/"+m.ModelName {
			t.Errorf("model file of %s = %q, want the model's hub page", m.ModelName, m.ModelFile)
		}
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

// TestHuggingFaceGetModel verifies GetModel verifies the model exists and,
// for GGUF releases, points the model file at the first GGUF file in the
// repository.
func TestHuggingFaceGetModel(t *testing.T) {
	server := hfTestServer(t, nil)
	defer server.Close()
	hfBaseURL = server.URL

	m := NewHuggingFaceScanner().GetModel("acme/Ggufy")
	if m == nil {
		t.Fatal("expected GetModel to find acme/Ggufy")
	}
	if m.ModelName != "acme/Ggufy" {
		t.Errorf("model name = %q, want acme/Ggufy", m.ModelName)
	}
	if m.ModelType != "gguf" {
		t.Errorf("model type = %q, want gguf", m.ModelType)
	}
	if m.ModelFile != server.URL+"/acme/Ggufy/resolve/main/Ggufy-Q4.gguf" {
		t.Errorf("model file = %q, want the first GGUF file's resolve URL", m.ModelFile)
	}
	if m.ScannerName != "HuggingFace" {
		t.Errorf("scanner name = %q, want HuggingFace", m.ScannerName)
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

// TestHuggingFaceGetModelNonGguf verifies a non-GGUF model is reported as
// such, with the model's hub page as its file, and that no tree lookup is
// attempted for it.
func TestHuggingFaceGetModelNonGguf(t *testing.T) {
	var requests []string
	server := hfTestServer(t, &requests)
	defer server.Close()
	hfBaseURL = server.URL

	m := NewHuggingFaceScanner().GetModel("acme/Plain")
	if m == nil {
		t.Fatal("expected GetModel to find acme/Plain")
	}
	if m.ModelType != "hf" {
		t.Errorf("model type = %q, want hf", m.ModelType)
	}
	if m.ModelFile != server.URL+"/acme/Plain" {
		t.Errorf("model file = %q, want the model's hub page", m.ModelFile)
	}
	for _, p := range requests {
		if p == "/api/models/acme/Plain/tree/main" {
			t.Error("did not expect a tree lookup for a non-GGUF model")
		}
	}
}
