//go:build cgo

package embeddedllama

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	llama "github.com/tcpipuk/llama-go"

	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// newTestRunningModel builds just enough of an embeddedRunningModel to exercise
// the error-logging paths, which need no loaded model.
func newTestRunningModel() (*embeddedRunningModel, *runnerDefinitions.LogBuffer) {
	logs := runnerDefinitions.NewLogBuffer(runnerDefinitions.DefaultMaxLogLines)
	return &embeddedRunningModel{logger: log.New(logs.Writer(), providerId+": ", 0)}, logs
}

// TestHTTPErrorLogsReason verifies a server-side failure is written to the log
// (the console and the GUI log viewer) and not only to the response body.
func TestHTTPErrorLogsReason(t *testing.T) {
	g, logs := newTestRunningModel()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	g.httpError(recorder, req, http.StatusInternalServerError, "responses failed: %v", "boom")

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "boom") {
		t.Errorf("body = %q, want the reason", recorder.Body.String())
	}

	lines := logs.Lines()
	if len(lines) != 1 {
		t.Fatalf("logs = %v, want one line", lines)
	}
	for _, want := range []string{"POST", "/v1/responses", "500", "boom"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("log line %q missing %q", lines[0], want)
		}
	}
}

// plainWriter is an http.ResponseWriter that is deliberately not an
// http.Flusher, standing in for a connection that cannot stream.
type plainWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *plainWriter) Header() http.Header         { return w.header }
func (w *plainWriter) Write(b []byte) (int, error) { return w.body.Write(b) }
func (w *plainWriter) WriteHeader(code int)        { w.status = code }

// TestStreamResponsesLogsUnsupported verifies that when streaming cannot be
// done, the failure is logged with its reason (rather than silently returning a
// bare 500, which is what made this hard to diagnose).
func TestStreamResponsesLogsUnsupported(t *testing.T) {
	g, logs := newTestRunningModel()

	w := &plainWriter{header: http.Header{}}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	g.streamResponses(w, req, responsesRequest{}, nil, llama.ChatOptions{})

	if w.status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.status)
	}
	lines := logs.Lines()
	if len(lines) == 0 || !strings.Contains(lines[len(lines)-1], "streaming is not supported") {
		t.Errorf("logs = %v, want the streaming failure reason", lines)
	}
}
