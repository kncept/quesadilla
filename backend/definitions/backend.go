package definitions

import (
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

type Backend interface {
	Id() string
	Description() string
	ModelTypes() []string
	InstalledVersions() []string
	InstallableVersions() []string
	LatestVersion() string //empty string if indeterminable
	InstallVersion(version string) error
	RemoveVersion(version string) error
	Start(*modelDefinitions.Model) (runnerDefinitions.RunningModel, error)
}
