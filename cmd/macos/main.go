package main

import (
	"github.com/kncept/quesadilla/app"
	"github.com/kncept/quesadilla/gui"
)

func main() {

	g := gui.CreateGui(SystemAgnosticOperations(), app.New())
	g.Start()
}

func SystemAgnosticOperations() gui.SystemAgnosticOperations {
	return &macosOperations{}
}

type macosOperations struct{}

// GetLocation implements [gui.SystemAgnosticOperations].
func (m *macosOperations) GetLocation() (any, error) {
	panic("unimplemented")
}

// GetPreference implements [gui.SystemAgnosticOperations].
func (m *macosOperations) GetPreference(dotTreeKey string) (string, error) {
	panic("unimplemented")
}

// LocationEnabled implements [gui.SystemAgnosticOperations].
func (m *macosOperations) LocationEnabled() bool {
	panic("unimplemented")
}

// SetPreference implements [gui.SystemAgnosticOperations].
func (m *macosOperations) SetPreference(dotTreeKey string, value string) error {
	panic("unimplemented")
}
