package backend

import (
	"slices"

	"github.com/kncept/quesadilla/backend/definitions"
	"github.com/kncept/quesadilla/backend/gogguf"
	"github.com/kncept/quesadilla/backend/llama"
)

// Repository maintains the application's backends. The backends (and their
// runners) are created once, when the repository is created, and shared from
// then on rather than being rebuilt on every lookup.
type Repository struct {
	backends []definitions.Backend
}

// NewRepository creates a backend repository, scanning for the known
// backends as it is created.
func NewRepository() *Repository {
	repository := &Repository{}
	repository.scan()
	return repository
}

// scan (re)builds the set of known backends. Being the single owner of the
// backend structs means their runners keep their state (installed versions,
// downloads, ...) between calls.
func (this *Repository) scan() {
	this.backends = []definitions.Backend{
		llama.LlamaBackend(),
		gogguf.GoggufBackend(),
	}
}

// Backends returns every known backend.
func (this *Repository) Backends() []definitions.Backend {
	return this.backends
}

// Backend returns the backend with the given id, or nil when absent.
func (this *Repository) Backend(id string) definitions.Backend {
	for _, b := range this.backends {
		if b.Id() == id {
			return b
		}
	}
	return nil
}

// BackendForModelType returns a backend that can run the given model type
// and has at least one runner registered.
func (this *Repository) BackendForModelType(modelType string) definitions.Backend {
	for _, backend := range this.backends {
		if slices.Contains(backend.ModelTypes(), modelType) && len(backend.InstalledVersions()) > 0 {
			return backend
		}
	}
	return nil
}
