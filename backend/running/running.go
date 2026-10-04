// Package running keeps a process-wide view of the models that are
// currently being served, so that any part of the app (CLI, GUI) can
// report what is running.
package running

import (
	"sort"
	"sync"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
)

// Registry is a thread-safe list of the models currently running.
type Registry struct {
	mu     sync.Mutex
	models []modelDefinitions.Model
}

func NewRegistry() *Registry {
	return &Registry{}
}

var defaultRegistry = NewRegistry()

// Default is the process-wide registry used by the backends and the GUI.
func Default() *Registry {
	return defaultRegistry
}

// Track registers model as running and returns a function that stops
// tracking it. The returned function is safe to call more than once
// (eg via defer) and from any goroutine.
func (r *Registry) Track(model modelDefinitions.Model) func() {
	r.mu.Lock()
	r.models = append(r.models, model)
	r.mu.Unlock()

	var stopOnce sync.Once
	return func() {
		stopOnce.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			for i, m := range r.models {
				if m == model {
					r.models = append(r.models[:i], r.models[i+1:]...)
					break
				}
			}
		})
	}
}

// Models returns the models currently running, sorted by name.
func (r *Registry) Models() []modelDefinitions.Model {
	r.mu.Lock()
	defer r.mu.Unlock()
	models := make([]modelDefinitions.Model, len(r.models))
	copy(models, r.models)
	sort.Slice(models, func(i, j int) bool {
		return models[i].ModelName < models[j].ModelName
	})
	return models
}

// Names returns the names of the models currently running, sorted.
func (r *Registry) Names() []string {
	models := r.Models()
	names := make([]string, len(models))
	for i, m := range models {
		names[i] = m.ModelName
	}
	return names
}
