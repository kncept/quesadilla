package definitions

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestNewLogBufferDefault(t *testing.T) {
	// a non-positive maxLines falls back to the default
	b := NewLogBuffer(0)
	if b.maxLines != DefaultMaxLogLines {
		t.Fatalf("maxLines = %d, want default %d", b.maxLines, DefaultMaxLogLines)
	}
	if b2 := NewLogBuffer(-5); b2.maxLines != DefaultMaxLogLines {
		t.Fatalf("maxLines for negative input = %d, want default %d", b2.maxLines, DefaultMaxLogLines)
	}
}

func TestLogBufferBounded(t *testing.T) {
	b := NewLogBuffer(3)
	for i := 1; i <= 5; i++ {
		b.AddLine(fmt.Sprintf("line-%d", i))
	}
	// only the last 3 lines are kept, oldest evicted
	want := []string{"line-3", "line-4", "line-5"}
	if got := b.Lines(); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("Lines() = %v, want %v", got, want)
	}
}

func TestLogBufferEmpty(t *testing.T) {
	b := NewLogBuffer(3)
	if got := b.Lines(); len(got) != 0 {
		t.Fatalf("Lines() = %v, want empty", got)
	}
}

func TestLogBufferLinesReturnsCopy(t *testing.T) {
	b := NewLogBuffer(3)
	b.AddLine("original")
	lines := b.Lines()
	lines[0] = "mutated"
	if got := b.Lines(); got[0] != "original" {
		t.Fatalf("mutating the returned slice changed the buffer: %v", got)
	}
}

func TestLogBufferWriterSplitsLines(t *testing.T) {
	b := NewLogBuffer(0)
	w := b.Writer()

	// a complete line is captured immediately
	if _, err := w.Write([]byte("first line\n")); err != nil {
		t.Fatal(err)
	}
	// a partial line is held until it is newline-terminated
	if _, err := w.Write([]byte("second li")); err != nil {
		t.Fatal(err)
	}
	if got := b.Lines(); len(got) != 1 || got[0] != "first line" {
		t.Fatalf("Lines() after partial write = %v, want [first line]", got)
	}
	// finishing the line (with a CR, as from a CRLF stream) captures it
	if _, err := w.Write([]byte("ne\r\n")); err != nil {
		t.Fatal(err)
	}
	want := []string{"first line", "second line"}
	if got := b.Lines(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Lines() = %v, want %v", got, want)
	}
}

func TestLogBufferWriterIndependentStreams(t *testing.T) {
	// two writers (eg stdout and stderr) keep their partial lines separate
	b := NewLogBuffer(0)
	out := b.Writer()
	errOut := b.Writer()

	if _, err := out.Write([]byte("std out part")); err != nil {
		t.Fatal(err)
	}
	if _, err := errOut.Write([]byte("std err part\n")); err != nil {
		t.Fatal(err)
	}
	// the stderr line is complete; the stdout partial is still held
	if got := b.Lines(); len(got) != 1 || got[0] != "std err part" {
		t.Fatalf("Lines() = %v, want [std err part]", got)
	}
	if _, err := out.Write([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	if got := b.Lines(); len(got) != 2 || got[1] != "std out part" {
		t.Fatalf("Lines() = %v, want [std err part std out part]", got)
	}
}

func TestLogBufferConcurrentWrites(t *testing.T) {
	b := NewLogBuffer(100)
	const goroutines = 8
	const perGoroutine = 200

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			w := b.Writer()
			for i := 0; i < perGoroutine; i++ {
				_, _ = w.Write([]byte(fmt.Sprintf("g%d-%d\n", g, i)))
			}
		}(g)
	}
	wg.Wait()

	if got := len(b.Lines()); got != 100 {
		t.Fatalf("Lines() len = %d, want the bounded max 100", got)
	}
	// the buffer must hold only complete, well-formed lines
	for _, line := range b.Lines() {
		if strings.Contains(line, "\n") || line == "" {
			t.Fatalf("unexpected line %q", line)
		}
	}
}
