package definitions

import (
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

var _ Backend = (*StandardBackend)(nil)

type Backend interface {
	Id() string
	Name() string
	Description() string
	Runners() []runnerDefinitions.Runner
}

func NewStandardBackend(
	id string,
	name string,
	description string,
) *StandardBackend {
	return &StandardBackend{
		id:          id,
		name:        name,
		description: description,
		runners:     make(map[string]runnerDefinitions.Runner),
	}
}

type StandardBackend struct {
	id          string
	name        string
	description string
	runners     map[string]runnerDefinitions.Runner
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
