// Package running keeps a process-wide view of the models that are
// currently being served, so that any part of the app (CLI, GUI) can
// report what is running.
package running

import (
	"sort"
	"sync"

	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// Registry is a thread-safe list of the models currently running.
type Registry struct {
	mu            sync.Mutex
	runningModels []runnerDefinitions.RunningModel
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
func (r *Registry) Track(runningModel runnerDefinitions.RunningModel) func() {
	r.mu.Lock()
	r.runningModels = append(r.runningModels, runningModel)
	r.mu.Unlock()

	var stopOnce sync.Once
	return func() {
		stopOnce.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			for i, m := range r.runningModels {
				if m == runningModel {
					r.runningModels = append(r.runningModels[:i], r.runningModels[i+1:]...)
					break
				}
			}
		})
	}
}

// Models returns the models currently running, sorted by name.
func (r *Registry) RunningModels() []runnerDefinitions.RunningModel {
	r.mu.Lock()
	defer r.mu.Unlock()
	runningModels := make([]runnerDefinitions.RunningModel, len(r.runningModels))
	copy(runningModels, r.runningModels)
	sort.Slice(runningModels, func(i, j int) bool {
		return runningModels[i].ModelName() < runningModels[j].ModelName()
	})
	return runningModels
}

// Names returns the names of the models currently running, sorted.
func (r *Registry) Names() []string {
	models := r.RunningModels()
	names := make([]string, len(models))
	for i, m := range models {
		names[i] = m.ModelName()
	}
	return names
}
