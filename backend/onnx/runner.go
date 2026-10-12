package onnx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"sync"
	"time"

	onnxEngine "github.com/owulveryck/onnx-go"
	onnxGraph "github.com/owulveryck/onnx-go/backend/x/gorgonnx"
	"gorgonia.org/tensor"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// servePort is the port the in-process onnx server listens on. The llama
// backend uses 9931 and gogguf 9932, so onnx takes the next one, letting all
// three backends serve at the same time. (As with the others, only one model
// per backend can be served at once: a second start fails with "address
// already in use".)
const servePort = 9933

// loadOnnxModel reads the ONNX file at path and decodes it into an in-memory
// model over a Gorgonia execution graph, ready to run.
func loadOnnxModel(path string) (*onnxEngine.Model, *onnxGraph.Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: reading model %s: %w", providerId, path, err)
	}
	graph := onnxGraph.NewGraph()
	model := onnxEngine.NewModel(graph)
	if err := model.UnmarshalBinary(data); err != nil {
		return nil, nil, fmt.Errorf("%s: decoding model %s: %w", providerId, path, err)
	}
	return model, graph, nil
}

// startOnnxServer loads the model with onnx-go and serves it in-process over
// a small HTTP inference API. It returns once the server is listening, so a
// port conflict or a failed model load surfaces as an error here rather than
// later.
func startOnnxServer(m *modelDefinitions.Model) (runnerDefinitions.RunningModel, error) {
	// Keep the last N lines in memory for the log viewer, like the other
	// backends do. A log.Logger over the buffer's writer feeds it complete
	// lines.
	logs := runnerDefinitions.NewLogBuffer(runnerDefinitions.DefaultMaxLogLines)
	// Every line is also printed to the console (os.Stdout), so the start,
	// stop and error events are visible even when no GUI is watching the
	// log viewer.
	logger := log.New(io.MultiWriter(logs.Writer(), os.Stdout), providerId+": ", 0)
	logger.Printf("starting %s (runtime %s)", m.FullModelName(), builtInVersion)
	logger.Printf("loading model %s", m.ModelFile)

	model, graph, err := loadOnnxModel(m.ModelFile)
	if err != nil {
		logger.Printf("error: %v", err)
		return nil, err
	}
	logger.Printf("model loaded")

	// Bind the listener before returning, so Start only succeeds when the
	// server is really up.
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", servePort))
	if err != nil {
		logger.Printf("error: listening on localhost:%d: %v", servePort, err)
		return nil, fmt.Errorf("%s: listening on localhost:%d: %w", providerId, servePort, err)
	}

	g := &onnxRunningModel{
		modelName:      m.FullModelName(),
		runtimeVersion: builtInVersion,
		startedAt:      time.Now(),
		logs:           logs,
		logger:         logger,
		model:          model,
		graph:          graph,
		done:           make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", g.handleInfo)
	mux.HandleFunc("/inference", g.handleInference)
	g.server = &http.Server{
		Addr:              fmt.Sprintf("localhost:%d", servePort),
		Handler:           requestLogger(logger, mux),
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          logger,
	}

	go g.serve(listener)
	return g, nil
}

// serve runs the HTTP server until it is shut down (via SendSigQuit or
// SendSigKill), then signals Wait.
func (g *onnxRunningModel) serve(listener net.Listener) {
	defer close(g.done)
	g.logger.Printf("serving %s on http://%s", g.modelName, g.server.Addr)
	if serveErr := g.server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
		g.logger.Printf("error: server: %v", serveErr)
	}
	g.logger.Printf("stopped %s", g.modelName)
}

// onnxRunningModel is a RunningModel for an onnx model served in-process
// (rather than as a child process, like the llama backend): quitting and
// killing it shut the http.Server down, and waiting for it waits for the
// server to stop.
type onnxRunningModel struct {
	modelName      string
	runtimeVersion string
	startedAt      time.Time
	logs           *runnerDefinitions.LogBuffer
	logger         *log.Logger
	model          *onnxEngine.Model
	graph          *onnxGraph.Graph
	server         *http.Server
	done           chan struct{}
	runMu          sync.Mutex // serializes inference runs
	quitOnce       sync.Once
	killOnce       sync.Once
}

var _ runnerDefinitions.RunningModel = (*onnxRunningModel)(nil)

// ModelName implements [runnerDefinitions.RunningModel].
func (g *onnxRunningModel) ModelName() string {
	return g.modelName
}

// ProviderName implements [runnerDefinitions.RunningModel].
func (g *onnxRunningModel) ProviderName() string {
	return providerId
}

// RuntimeVersion implements [runnerDefinitions.RunningModel].
func (g *onnxRunningModel) RuntimeVersion() string {
	return g.runtimeVersion
}

// Uptime implements [runnerDefinitions.RunningModel].
func (g *onnxRunningModel) Uptime() time.Duration {
	return time.Since(g.startedAt)
}

// Logs implements [runnerDefinitions.RunningModel].
func (g *onnxRunningModel) Logs() []string {
	return g.logs.Lines()
}

// Wait implements [runnerDefinitions.RunningModel]. It blocks until the
// server has stopped.
func (g *onnxRunningModel) Wait() {
	<-g.done
}

// SendSigQuit implements [runnerDefinitions.RunningModel]. It asks the server
// to shut down gracefully: no new connections, in-flight requests finish.
func (g *onnxRunningModel) SendSigQuit() {
	g.quitOnce.Do(func() {
		g.logger.Printf("stopping %s (graceful shutdown requested)", g.modelName)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := g.server.Shutdown(ctx); err != nil {
			g.logger.Printf("error: graceful shutdown: %v", err)
		}
	})
}

// SendSigKill implements [runnerDefinitions.RunningModel]. It drops the
// server immediately, including in-flight requests.
func (g *onnxRunningModel) SendSigKill() {
	g.killOnce.Do(func() {
		g.logger.Printf("killing %s", g.modelName)
		g.server.Close()
	})
}

// inferenceRequest is the body of a POST /inference request: one entry per
// model input, in the model's input order. Each entry carries a flat list of
// float values; when its shape is omitted, the shape the model declared for
// that input is used.
type inferenceRequest struct {
	Inputs []inferenceInput `json:"inputs"`
}

type inferenceInput struct {
	Shape []int     `json:"shape,omitempty"`
	Data  []float64 `json:"data"`
}

// inferenceResponse is the reply to a successful inference: one flat list of
// values per model output, in the model's output order.
type inferenceResponse struct {
	Outputs [][]float64 `json:"outputs"`
}

// handleInfo answers GET / with a small JSON summary of what is being served.
func (g *onnxRunningModel) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider": providerId,
		"model":    g.modelName,
		"version":  g.runtimeVersion,
		"uptime":   g.Uptime().String(),
		"inputs":   len(g.model.GetInputTensors()),
	})
}

// handleInference serves one inference: it sets the request's inputs on the
// loaded model, runs the graph once, and replies with the model's outputs.
// The onnx-go graph is not safe for concurrent runs, so runs are serialized.
func (g *onnxRunningModel) handleInference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req inferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	if len(req.Inputs) == 0 {
		http.Error(w, "no inputs provided", http.StatusBadRequest)
		return
	}

	declared := g.model.GetInputTensors()
	for i, in := range req.Inputs {
		shape := in.Shape
		if len(shape) == 0 {
			if i >= len(declared) || declared[i] == nil {
				http.Error(w, fmt.Sprintf("input %d: the model declares no shape for it - provide one in the request", i), http.StatusBadRequest)
				return
			}
			shape = declared[i].Shape()
		}
		t, err := newTensor(shape, in.Data)
		if err != nil {
			http.Error(w, fmt.Sprintf("input %d: %v", i, err), http.StatusBadRequest)
			return
		}
		if err := g.model.SetInput(i, t); err != nil {
			http.Error(w, fmt.Sprintf("input %d: %v", i, err), http.StatusBadRequest)
			return
		}
	}

	started := time.Now()
	g.runMu.Lock()
	runErr := g.graph.Run()
	g.runMu.Unlock()
	if runErr != nil {
		g.logger.Printf("inference failed: %v", runErr)
		http.Error(w, fmt.Sprintf("inference failed: %v", runErr), http.StatusInternalServerError)
		return
	}
	outputs, err := g.model.GetOutputTensors()
	if err != nil {
		g.logger.Printf("reading the outputs failed: %v", err)
		http.Error(w, fmt.Sprintf("reading the outputs failed: %v", err), http.StatusInternalServerError)
		return
	}

	resp := inferenceResponse{Outputs: make([][]float64, len(outputs))}
	for i, out := range outputs {
		resp.Outputs[i] = flattenTensor(out)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
	g.logger.Printf("inference completed in %s", time.Since(started).Round(time.Millisecond))
}

// newTensor builds a float32 tensor with the given shape from the given flat
// values, as the onnx-go backends expect for model inputs.
func newTensor(shape []int, values []float64) (tensor.Tensor, error) {
	size := 1
	for _, dim := range shape {
		if dim <= 0 {
			return nil, fmt.Errorf("invalid shape %v (every dimension must be > 0)", shape)
		}
		size *= dim
	}
	if len(values) != size {
		return nil, fmt.Errorf("data has %d values, shape %v holds %d", len(values), shape, size)
	}
	backing := make([]float32, len(values))
	for i, v := range values {
		backing[i] = float32(v)
	}
	return tensor.New(tensor.WithShape(shape...), tensor.WithBacking(backing)), nil
}

// flattenTensor flattens a tensor's values into a single row-major list of
// floats, for the JSON response.
func flattenTensor(t tensor.Tensor) []float64 {
	if t == nil {
		return nil
	}
	switch data := t.Data().(type) {
	case []float32:
		out := make([]float64, len(data))
		for i, v := range data {
			out[i] = float64(v)
		}
		return out
	case []float64:
		return data
	}
	// any other numeric dtype: walk the flat backing slice generically
	val := reflect.ValueOf(t.Data())
	if val.Kind() != reflect.Slice {
		return nil
	}
	out := make([]float64, val.Len())
	for i := range out {
		out[i] = val.Index(i).Float()
	}
	return out
}

// requestLogger wraps a handler, logging one line per request through the
// model's logger (and so into both the log buffer and the console), so the
// log viewer and the console show what the server is doing.
func requestLogger(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logger.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, recorder.status, time.Since(started).Round(time.Millisecond))
	})
}

// statusRecorder captures the response status so requestLogger can log it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader implements http.ResponseWriter.
func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
