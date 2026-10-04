package gui

type SystemAgnosticOperations interface {
	PreferencesOperations
	LocationOperations
}

type PreferencesOperations interface {
	SetPreference(dotTreeKey string, value string) error
	GetPreference(dotTreeKey string) (string, error)
}

type LocationOperations interface {
	LocationEnabled() bool
	GetLocation() (any, error) // []float64 or LAT, LON
}
