package running

import (
	"testing"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
)

func TestTrackAndStop(t *testing.T) {
	r := NewRegistry()

	a := modelDefinitions.Model{ModelName: "model-a", ModelType: "gguf"}
	b := modelDefinitions.Model{ModelName: "model-b", ModelType: "gguf"}

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
	if got := r.Models(); len(got) != 0 {
		t.Fatalf("expected no models, got: %v", got)
	}
}

func TestModelsSorted(t *testing.T) {
	r := NewRegistry()
	r.Track(modelDefinitions.Model{ModelName: "zeta"})
	r.Track(modelDefinitions.Model{ModelName: "alpha"})

	got := r.Names()
	if len(got) != 2 || got[0] != "alpha" || got[1] != "zeta" {
		t.Fatalf("unexpected sorted names: %v", got)
	}
}
