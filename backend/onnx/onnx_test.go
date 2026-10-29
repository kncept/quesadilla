package onnx

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

func TestBackendIdentity(t *testing.T) {
	b := OnnxBackend()
	if b.Id() != "onnx" {
		t.Errorf("Id() = %q, want %q", b.Id(), "onnx")
	}
	if b.Description() == "" {
		t.Error("Description() empty")
	}
	if got := b.ModelTypes(); len(got) != 1 || got[0] != "onnx" {
		t.Errorf("ModelTypes() = %v, want [onnx]", got)
	}
}

func TestBuiltInVersions(t *testing.T) {
	b := OnnxBackend()

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
	b := OnnxBackend()
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
	b := OnnxBackend()
	m := &modelDefinitions.Model{
		ModelName: "no-such-model",
		ModelType: string(modelDefinitions.ModelTypeONNX),
		Variants: []modelDefinitions.ModelVariant{
			{FileName: "no-such-model.onnx", Location: "/nonexistent/no-such-model.onnx", LinkedModel: true},
		},
	}
	_, err := b.Start(m)
	if err == nil {
		t.Fatal("Start succeeded, want an error for a missing model file")
	}
	if !strings.Contains(err.Error(), "/nonexistent/no-such-model.onnx") {
		t.Errorf("Start error %q does not mention the model file", err)
	}
}

// fixtureModel returns the test model (a two-input Add, inputs x and y of
// shape [3,4,5], one output "sum").
func fixtureModel() *modelDefinitions.Model {
	// Get the absolute path to the testdata directory
	testDataDir, _ := filepath.Abs("testdata")
	return &modelDefinitions.Model{
		ModelName:     "add-model",
		ModelType:     string(modelDefinitions.ModelTypeONNX),
		ModelDirectory: testDataDir,
		Variants: []modelDefinitions.ModelVariant{
			{FileName: "add_model.onnx", Location: filepath.Join(testDataDir, "add_model.onnx")},
		},
	}
}

// TestLoadAndRunFixture verifies the onnx-go pipeline end to end: the model
// decodes, the inputs can be set, and a run produces the summed output.
func TestLoadAndRunFixture(t *testing.T) {
	fm := fixtureModel()
	model, graph, err := loadOnnxModel(fm.GetDefaultVariantPath())
	if err != nil {
		t.Fatalf("loadOnnxModel: %v", err)
	}

	inputs := model.GetInputTensors()
	if len(inputs) != 2 {
		t.Fatalf("the fixture declares %d inputs, want 2", len(inputs))
	}

	ones := make([]float64, 60)
	twos := make([]float64, 60)
	for i := range ones {
		ones[i] = 1
		twos[i] = 2
	}
	t1, err := newTensor([]int{3, 4, 5}, ones)
	if err != nil {
		t.Fatalf("newTensor: %v", err)
	}
	t2, err := newTensor([]int{3, 4, 5}, twos)
	if err != nil {
		t.Fatalf("newTensor: %v", err)
	}
	if err := model.SetInput(0, t1); err != nil {
		t.Fatalf("SetInput(0): %v", err)
	}
	if err := model.SetInput(1, t2); err != nil {
		t.Fatalf("SetInput(1): %v", err)
	}

	if err := graph.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	outputs, err := model.GetOutputTensors()
	if err != nil {
		t.Fatalf("GetOutputTensors: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(outputs))
	}
	flat := flattenTensor(outputs[0])
	if len(flat) != 60 {
		t.Fatalf("output has %d values, want 60", len(flat))
	}
	for i, v := range flat {
		if v != 3 {
			t.Fatalf("output[%d] = %v, want 3 (1+2)", i, v)
		}
	}
}

// newTestRunningModel builds a running model over the loaded fixture, served
// on the given listener (so tests can use an ephemeral port).
func newTestRunningModel(t *testing.T, listener net.Listener) *onnxRunningModel {
	t.Helper()
	fm := fixtureModel()
	model, graph, err := loadOnnxModel(fm.GetDefaultVariantPath())
	if err != nil {
		t.Fatalf("loadOnnxModel: %v", err)
	}
	logs := runnerDefinitions.NewLogBuffer(runnerDefinitions.DefaultMaxLogLines)
	return &onnxRunningModel{
		modelName:      fm.FullModelName(),
		runtimeVersion: builtInVersion,
		startedAt:      time.Now(),
		logs:           logs,
		logger:         log.New(logs.Writer(), providerId+": ", 0),
		model:          model,
		graph:          graph,
		server: &http.Server{
			Addr:    listener.Addr().String(),
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		},
		done: make(chan struct{}),
	}
}

// postInference runs one POST /inference through the running model's handler
// and decodes the JSON response.
func postInference(t *testing.T, g *onnxRunningModel, body string) (int, inferenceResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/inference", strings.NewReader(body))
	rec := httptest.NewRecorder()
	g.handleInference(rec, req)
	var resp inferenceResponse
	if rec.Code == http.StatusOK {
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decoding the inference response: %v", err)
		}
	}
	return rec.Code, resp
}

// TestInferenceHandlerServesResults verifies the inference endpoint end to
// end: with the shapes omitted it uses the model's declared input shapes,
// and the reply carries the flattened output.
func TestInferenceHandlerServesResults(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	g := newTestRunningModel(t, listener)

	ones := make([]float64, 60)
	twos := make([]float64, 60)
	for i := range ones {
		ones[i] = 1
		twos[i] = 2
	}
	body, err := json.Marshal(inferenceRequest{Inputs: []inferenceInput{
		{Data: ones},
		{Data: twos},
	}})
	if err != nil {
		t.Fatal(err)
	}

	code, resp := postInference(t, g, string(body))
	if code != http.StatusOK {
		t.Fatalf("POST /inference -> %d, want 200", code)
	}
	if len(resp.Outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(resp.Outputs))
	}
	if len(resp.Outputs[0]) != 60 {
		t.Fatalf("output has %d values, want 60", len(resp.Outputs[0]))
	}
	for i, v := range resp.Outputs[0] {
		if v != 3 {
			t.Fatalf("output[%d] = %v, want 3 (1+2)", i, v)
		}
	}
}

// TestInferenceHandlerRejectsBadRequests verifies the endpoint rejects a
// request with no inputs and one whose data does not fit the shape.
func TestInferenceHandlerRejectsBadRequests(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	g := newTestRunningModel(t, listener)

	if code, _ := postInference(t, g, `{"inputs": []}`); code != http.StatusBadRequest {
		t.Errorf("no inputs -> %d, want 400", code)
	}
	// 60 values for [3,4,5] is right; 61 is not
	ones := make([]float64, 61)
	for i := range ones {
		ones[i] = 1
	}
	body, _ := json.Marshal(inferenceRequest{Inputs: []inferenceInput{{Data: ones}}})
	if code, _ := postInference(t, g, string(body)); code != http.StatusBadRequest {
		t.Errorf("wrong data size -> %d, want 400", code)
	}
}

// TestRunningModelQuitAndWait verifies the in-process running model's
// lifecycle: it serves until SendSigQuit is called, then Wait returns.
func TestRunningModelQuitAndWait(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	g := newTestRunningModel(t, listener)
	go g.serve(listener)

	if got := g.ModelName(); got != "add-model" {
		t.Errorf("ModelName() = %q", got)
	}
	if got := g.ProviderName(); got != providerId {
		t.Errorf("ProviderName() = %q, want %q", got, providerId)
	}
	if got := g.RuntimeVersion(); got != builtInVersion {
		t.Errorf("RuntimeVersion() = %q", got)
	}
	if uptime := g.Uptime(); uptime < 0 {
		t.Errorf("Uptime() = %v, want >= 0", uptime)
	}

	logs := g.logs
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
	g := newTestRunningModel(t, listener)
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
	handler := requestLogger(log.New(logs.Writer(), providerId+": ", 0), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost/inference", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	lines := logs.Lines()
	if len(lines) != 1 {
		t.Fatalf("Logs() = %v, want one request line", lines)
	}
	line := lines[0]
	for _, want := range []string{"GET", "/inference", "404"} {
		if !strings.Contains(line, want) {
			t.Errorf("log line %q missing %q", line, want)
		}
	}
}

// TestNewTensorRejectsBadSizes verifies newTensor refuses shapes that do not
// hold exactly the given number of values.
func TestNewTensorRejectsBadSizes(t *testing.T) {
	if _, err := newTensor([]int{2, 2}, []float64{1, 2, 3}); err == nil {
		t.Error("expected an error for too few values")
	}
	if _, err := newTensor([]int{2}, []float64{1, 2, 3}); err == nil {
		t.Error("expected an error for too many values")
	}
	if _, err := newTensor([]int{0, 2}, []float64{1, 2}); err == nil {
		t.Error("expected an error for a zero dimension")
	}
	// a good one builds
	tensor, err := newTensor([]int{2, 2}, []float64{1, 2, 3, 4})
	if err != nil {
		t.Fatalf("newTensor: %v", err)
	}
	if got := tensor.Shape(); len(got) != 2 || got[0] != 2 || got[1] != 2 {
		t.Errorf("tensor shape = %v, want [2 2]", got)
	}
}

// TestFlattenTensor verifies the row-major flattening of tensor values.
func TestFlattenTensor(t *testing.T) {
	tensor, err := newTensor([]int{2, 2}, []float64{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	if got := flattenTensor(tensor); len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Errorf("flattenTensor = %v, want [1 2 3 4]", got)
	}
	if got := flattenTensor(nil); got != nil {
		t.Errorf("flattenTensor(nil) = %v, want nil", got)
	}
}
