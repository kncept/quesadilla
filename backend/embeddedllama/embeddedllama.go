package embeddedllama

import (
	"fmt"

	"github.com/kncept/quesadilla/backend/definitions"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

var _ definitions.Backend = (*embeddedLlamaBackend)(nil)

const providerId string = "embedded-llama"

// builtInVersion labels the embedded llama.cpp runtime. Like gogguf and onnx,
// embedded-llama is compiled into the quesadilla binary rather than downloaded
// as a separate runtime, so there is exactly one version of it: the one this
// binary was built with. Unlike the pure-Go backends it is cgo, backed by
// github.com/tcpipuk/llama-go, and so is only present in cgo builds (see
// docs/BACKENDS.md).
const builtInVersion = "built-in"

// EmbeddedLlamaBackend builds the embedded-llama backend: llama.cpp linked
// directly into the app through the llama-go bindings, serving models over an
// OpenAI-compatible HTTP API, in-process.
func EmbeddedLlamaBackend() definitions.Backend {
	return &embeddedLlamaBackend{}
}

type embeddedLlamaBackend struct{}

// Description implements [definitions.Backend].
func (this *embeddedLlamaBackend) Description() string {
	if runtimeAvailable {
		return "llama.cpp compiled into the app (github.com/tcpipuk/llama-go), served in-process"
	}
	return "llama.cpp compiled into the app (github.com/tcpipuk/llama-go) - not built in this binary"
}

// Id implements [definitions.Backend].
func (this *embeddedLlamaBackend) Id() string {
	return providerId
}

// InstallVersion implements [definitions.Backend]. The runtime is compiled
// into the binary, so there is nothing to install.
func (this *embeddedLlamaBackend) InstallVersion(version string) error {
	return fmt.Errorf("%s is built into the quesadilla binary - there is nothing to install", providerId)
}

// InstallableVersions implements [definitions.Backend].
func (this *embeddedLlamaBackend) InstallableVersions() []string {
	return nil
}

// InstalledVersions implements [definitions.Backend]. The runtime is compiled
// into the binary rather than installed, so like gogguf and onnx it always
// reports its single built-in version - even in a build made without the
// embedded runtime, where Start is what reports the runtime as unavailable.
func (this *embeddedLlamaBackend) InstalledVersions() []string {
	return []string{builtInVersion}
}

// LatestVersion implements [definitions.Backend].
func (this *embeddedLlamaBackend) LatestVersion() string {
	return builtInVersion
}

// ModelTypes implements [definitions.Backend].
func (this *embeddedLlamaBackend) ModelTypes() []string {
	return []string{string(modelDefinitions.ModelTypeGGUF)}
}

// RemoveVersion implements [definitions.Backend]. The runtime is compiled
// into the binary, so there is nothing to remove.
func (this *embeddedLlamaBackend) RemoveVersion(version string) error {
	return fmt.Errorf("%s is built into the quesadilla binary - there is nothing to remove", providerId)
}

// Start implements [definitions.Backend].
func (this *embeddedLlamaBackend) Start(m *modelDefinitions.Model) (runnerDefinitions.RunningModel, error) {
	return startEmbeddedLlamaServer(m)
}
