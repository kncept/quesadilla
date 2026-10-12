package app

import (
	"slices"
	"testing"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
)

// TestSupportedModelTypes verifies the app reports the union of the model
// types its backends support (now gguf and onnx), sorted and unique.
func TestSupportedModelTypes(t *testing.T) {
	qApp := New()
	got := qApp.SupportedModelTypes()

	for _, want := range []string{string(modelDefinitions.ModelTypeGGUF), string(modelDefinitions.ModelTypeONNX)} {
		if !slices.Contains(got, want) {
			t.Errorf("SupportedModelTypes() = %v, missing %q", got, want)
		}
	}

	// the list is sorted and unique
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("SupportedModelTypes() = %v is not sorted and unique", got)
		}
	}
}
