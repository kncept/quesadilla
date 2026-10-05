package definitions

import (
	"fmt"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

var _ Backend = (*StandardBackend)(nil)

type Backend interface {
	Id() string
	Name() string
	Description() string
	ModelTypes() []string
	InstalledVersions() []string
	InstallableVersions() []string
	LatestVersion() string //empty string if indeterminable
	InstallVersion(version string) error
	RemoveVersion(version string) error
	Run(*modelDefinitions.Model) error
}

// BackendRunner runs a model using this backend.
type BackendRunner func(*modelDefinitions.Model) error

func NewStandardBackend(
	id string,
	name string,
	description string,
	modelTypes ...string,
) *StandardBackend {
	return &StandardBackend{
		id:          id,
		name:        name,
		description: description,
		runners:     make(map[string]runnerDefinitions.Runner),
		modelTypes:  modelTypes,
	}
}

type StandardBackend struct {
	id          string
	name        string
	description string
	Runner      BackendRunner
	runners     map[string]runnerDefinitions.Runner
	modelTypes  []string
}

// Run implements [Backend].
func (this *StandardBackend) Run(model *modelDefinitions.Model) error {
	if this.Runner == nil {
		return fmt.Errorf("no Runner Available for backend %s", this.id)
	}
	return this.Runner(model)
}

// runner returns the backend's runner: the component that provides its
// versions (what is installed, what is available, and how to install and
// remove them).
func (this *StandardBackend) runner() (runnerDefinitions.Runner, error) {
	for _, r := range this.runners {
		return r, nil
	}
	return nil, fmt.Errorf("no runners registered for backend %s", this.id)
}

// InstalledVersions implements [Backend].
func (this *StandardBackend) InstalledVersions() []string {
	r, err := this.runner()
	if err != nil {
		return []string{}
	}
	return r.InstalledVersions()
}

// InstallableVersions implements [Backend].
func (this *StandardBackend) InstallableVersions() []string {
	r, err := this.runner()
	if err != nil {
		return []string{}
	}
	return r.InstallableVersions()
}

// LatestVersion implements [Backend].
func (this *StandardBackend) LatestVersion() string {
	r, err := this.runner()
	if err != nil {
		return ""
	}
	return r.LatestVersion()
}

// InstallVersion implements [Backend].
func (this *StandardBackend) InstallVersion(version string) error {
	r, err := this.runner()
	if err != nil {
		return err
	}
	return r.InstallVersion(version)
}

// RemoveVersion implements [Backend].
func (this *StandardBackend) RemoveVersion(version string) error {
	r, err := this.runner()
	if err != nil {
		return err
	}
	return r.RemoveVersion(version)
}

// ModelTypes implements [Backend].
func (this *StandardBackend) ModelTypes() []string {
	return this.modelTypes
}

// Description implements [Backend].
func (this *StandardBackend) Description() string {
	return this.description
}

// Id implements [Backend].
func (this *StandardBackend) Id() string {
	return this.id
}

// Name implements [Backend].
func (this *StandardBackend) Name() string {
	return this.name
}

// Runners returns the runners registered with this backend: the components
// that provide its versions.
func (this *StandardBackend) Runners() []runnerDefinitions.Runner {
	values := make([]runnerDefinitions.Runner, 0, len(this.runners))
	for _, v := range this.runners {
		values = append(values, v)
	}
	return values
}

func (this *StandardBackend) RegisterRunner(runner runnerDefinitions.Runner) *StandardBackend {
	this.runners[runner.Id()] = runner
	return this
}
