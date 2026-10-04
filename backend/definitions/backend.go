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
	Runners() []runnerDefinitions.Runner
	ModelTypes() []string
	Run(*modelDefinitions.Model) error
}

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
	if len(this.Runners()) == 0 {
		return fmt.Errorf("No Runners Available for backend %s", this.id)
	}
	return this.Runner(model)
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

// Runners implements [Backend].
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
