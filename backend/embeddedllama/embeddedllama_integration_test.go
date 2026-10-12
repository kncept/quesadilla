//go:build cgo

package embeddedllama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
)

// TestEmbeddedLlamaIntegration loads a real GGUF model through llama.cpp and
// serves one request from the in-process OpenAI-compatible API. It is skipped
// unless QUESADILLA_TEST_MODEL points at a GGUF file, so the normal test run
// stays hermetic and a small model can be dropped in to exercise the native
// runtime wherever the cgo build is available (including, later, other
// platforms in CI).
func TestEmbeddedLlamaIntegration(t *testing.T) {
	modelFile := os.Getenv("QUESADILLA_TEST_MODEL")
	if modelFile == "" {
		t.Skip("set QUESADILLA_TEST_MODEL to a .gguf file to exercise the native runtime")
	}

	m := &modelDefinitions.Model{
		ModelName: "integration",
		ModelType: "gguf",
		ModelFile: modelFile,
	}
	running, err := startEmbeddedLlamaServer(m)
	if err != nil {
		t.Fatalf("startEmbeddedLlamaServer: %v", err)
	}
	t.Cleanup(func() {
		running.SendSigQuit()
		running.Wait()
	})

	if got := running.RuntimeVersion(); got != builtInVersion {
		t.Errorf("RuntimeVersion() = %q, want %q", got, builtInVersion)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://localhost:%d/", servePort), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	var info map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("decoding GET /: %v", err)
	}
	if info["provider"] != providerId {
		t.Errorf("provider = %v, want %q", info["provider"], providerId)
	}
	if info["version"] != builtInVersion {
		t.Errorf("version = %v, want %q", info["version"], builtInVersion)
	}
}
