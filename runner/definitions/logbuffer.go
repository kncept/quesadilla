package definitions

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// DefaultMaxLogLines is the default number of recent log lines a running model
// keeps in memory for the log viewer.
const DefaultMaxLogLines = 200

// LogBuffer is a thread-safe, bounded buffer of recent log lines. It keeps at
// most maxLines lines, evicting the oldest when it overflows. It is safe for
// concurrent use: a runner writes into it from a process's stdout/stderr while
// the GUI reads it for the log viewer.
type LogBuffer struct {
	mu       sync.Mutex
	maxLines int
	lines    []string
}

// NewLogBuffer creates a LogBuffer that keeps at most maxLines recent lines.
// A non-positive maxLines falls back to DefaultMaxLogLines.
func NewLogBuffer(maxLines int) *LogBuffer {
	if maxLines <= 0 {
		maxLines = DefaultMaxLogLines
	}
	return &LogBuffer{maxLines: maxLines}
}

// AddLine appends a single line, evicting the oldest line when the buffer is
// full.
func (b *LogBuffer) AddLine(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, line)
	if len(b.lines) > b.maxLines {
		b.lines = b.lines[len(b.lines)-b.maxLines:]
	}
}

// Lines returns a copy of the buffered lines, oldest first.
func (b *LogBuffer) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}

// Writer returns a new io.Writer that feeds complete lines into the buffer.
// Each call returns an independent writer, so one can be used for a process's
// stdout and another for its stderr without their partial lines interfering.
// Lines are held until newline-terminated, with the line ending and any
// trailing carriage return removed.
func (b *LogBuffer) Writer() io.Writer {
	return &lineSplitter{buffer: b}
}

// lineSplitter is an io.Writer that splits a byte stream on newlines and feeds
// each complete line to a LogBuffer. It is safe for concurrent use.
type lineSplitter struct {
	mu      sync.Mutex
	buffer  *LogBuffer
	pending []byte
}

// Write implements io.Writer.
func (w *lineSplitter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	for {
		idx := bytes.IndexByte(w.pending, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimSuffix(string(w.pending[:idx]), "\r")
		w.pending = w.pending[idx+1:]
		w.buffer.AddLine(line)
	}
	return len(p), nil
}
