package backend

import (
	"slices"

	"github.com/kncept/quesadilla/backend/definitions"
	"github.com/kncept/quesadilla/backend/llama"
)

// stub, will expand to include more backends
func Backends() []definitions.Backend {
	return []definitions.Backend{
		llama.LlamaBackend(),
	}
}

func Backend(id string) definitions.Backend {
	for _, b := range Backends() {
		if b.Id() == id {
			return b
		}
	}
	return nil
}

func BackendForModelType(modelType string) definitions.Backend {
	for _, backend := range Backends() {
		// valid for model type
		// and has at least ONE runner
		if slices.Contains(backend.ModelTypes(), modelType) && hasRunner(backend) {
			return backend
		}
	}
	return nil
}

// hasRunner reports whether the backend has at least one runner registered,
// so it can provide (and run) versions of itself.
func hasRunner(backend definitions.Backend) bool {
	sb, ok := backend.(*definitions.StandardBackend)
	if !ok {
		return true //non-standard backends manage their own runners
	}
	return len(sb.Runners()) > 0
}
