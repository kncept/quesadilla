package gogguf

import (
	"fmt"

	"github.com/kncept/quesadilla/backend/definitions"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

var _ definitions.Backend = (*goggufBackend)(nil)

const providerId string = "gogguf"

// builtInVersion labels the gogguf runtime. Unlike the llama backend, gogguf
// is a pure-Go library compiled into the quesadilla binary (from the
// feat/arm64-darwin-support branch of github.com/nkrul/gogguf), so there is
// exactly one version of it: the one this binary was built with.
const builtInVersion = "built-in"

// GoggufBackend builds the gogguf backend: a pure-Go GGUF inference engine
// that serves models over an OpenAI-compatible HTTP API, in-process.
func GoggufBackend() definitions.Backend {
	return &goggufBackend{}
}

type goggufBackend struct{}

// Description implements [definitions.Backend].
func (this *goggufBackend) Description() string {
	return "Pure-Go GGUF inference engine, built into the app (github.com/nkrul/gogguf)"
}

// Id implements [definitions.Backend].
func (this *goggufBackend) Id() string {
	return providerId
}

// InstallVersion implements [definitions.Backend]. The runtime is compiled
// into the binary, so there is nothing to install.
func (this *goggufBackend) InstallVersion(version string) error {
	return fmt.Errorf("%s is built into the quesadilla binary - there is nothing to install", providerId)
}

// InstallableVersions implements [definitions.Backend].
func (this *goggufBackend) InstallableVersions() []string {
	return nil
}

// InstalledVersions implements [definitions.Backend].
func (this *goggufBackend) InstalledVersions() []string {
	return []string{builtInVersion}
}

// LatestVersion implements [definitions.Backend].
func (this *goggufBackend) LatestVersion() string {
	return builtInVersion
}

// ModelTypes implements [definitions.Backend].
func (this *goggufBackend) ModelTypes() []string {
	return []string{"gguf"}
}

// RemoveVersion implements [definitions.Backend]. The runtime is compiled
// into the binary, so there is nothing to remove.
func (this *goggufBackend) RemoveVersion(version string) error {
	return fmt.Errorf("%s is built into the quesadilla binary - there is nothing to remove", providerId)
}

// Start implements [definitions.Backend].
func (this *goggufBackend) Start(m *modelDefinitions.Model) (runnerDefinitions.RunningModel, error) {
	return startGoggufServer(m)
}
