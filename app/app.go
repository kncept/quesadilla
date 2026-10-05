// Package app is the single entry point into the application's data. It owns
// the backend and model repositories and hands them out, so the rest of the
// app shares one set of repositories rather than rebuilding them.
package app

import (
	"log"
	"sync"

	"github.com/kncept/quesadilla/backend"
	"github.com/kncept/quesadilla/backend/definitions"
	"github.com/kncept/quesadilla/backend/running"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	"github.com/kncept/quesadilla/model/repository"
)

// QApp keeps references to the application's backend and model repositories.
// Both repositories are created (and scanned) once, when QApp is created. It
// also owns the running-model registry and the wait group that back [QApp.Start]
// and [QApp.AwaitAll], so callers can launch models without blocking and later
// wait for them all to finish.
type QApp struct {
	Backends *backend.Repository
	Models   *repository.Repository

	registry         *running.Registry
	allRunsWaitGroup sync.WaitGroup
}

// New creates a QApp, creating both repositories. Each repository scans for
// its contents as it is created.
func New() *QApp {
	return &QApp{
		Backends: backend.NewRepository(),
		Models:   repository.NewRepository(),
		registry: running.NewRegistry(),
	}
}

// Start runs the model on the given backend without blocking. It launches the
// backend's run in its own goroutine and records the model as running for as
// long as that run lives, so AwaitAll can later wait for it (and any other
// started models) to finish.
func (this *QApp) Start(backend definitions.Backend, model *modelDefinitions.Model) *sync.WaitGroup {
	stopTracking := this.registry.Track(*model)
	this.allRunsWaitGroup.Add(1)
	singleAwaitGroup := new(sync.WaitGroup)
	singleAwaitGroup.Add(1)
	go func() {
		defer this.allRunsWaitGroup.Done()
		defer singleAwaitGroup.Done()
		defer stopTracking()
		if err := backend.Run(model); err != nil {
			// The run lives in its own goroutine, so a failure can't be
			// returned to the caller; report it here instead.
			log.Printf("app: running %s failed: %v", model.ModelName, err)
		}
	}()
	return singleAwaitGroup
}

// AwaitAll blocks until every model started via Start has finished running.
func (this *QApp) AwaitAll() {
	this.allRunsWaitGroup.Wait()
}

// RunningModels returns the models currently running, sorted by name.
func (this *QApp) RunningModels() []modelDefinitions.Model {
	return this.registry.Models()
}
