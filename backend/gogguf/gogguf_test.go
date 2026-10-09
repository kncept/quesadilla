package gogguf

import (
	"log"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

func TestBackendIdentity(t *testing.T) {
	b := GoggufBackend()
	if b.Id() != "gogguf" {
		t.Errorf("Id() = %q, want %q", b.Id(), "gogguf")
	}
	if b.Description() == "" {
		t.Error("Description() empty")
	}
	if got := b.ModelTypes(); len(got) != 1 || got[0] != "gguf" {
		t.Errorf("ModelTypes() = %v, want [gguf]", got)
	}
}

func TestBuiltInVersions(t *testing.T) {
	b := GoggufBackend()

	if got := b.InstalledVersions(); len(got) != 1 || got[0] != builtInVersion {
		t.Errorf("InstalledVersions() = %v, want [%s]", got, builtInVersion)
	}
	if got := b.InstallableVersions(); len(got) != 0 {
		t.Errorf("InstallableVersions() = %v, want none (runtime is built in)", got)
	}
	if got := b.LatestVersion(); got != builtInVersion {
		t.Errorf("LatestVersion() = %q, want %q", got, builtInVersion)
	}
}

func TestInstallAndRemoveAreNoOps(t *testing.T) {
	b := GoggufBackend()
	if err := b.InstallVersion(builtInVersion); err == nil {
		t.Error("InstallVersion succeeded, want an error (nothing to install)")
	}
	if err := b.RemoveVersion(builtInVersion); err == nil {
		t.Error("RemoveVersion succeeded, want an error (nothing to remove)")
	}
	// the built-in version is unaffected by either
	if got := b.InstalledVersions(); len(got) != 1 || got[0] != builtInVersion {
		t.Errorf("InstalledVersions() = %v after install/remove attempts", got)
	}
}

// TestStartFailsForMissingModel verifies Start reports a load error (and does
// not hang or panic) when the model file does not exist.
func TestStartFailsForMissingModel(t *testing.T) {
	b := GoggufBackend()
	m := &modelDefinitions.Model{
		ModelName: "no-such-model",
		ModelType: "gguf",
		ModelFile: "/nonexistent/no-such-model.gguf",
	}
	_, err := b.Start(m)
	if err == nil {
		t.Fatal("Start succeeded, want an error for a missing model file")
	}
	if !strings.Contains(err.Error(), m.ModelFile) {
		t.Errorf("Start error %q does not mention the model file", err)
	}
}

// TestRunningModelQuitAndWait verifies the in-process running model's
// lifecycle: it serves until SendSigQuit is called, then Wait returns. The
// engine is nil (its Close is nil-safe), so no model file is needed.
func TestRunningModelQuitAndWait(t *testing.T) {
	logs := runnerDefinitions.NewLogBuffer(runnerDefinitions.DefaultMaxLogLines)

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()

	g := &goggufRunningModel{
		modelName:      "fake-model",
		runtimeVersion: builtInVersion,
		startedAt:      time.Now(),
		logs:           logs,
		logger:         log.New(logs.Writer(), providerId+": ", 0),
		engine:         nil,
		server: &http.Server{
			Addr:    addr,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		},
		done: make(chan struct{}),
	}
	go g.serve(listener)

	if got := g.ModelName(); got != "fake-model" {
		t.Errorf("ModelName() = %q", got)
	}
	if got := g.ProviderName(); got != providerId {
		t.Errorf("ProviderName() = %q, want %q", got, providerId)
	}
	if got := g.RuntimeVersion(); got != builtInVersion {
		t.Errorf("RuntimeVersion() = %q", got)
	}
	if g.Uptime() < 0 {
		t.Errorf("Uptime() = %v, want >= 0", g.Uptime())
	}

	logs.AddLine("marker-before-quit")
	g.SendSigQuit()
	g.SendSigQuit() // idempotent

	waitDone := make(chan struct{})
	go func() {
		g.Wait()
		g.Wait() // idempotent
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Wait did not return after SendSigQuit")
	}

	// Logs must still serve the lines captured before the server stopped
	// (like the llama backend does after its process exits).
	found := false
	for _, line := range g.Logs() {
		if line == "marker-before-quit" {
			found = true
		}
	}
	if !found {
		t.Errorf("Logs() = %v, want the pre-quit marker line", g.Logs())
	}
}

// TestRunningModelKill verifies SendSigKill stops the server too.
func TestRunningModelKill(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	logs := runnerDefinitions.NewLogBuffer(runnerDefinitions.DefaultMaxLogLines)
	g := &goggufRunningModel{
		modelName: "fake-model",
		startedAt: time.Now(),
		logs:      logs,
		logger:    log.New(logs.Writer(), providerId+": ", 0),
		engine:    nil,
		server:    &http.Server{Addr: listener.Addr().String(), Handler: http.NewServeMux()},
		done:      make(chan struct{}),
	}
	go g.serve(listener)

	g.SendSigKill()

	select {
	case <-g.done:
	case <-time.After(10 * time.Second):
		t.Fatal("server did not stop after SendSigKill")
	}
}

// TestRequestLoggerCapturesLines verifies the per-request log line format.
func TestRequestLoggerCapturesLines(t *testing.T) {
	logs := runnerDefinitions.NewLogBuffer(runnerDefinitions.DefaultMaxLogLines)
	handler := requestLogger(logs, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req, err := http.NewRequest(http.MethodGet, "http://localhost/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptestNewRecorder()
	handler.ServeHTTP(rec, req)

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

// httptestNewRecorder is a minimal response recorder (avoiding an extra
// import for a test-only type).
type testResponseRecorder struct {
	header http.Header
	body   strings.Builder
	code   int
}

func httptestNewRecorder() *testResponseRecorder {
	return &testResponseRecorder{header: http.Header{}, code: http.StatusOK}
}

func (r *testResponseRecorder) Header() http.Header { return r.header }

func (r *testResponseRecorder) Write(p []byte) (int, error) {
	return r.body.Write(p)
}

func (r *testResponseRecorder) WriteHeader(code int) {
	r.code = code
}
