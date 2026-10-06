package running

import (
	"testing"

	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// fakeRunningModel is a minimal runnerDefinitions.RunningModel for tests. The
// tests only read its name; the signal methods are no-ops because a fake is
// never signalled.
type fakeRunningModel struct {
	name string
}

// ModelName implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) ModelName() string { return f.name }

// ProviderName implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) ProviderName() string { return "" }

// RuntimeVersion implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) RuntimeVersion() string { return "" }

// Wait implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) Wait() {}

// SendSigQuit implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) SendSigQuit() {}

// SendSigKill implements [runnerDefinitions.RunningModel].
func (f *fakeRunningModel) SendSigKill() {}

var _ runnerDefinitions.RunningModel = (*fakeRunningModel)(nil)

func TestTrackAndStop(t *testing.T) {
	r := NewRegistry()

	a := &fakeRunningModel{name: "model-a"}
	b := &fakeRunningModel{name: "model-b"}

	stopA := r.Track(a)
	stopB := r.Track(b)

	if got := r.Names(); len(got) != 2 || got[0] != "model-a" || got[1] != "model-b" {
		t.Fatalf("unexpected names: %v", got)
	}

	stopA()
	stopA() // idempotent

	if got := r.Names(); len(got) != 1 || got[0] != "model-b" {
		t.Fatalf("unexpected names after stop: %v", got)
	}

	stopB()
	if got := len(r.RunningModels()); got != 0 {
		t.Fatalf("expected no models, got: %d", got)
	}
}

func TestModelsSorted(t *testing.T) {
	r := NewRegistry()
	r.Track(&fakeRunningModel{name: "zeta"})
	r.Track(&fakeRunningModel{name: "alpha"})

	got := r.Names()
	if len(got) != 2 || got[0] != "alpha" || got[1] != "zeta" {
		t.Fatalf("unexpected sorted names: %v", got)
	}
}
