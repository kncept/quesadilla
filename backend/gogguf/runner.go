package gogguf

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	goggufEngine "github.com/magomedcoder/gogguf"
	goggufServer "github.com/magomedcoder/gogguf/server"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// servePort is the port the in-process gogguf server listens on. The llama
// backend uses 9931, so gogguf takes the next one, letting both backends
// serve at the same time. (As with llama, only one model per backend can be
// served at once: a second start fails with "address already in use".)
const servePort = 9932

// startGoggufServer loads the model with gogguf and serves it in-process
// over an OpenAI-compatible HTTP API. It returns once the server is
// listening, so a port conflict or a failed model load surfaces as an error
// here rather than later.
func startGoggufServer(m *modelDefinitions.Model) (runnerDefinitions.RunningModel, error) {
	// Keep the last N lines in memory for the log viewer, like the llama
	// backend does for its server process. A log.Logger over the buffer's
	// writer feeds it complete lines.
	logs := runnerDefinitions.NewLogBuffer(runnerDefinitions.DefaultMaxLogLines)
	logger := log.New(logs.Writer(), providerId+": ", 0)
	logger.Printf("loading model %s (runtime %s %s)", m.ModelFile, providerId, builtInVersion)

	// LoadMapped reads the weights through an mmap (zero-copy); this is the
	// path the darwin/arm64 patch in nkrul/gogguf enables.
	engine, err := goggufEngine.LoadMapped(m.ModelFile, goggufEngine.LoadOptions{})
	if err != nil {
		logger.Printf("failed to load model: %v", err)
		return nil, fmt.Errorf("%s: loading model %s: %w", providerId, m.ModelFile, err)
	}

	// Bind the listener before returning, so Start only succeeds when the
	// server is really up.
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", servePort))
	if err != nil {
		engine.Close()
		return nil, fmt.Errorf("%s: listening on localhost:%d: %w", providerId, servePort, err)
	}

	g := &goggufRunningModel{
		modelName:      m.ModelName,
		runtimeVersion: builtInVersion,
		startedAt:      time.Now(),
		logs:           logs,
		logger:         logger,
		engine:         engine,
		server: &http.Server{
			Addr:              fmt.Sprintf("localhost:%d", servePort),
			Handler:           requestLogger(logs, goggufServer.New(engine, m.ModelFile).Handler()),
			ReadHeaderTimeout: 10 * time.Second,
			ErrorLog:          logger,
		},
		done: make(chan struct{}),
	}

	go g.serve(listener)
	return g, nil
}

// serve runs the HTTP server until it is shut down (via SendSigQuit or
// SendSigKill), then closes the engine and signals Wait.
func (g *goggufRunningModel) serve(listener net.Listener) {
	defer close(g.done)
	defer func() {
		if closeErr := g.engine.Close(); closeErr != nil {
			g.logger.Printf("closing engine: %v", closeErr)
		}
	}()
	g.logger.Printf("serving %s on http://%s", g.modelName, g.server.Addr)
	if serveErr := g.server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
		g.logger.Printf("server error: %v", serveErr)
	}
}

// goggufRunningModel is a RunningModel for a gogguf server running in-process
// (rather than as a child process, like the llama backend): quitting and
// killing it shut the http.Server down, and waiting for it waits for the
// server to stop.
type goggufRunningModel struct {
	modelName      string
	runtimeVersion string
	startedAt      time.Time
	logs           *runnerDefinitions.LogBuffer
	logger         *log.Logger
	engine         *goggufEngine.Engine
	server         *http.Server
	done           chan struct{}
	quitOnce       sync.Once
	killOnce       sync.Once
}

var _ runnerDefinitions.RunningModel = (*goggufRunningModel)(nil)

// ModelName implements [runnerDefinitions.RunningModel].
func (g *goggufRunningModel) ModelName() string {
	return g.modelName
}

// ProviderName implements [runnerDefinitions.RunningModel].
func (g *goggufRunningModel) ProviderName() string {
	return providerId
}

// RuntimeVersion implements [runnerDefinitions.RunningModel].
func (g *goggufRunningModel) RuntimeVersion() string {
	return g.runtimeVersion
}

// Uptime implements [runnerDefinitions.RunningModel].
func (g *goggufRunningModel) Uptime() time.Duration {
	return time.Since(g.startedAt)
}

// Logs implements [runnerDefinitions.RunningModel].
func (g *goggufRunningModel) Logs() []string {
	return g.logs.Lines()
}

// Wait implements [runnerDefinitions.RunningModel]. It blocks until the
// server has stopped.
func (g *goggufRunningModel) Wait() {
	<-g.done
}

// SendSigQuit implements [runnerDefinitions.RunningModel]. It asks the server
// to shut down gracefully: no new connections, in-flight requests finish.
func (g *goggufRunningModel) SendSigQuit() {
	g.quitOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := g.server.Shutdown(ctx); err != nil {
			g.logger.Printf("graceful shutdown: %v", err)
		}
	})
}

// SendSigKill implements [runnerDefinitions.RunningModel]. It drops the
// server immediately, including in-flight requests.
func (g *goggufRunningModel) SendSigKill() {
	g.killOnce.Do(func() {
		g.server.Close()
	})
}

// requestLogger wraps a handler, logging one line per request into the
// model's log buffer, so the log viewer shows what the server is doing.
func requestLogger(logs *runnerDefinitions.LogBuffer, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		logs.AddLine(fmt.Sprintf("%s %s -> %d (%s)", r.Method, r.URL.Path, recorder.status, time.Since(started).Round(time.Millisecond)))
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
