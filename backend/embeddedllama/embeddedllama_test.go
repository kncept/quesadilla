package embeddedllama

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

func TestBackendIdentity(t *testing.T) {
	b := EmbeddedLlamaBackend()
	if b.Id() != "embedded-llama" {
		t.Errorf("Id() = %q, want %q", b.Id(), "embedded-llama")
	}
	if b.Description() == "" {
		t.Error("Description() empty")
	}
	if got := b.ModelTypes(); len(got) != 1 || got[0] != "gguf" {
		t.Errorf("ModelTypes() = %v, want [gguf]", got)
	}
}

// TestBuiltInVersions verifies the runtime always reports its single built-in
// version, whether or not it was compiled into this build, and that it is
// never installable (there is nothing to download).
func TestBuiltInVersions(t *testing.T) {
	b := EmbeddedLlamaBackend()

	installed := b.InstalledVersions()
	if len(installed) != 1 || installed[0] != builtInVersion {
		t.Errorf("InstalledVersions() = %v, want [%s]", installed, builtInVersion)
	}
	if got := b.LatestVersion(); got != builtInVersion {
		t.Errorf("LatestVersion() = %q, want %q", got, builtInVersion)
	}

	if got := b.InstallableVersions(); len(got) != 0 {
		t.Errorf("InstallableVersions() = %v, want none (runtime is built in)", got)
	}
}

func TestInstallAndRemoveAreNoOps(t *testing.T) {
	b := EmbeddedLlamaBackend()
	if err := b.InstallVersion(builtInVersion); err == nil {
		t.Error("InstallVersion succeeded, want an error (nothing to install)")
	}
	if err := b.RemoveVersion(builtInVersion); err == nil {
		t.Error("RemoveVersion succeeded, want an error (nothing to remove)")
	}
	// the built-in version is unaffected by either
	if got := b.InstalledVersions(); len(got) > 0 && got[0] != builtInVersion {
		t.Errorf("InstalledVersions() = %v after install/remove attempts", got)
	}
}

// TestStartFailsForMissingModel verifies Start reports an error (and does not
// hang or panic) when the model file does not exist. In a stub build the
// runtime is not compiled in at all, so it fails before touching the file.
func TestStartFailsForMissingModel(t *testing.T) {
	b := EmbeddedLlamaBackend()
	m := &modelDefinitions.Model{
		ModelName: "no-such-model",
		ModelType: "gguf",
		ModelFile: "/nonexistent/no-such-model.gguf",
	}
	if _, err := b.Start(m); err == nil {
		t.Fatal("Start succeeded, want an error for a missing model file")
	}
}

// TestRequestLoggerCapturesLines verifies the per-request log line format.
func TestRequestLoggerCapturesLines(t *testing.T) {
	logs := runnerDefinitions.NewLogBuffer(runnerDefinitions.DefaultMaxLogLines)
	handler := requestLogger(log.New(logs.Writer(), providerId+": ", 0), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost/v1/chat/completions", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	lines := logs.Lines()
	if len(lines) != 1 {
		t.Fatalf("Logs() = %v, want one request line", lines)
	}
	line := lines[0]
	for _, want := range []string{"GET", "/v1/chat/completions", "404"} {
		if !strings.Contains(line, want) {
			t.Errorf("log line %q missing %q", line, want)
		}
	}
}

// TestRequestLoggerPreservesFlusher guards the streaming handlers: they assert
// the ResponseWriter is an http.Flusher, and the logging wrapper must not hide
// it. If it does, every streamed request fails with a 500 instead of streaming.
func TestRequestLoggerPreservesFlusher(t *testing.T) {
	var inner http.ResponseWriter
	handler := requestLogger(log.New(io.Discard, "", 0), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inner = w
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil))

	if inner == nil {
		t.Fatal("handler was not called")
	}
	if _, ok := inner.(http.Flusher); !ok {
		t.Errorf("wrapped writer %T does not implement http.Flusher: streaming handlers will 500", inner)
	}
	if _, ok := inner.(interface{ Unwrap() http.ResponseWriter }); !ok {
		t.Errorf("wrapped writer %T does not implement Unwrap", inner)
	}
}
