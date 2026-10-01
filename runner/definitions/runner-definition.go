package definitions

import modelDefinitions "github.com/kncept/quesadilla/model/definitions"

type Runner interface {
	InstalledVersions() []string
	InstallableVersions() []string
	LatestVersion() string //empty string if indeterminable
	InstallVersion(version string) error
	RemoveVersion(version string) error

	ProviderId() string

	Id() string
	Name() string
	Description() string

	Run(*modelDefinitions.Model) error
}

func GetRunner(runners []Runner, id string) Runner {
	for _, r := range runners {
		if r.Id() == id {
			return r
		}
	}
	return nil
}
