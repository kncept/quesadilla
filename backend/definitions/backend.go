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

// ManageableBackend is a Backend whose runtime versions are downloaded and
// installed at runtime, rather than compiled into the application. Backends
// that opt in get a Manage window in the GUI, where their installed and
// installable versions can be listed, installed and removed.
//
// The method is a marker: it distinguishes backends with something to manage
// from those that are built in and have no versions to install.
type ManageableBackend interface {
	Backend
	Manageable()
}
