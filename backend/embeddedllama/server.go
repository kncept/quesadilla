package embeddedllama

import (
	"log"
	"net/http"
	"time"
)

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

// Flush forwards to the wrapped writer, so streaming handlers still see an
// http.Flusher through the request-logging wrapper. Without it the wrapper
// hides the underlying flusher, every streaming handler's flusher type
// assertion fails, and streamed replies turn into 500s.
func (s *statusRecorder) Flush() {
	if flusher, ok := s.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap returns the wrapped writer, letting http.ResponseController reach the
// real connection through the wrapper.
func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
