// Package app is the single entry point into the application's data. It owns
// the backend and model repositories and hands them out, so the rest of the
// app shares one set of repositories rather than rebuilding them.
package app

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/kncept/quesadilla/backend"
	backendDefinitions "github.com/kncept/quesadilla/backend/definitions"
	"github.com/kncept/quesadilla/backend/running"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	"github.com/kncept/quesadilla/model/repository"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// QApp keeps references to the application's backend and model repositories.
// Both repositories are created (and scanned) once, when QApp is created. It
// tracks the running models in the process-wide registry (shared with the GUI
// and the tray) and owns the wait group that back [QApp.Start] and
// [QApp.AwaitAll], so callers can launch models without blocking and later
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
		// the process-wide registry, so the GUI's running status and the
		// tray menu see the models QApp.Start has launched
		registry: running.Default(),
	}
}

// Start runs the model on the given backend without blocking. It launches the
// backend's run in its own goroutine and records the model as running for as
// long as that run lives, so AwaitAll can later wait for it (and any other
// started models) to finish.
func (this *QApp) Start(backend backendDefinitions.Backend, model *modelDefinitions.Model) *sync.WaitGroup {
	fmt.Printf("Starting %s on %s\n", model.ModelName, backend.Id())
	this.allRunsWaitGroup.Add(1)
	singleAwaitGroup := new(sync.WaitGroup)
	singleAwaitGroup.Add(1)
	waitForStart := new(sync.WaitGroup)
	waitForStart.Add(1)
	go func() {
		defer this.allRunsWaitGroup.Done()
		defer singleAwaitGroup.Done()
		runningModel, err := backend.Start(model)
		if err != nil {
			waitForStart.Done()
			// The run lives in its own goroutine, so a failure can't be
			// returned to the caller; report it here instead.
			log.Printf("app: running %s failed: %v", model.ModelName, err)
		}

		stopTracking := this.registry.Track(runningModel)
		defer stopTracking()

		// we have started, allow parent 'Start' function to complete
		waitForStart.Done()

		fmt.Printf("Started model %s on %s\n", model.ModelName, backend.Id())
		// now the goroutine waits, to make sure that 'on finish' events
		// (stopTracking and wait groups) trigger correctly
		runningModel.Wait()
	}()
	waitForStart.Wait()
	return singleAwaitGroup
}

// AwaitAll blocks until every model started via Start has finished running.
func (this *QApp) AwaitAll() {
	this.allRunsWaitGroup.Wait()
}

// RunningModels returns the models currently running, sorted by name.
func (this *QApp) RunningModels() []runnerDefinitions.RunningModel {
	return this.registry.RunningModels()
}

// Stop asks the named model to quit, if it is running. It reports whether a
// running model was found. The quit is requested asynchronously; the model
// leaves the registry when its process actually exits.
func (this *QApp) Stop(modelName string) bool {
	for _, m := range this.registry.RunningModels() {
		if m.ModelName() == modelName {
			m.SendSigQuit()
			return true
		}
	}
	return false
}

func (this *QApp) StopAll() {
	for _, m := range this.registry.RunningModels() {
		m.SendSigQuit()
	}
	sleepTime := 500 * time.Millisecond

	// wait up to 30 seconds for them all to finish
	endTime := time.Now().UnixMilli() + (30 * time.Second).Milliseconds()
	for time.Now().UnixMilli() < endTime && len(this.registry.RunningModels()) > 0 {
		time.Sleep(sleepTime)
	}

	// yeah, just kill them now
	for _, m := range this.registry.RunningModels() {
		m.SendSigKill()
	}
}
