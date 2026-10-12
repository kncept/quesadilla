//go:build cgo

package embeddedllama

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	llama "github.com/tcpipuk/llama-go"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// runtimeAvailable reports whether the embedded llama.cpp runtime is compiled
// into this binary. It is true in every cgo build, where the llama-go bindings
// and the llama.cpp static libraries are linked in.
const runtimeAvailable = true

// servePort is the port the in-process embedded-llama server listens on. The
// llama backend uses 9931, gogguf 9932 and onnx 9933, so embedded-llama takes
// the next one, letting all four backends serve at the same time. (As with the
// others, only one model per backend can be served at once: a second start
// fails with "address already in use".)
const servePort = 9934

// contextSize is the token window each served model gets. It is deliberately
// smaller than most models' native maximum, so a model cannot allocate an
// enormous KV cache by default.
const contextSize = 8192

// startEmbeddedLlamaServer loads the model with llama-go and serves it
// in-process over an OpenAI-compatible HTTP API. It returns once the server is
// listening, so a port conflict or a failed model load surfaces as an error
// here rather than later.
func startEmbeddedLlamaServer(m *modelDefinitions.Model) (runnerDefinitions.RunningModel, error) {
	// Keep the last N lines in memory for the log viewer, like the other
	// backends do. A log.Logger over the buffer's writer feeds it complete
	// lines.
	logs := runnerDefinitions.NewLogBuffer(runnerDefinitions.DefaultMaxLogLines)
	// Every line is also printed to the console (os.Stdout), so the start,
	// stop and error events are visible even when no GUI is watching the log
	// viewer.
	logger := log.New(io.MultiWriter(logs.Writer(), os.Stdout), providerId+": ", 0)
	logger.Printf("starting %s (runtime %s)", m.FullModelName(), builtInVersion)
	logger.Printf("loading model %s", m.ModelFile)

	// llama-go offloads every layer to the GPU when one is available and
	// falls back to the CPU when it is not, so -1 is safe on every host.
	model, err := llama.LoadModel(m.ModelFile,
		llama.WithGPULayers(-1),
		llama.WithProgressCallback(progressLogger(logger)),
	)
	if err != nil {
		logger.Printf("error: failed to load model: %v", err)
		return nil, fmt.Errorf("%s: loading model %s: %w", providerId, m.ModelFile, err)
	}
	logger.Printf("model loaded")

	// A llama-go Context is not thread-safe; runs are serialised on the
	// running model's runMu, so one context serves every request.
	llamaContext, err := model.NewContext(
		llama.WithContext(contextSize),
		llama.WithF16Memory(),
	)
	if err != nil {
		model.Close()
		logger.Printf("error: failed to create context: %v", err)
		return nil, fmt.Errorf("%s: creating context: %w", providerId, err)
	}
	logger.Printf("context ready (%d token window)", contextSize)

	// Bind the listener before returning, so Start only succeeds when the
	// server is really up.
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", servePort))
	if err != nil {
		llamaContext.Close()
		model.Close()
		logger.Printf("error: listening on localhost:%d: %v", servePort, err)
		return nil, fmt.Errorf("%s: listening on localhost:%d: %w", providerId, servePort, err)
	}

	g := &embeddedRunningModel{
		modelName:      m.FullModelName(),
		runtimeVersion: builtInVersion,
		startedAt:      time.Now(),
		logs:           logs,
		logger:         logger,
		model:          model,
		ctx:            llamaContext,
		done:           make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", g.handleInfo)
	mux.HandleFunc("/v1/models", g.handleModels)
	mux.HandleFunc("/v1/chat/completions", g.handleChatCompletions)
	mux.HandleFunc("/v1/completions", g.handleCompletions)
	mux.HandleFunc("/v1/responses", g.handleResponses)
	g.server = &http.Server{
		Addr:              fmt.Sprintf("localhost:%d", servePort),
		Handler:           requestLogger(logger, mux),
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          logger,
	}

	go g.serve(listener)
	return g, nil
}

// progressLogger returns a model-loading progress callback that logs every
// tenth of the load through the model's logger. The callback is invoked from a
// C thread, so the last-logged step is guarded by a mutex.
func progressLogger(logger *log.Logger) llama.ProgressCallback {
	var mu sync.Mutex
	last := -1
	return func(progress float32) bool {
		step := int(progress * 10)
		mu.Lock()
		defer mu.Unlock()
		if step > last {
			last = step
			logger.Printf("loading model: %d%%", step*10)
		}
		return true
	}
}

// serve runs the HTTP server until it is shut down (via SendSigQuit or
// SendSigKill), then closes the context and the model and signals Wait.
func (g *embeddedRunningModel) serve(listener net.Listener) {
	defer close(g.done)
	defer func() {
		if err := g.ctx.Close(); err != nil {
			g.logger.Printf("error: closing context: %v", err)
		}
		if err := g.model.Close(); err != nil {
			g.logger.Printf("error: closing model: %v", err)
		}
	}()
	g.logger.Printf("serving %s on http://%s", g.modelName, g.server.Addr)
	if serveErr := g.server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
		g.logger.Printf("error: server: %v", serveErr)
	}
	g.logger.Printf("stopped %s", g.modelName)
}

// embeddedRunningModel is a RunningModel for a llama-go model served
// in-process (rather than as a child process, like the llama backend):
// quitting and killing it shut the http.Server down, and waiting for it waits
// for the server to stop.
type embeddedRunningModel struct {
	modelName      string
	runtimeVersion string
	startedAt      time.Time
	logs           *runnerDefinitions.LogBuffer
	logger         *log.Logger
	model          *llama.Model
	ctx            *llama.Context
	server         *http.Server
	done           chan struct{}
	runMu          sync.Mutex // serialises runs: a llama-go Context is not thread-safe
	quitOnce       sync.Once
	killOnce       sync.Once
}

var _ runnerDefinitions.RunningModel = (*embeddedRunningModel)(nil)

// ModelName implements [runnerDefinitions.RunningModel].
func (g *embeddedRunningModel) ModelName() string {
	return g.modelName
}

// ProviderName implements [runnerDefinitions.RunningModel].
func (g *embeddedRunningModel) ProviderName() string {
	return providerId
}

// RuntimeVersion implements [runnerDefinitions.RunningModel].
func (g *embeddedRunningModel) RuntimeVersion() string {
	return g.runtimeVersion
}

// Uptime implements [runnerDefinitions.RunningModel].
func (g *embeddedRunningModel) Uptime() time.Duration {
	return time.Since(g.startedAt)
}

// Logs implements [runnerDefinitions.RunningModel].
func (g *embeddedRunningModel) Logs() []string {
	return g.logs.Lines()
}

// Wait implements [runnerDefinitions.RunningModel]. It blocks until the
// server has stopped.
func (g *embeddedRunningModel) Wait() {
	<-g.done
}

// SendSigQuit implements [runnerDefinitions.RunningModel]. It asks the server
// to shut down gracefully: no new connections, in-flight requests finish.
func (g *embeddedRunningModel) SendSigQuit() {
	g.quitOnce.Do(func() {
		g.logger.Printf("stopping %s (graceful shutdown requested)", g.modelName)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := g.server.Shutdown(ctx); err != nil {
			g.logger.Printf("error: graceful shutdown: %v", err)
		}
	})
}

// SendSigKill implements [runnerDefinitions.RunningModel]. It drops the server
// immediately, including in-flight requests.
func (g *embeddedRunningModel) SendSigKill() {
	g.killOnce.Do(func() {
		g.logger.Printf("killing %s", g.modelName)
		g.server.Close()
	})
}

// chatCompletionRequest is the body of a POST /v1/chat/completions request, in
// OpenAI's shape (the subset the backend honours).
type chatCompletionRequest struct {
	Model       string          `json:"model"`
	Messages    []chatMessage   `json:"messages"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Temperature *float32        `json:"temperature,omitempty"`
	TopP        *float32        `json:"top_p,omitempty"`
	TopK        *int            `json:"top_k,omitempty"`
	Seed        *int            `json:"seed,omitempty"`
	Stop        json.RawMessage `json:"stop,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
}

// chatMessage is one message of a chat conversation.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionResponse is the reply to a successful non-streaming chat
// completion.
type chatCompletionResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// chatCompletionChunk is one event of a streamed chat completion.
type chatCompletionChunk struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Created int64             `json:"created"`
	Model   string            `json:"model"`
	Choices []chatDeltaChoice `json:"choices"`
}

type chatDeltaChoice struct {
	Index        int         `json:"index"`
	Delta        chatMessage `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

// completionRequest is the body of a POST /v1/completions request, in OpenAI's
// shape (the subset the backend honours).
type completionRequest struct {
	Model       string          `json:"model"`
	Prompt      string          `json:"prompt"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Temperature *float32        `json:"temperature,omitempty"`
	TopP        *float32        `json:"top_p,omitempty"`
	TopK        *int            `json:"top_k,omitempty"`
	Seed        *int            `json:"seed,omitempty"`
	Stop        json.RawMessage `json:"stop,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
}

// completionResponse is the reply to a successful non-streaming text
// completion.
type completionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []completionChoice `json:"choices"`
}

type completionChoice struct {
	Text         string `json:"text"`
	Index        int    `json:"index"`
	FinishReason string `json:"finish_reason"`
}

// completionChunk is one event of a streamed text completion.
type completionChunk struct {
	ID      string                  `json:"id"`
	Object  string                  `json:"object"`
	Created int64                   `json:"created"`
	Model   string                  `json:"model"`
	Choices []completionChunkChoice `json:"choices"`
}

type completionChunkChoice struct {
	Text         string  `json:"text"`
	Index        int     `json:"index"`
	FinishReason *string `json:"finish_reason"`
}

// handleInfo answers GET / with a small JSON summary of what is being served.
func (g *embeddedRunningModel) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	g.writeJSON(w, map[string]any{
		"provider":     providerId,
		"model":        g.modelName,
		"version":      g.runtimeVersion,
		"uptime":       g.Uptime().String(),
		"context_size": contextSize,
	})
}

// handleModels answers GET /v1/models with the served model, in OpenAI's list
// shape.
func (g *embeddedRunningModel) handleModels(w http.ResponseWriter, r *http.Request) {
	g.writeJSON(w, map[string]any{
		"object": "list",
		"data": []map[string]any{{
			"id":       g.modelName,
			"object":   "model",
			"created":  g.startedAt.Unix(),
			"owned_by": providerId,
		}},
	})
}

// handleChatCompletions serves one chat completion, streaming it when the
// request asks for it.
func (g *embeddedRunningModel) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	if len(req.Messages) == 0 {
		http.Error(w, "no messages provided", http.StatusBadRequest)
		return
	}

	messages := make([]llama.ChatMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = llama.ChatMessage{Role: msg.Role, Content: msg.Content}
	}
	opts := llama.ChatOptions{
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		TopK:        req.TopK,
		Seed:        req.Seed,
		StopWords:   parseStop(req.Stop),
	}

	if req.Stream {
		g.streamChatCompletion(w, r, messages, opts)
		return
	}

	started := time.Now()
	resp, err := g.chat(r.Context(), messages, opts)
	if err != nil {
		g.httpError(w, r, http.StatusInternalServerError, "chat failed: %v", err)
		return
	}
	g.writeJSON(w, chatCompletionResponse{
		ID:      newCompletionID("chatcmpl"),
		Object:  "chat.completion",
		Created: started.Unix(),
		Model:   g.modelName,
		Choices: []chatChoice{{
			Index:        0,
			Message:      chatMessage{Role: "assistant", Content: resp.Content},
			FinishReason: "stop",
		}},
	})
	g.logger.Printf("chat completed in %s", time.Since(started).Round(time.Millisecond))
}

// streamChatCompletion streams one chat completion as server-sent events, in
// OpenAI's streaming shape.
func (g *embeddedRunningModel) streamChatCompletion(w http.ResponseWriter, r *http.Request, messages []llama.ChatMessage, opts llama.ChatOptions) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		g.httpError(w, r, http.StatusInternalServerError, "streaming is not supported by this connection")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	id := newCompletionID("chatcmpl")
	created := time.Now().Unix()

	// The first event carries the assistant role, like OpenAI's.
	writeSSE(w, flusher, chatCompletionChunk{
		ID: id, Object: "chat.completion.chunk", Created: created, Model: g.modelName,
		Choices: []chatDeltaChoice{{Index: 0, Delta: chatMessage{Role: "assistant"}}},
	})

	g.runMu.Lock()
	deltaCh, errCh := g.ctx.ChatStream(r.Context(), messages, opts)
	for deltaCh != nil || errCh != nil {
		select {
		case delta, open := <-deltaCh:
			if !open {
				deltaCh = nil
				continue
			}
			writeSSE(w, flusher, chatCompletionChunk{
				ID: id, Object: "chat.completion.chunk", Created: created, Model: g.modelName,
				Choices: []chatDeltaChoice{{Index: 0, Delta: chatMessage{Content: delta.Content}}},
			})
		case err, open := <-errCh:
			if !open {
				errCh = nil
				continue
			}
			if err != nil {
				g.logger.Printf("error: %s %s: chat stream failed: %v", r.Method, r.URL.Path, err)
			}
		}
	}
	g.runMu.Unlock()

	finish := "stop"
	writeSSE(w, flusher, chatCompletionChunk{
		ID: id, Object: "chat.completion.chunk", Created: created, Model: g.modelName,
		Choices: []chatDeltaChoice{{Index: 0, FinishReason: &finish}},
	})
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
	g.logger.Printf("chat streamed in %s", time.Since(time.Unix(created, 0)).Round(time.Millisecond))
}

// handleCompletions serves one raw text completion, streaming it when the
// request asks for it.
func (g *embeddedRunningModel) handleCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req completionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	if req.Prompt == "" {
		http.Error(w, "no prompt provided", http.StatusBadRequest)
		return
	}

	opts := generateOptions(req)

	if req.Stream {
		g.streamCompletion(w, r, req.Prompt, opts)
		return
	}

	started := time.Now()
	g.runMu.Lock()
	text, err := g.ctx.Generate(req.Prompt, opts...)
	g.runMu.Unlock()
	if err != nil {
		g.httpError(w, r, http.StatusInternalServerError, "completion failed: %v", err)
		return
	}
	g.writeJSON(w, completionResponse{
		ID:      newCompletionID("cmpl"),
		Object:  "text_completion",
		Created: started.Unix(),
		Model:   g.modelName,
		Choices: []completionChoice{{Text: text, Index: 0, FinishReason: "stop"}},
	})
	g.logger.Printf("completion completed in %s", time.Since(started).Round(time.Millisecond))
}

// streamCompletion streams one raw text completion as server-sent events, in
// OpenAI's streaming shape.
func (g *embeddedRunningModel) streamCompletion(w http.ResponseWriter, r *http.Request, prompt string, opts []llama.GenerateOption) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		g.httpError(w, r, http.StatusInternalServerError, "streaming is not supported by this connection")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	id := newCompletionID("cmpl")
	created := time.Now().Unix()

	g.runMu.Lock()
	tokenCh, errCh := g.ctx.GenerateChannel(r.Context(), prompt, opts...)
	for tokenCh != nil || errCh != nil {
		select {
		case token, open := <-tokenCh:
			if !open {
				tokenCh = nil
				continue
			}
			writeSSE(w, flusher, completionChunk{
				ID: id, Object: "text_completion", Created: created, Model: g.modelName,
				Choices: []completionChunkChoice{{Text: token, Index: 0}},
			})
		case err, open := <-errCh:
			if !open {
				errCh = nil
				continue
			}
			if err != nil {
				g.logger.Printf("error: %s %s: completion stream failed: %v", r.Method, r.URL.Path, err)
			}
		}
	}
	g.runMu.Unlock()

	finish := "stop"
	writeSSE(w, flusher, completionChunk{
		ID: id, Object: "text_completion", Created: created, Model: g.modelName,
		Choices: []completionChunkChoice{{Index: 0, FinishReason: &finish}},
	})
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// chat runs a chat completion, serialised against other runs on the same
// context.
func (g *embeddedRunningModel) chat(ctx context.Context, messages []llama.ChatMessage, opts llama.ChatOptions) (*llama.ChatResponse, error) {
	g.runMu.Lock()
	defer g.runMu.Unlock()
	return g.ctx.Chat(ctx, messages, opts)
}

// generateOptions turns a completion request into llama-go generation options.
func generateOptions(req completionRequest) []llama.GenerateOption {
	var opts []llama.GenerateOption
	if req.MaxTokens != nil {
		opts = append(opts, llama.WithMaxTokens(*req.MaxTokens))
	}
	if req.Temperature != nil {
		opts = append(opts, llama.WithTemperature(*req.Temperature))
	}
	if req.TopP != nil {
		opts = append(opts, llama.WithTopP(*req.TopP))
	}
	if req.TopK != nil {
		opts = append(opts, llama.WithTopK(*req.TopK))
	}
	if req.Seed != nil {
		opts = append(opts, llama.WithSeed(*req.Seed))
	}
	if stop := parseStop(req.Stop); len(stop) > 0 {
		opts = append(opts, llama.WithStopWords(stop...))
	}
	return opts
}

// parseStop decodes OpenAI's stop parameter, which is either a single string
// or a list of strings.
func parseStop(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if single == "" {
			return nil
		}
		return []string{single}
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return many
	}
	return nil
}

// newCompletionID builds an OpenAI-style completion id, eg "chatcmpl-1a2b3c".
func newCompletionID(prefix string) string {
	return fmt.Sprintf("%s-%x", prefix, time.Now().UnixNano())
}

// httpError logs why a request failed and then writes the error response.
// Every server-side failure (5xx) goes through it, so the reason ends up on the
// console and in the GUI log viewer instead of only in the response body.
func (g *embeddedRunningModel) httpError(w http.ResponseWriter, r *http.Request, status int, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	g.logger.Printf("error: %s %s -> %d: %s", r.Method, r.URL.Path, status, message)
	http.Error(w, message, status)
}

// writeJSON writes a JSON response. It encodes to memory first so an encoding
// failure can be logged and reported as a 500, rather than silently truncating
// the body.
func (g *embeddedRunningModel) writeJSON(w http.ResponseWriter, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		g.logger.Printf("error: encoding JSON response: %v", err)
		http.Error(w, fmt.Sprintf("encoding response: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// writeSSE writes one server-sent event and flushes it.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
