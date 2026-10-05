// Package app is the single entry point into the application's data. It owns
// the backend and model repositories and hands them out, so the rest of the
// app shares one set of repositories rather than rebuilding them.
package app

import (
	"github.com/kncept/quesadilla/backend"
	"github.com/kncept/quesadilla/model/repository"
)

// QApp keeps references to the application's backend and model repositories.
// Both repositories are created (and scanned) once, when QApp is created.
type QApp struct {
	Backends *backend.Repository
	Models   *repository.Repository
}

// New creates a QApp, creating both repositories. Each repository scans for
// its contents as it is created.
func New() *QApp {
	return &QApp{
		Backends: backend.NewRepository(),
		Models:   repository.NewRepository(),
	}
}
