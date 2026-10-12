package onnx

import (
	"fmt"

	"github.com/kncept/quesadilla/backend/definitions"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

var _ definitions.Backend = (*onnxBackend)(nil)

const providerId string = "onnx"

// builtInVersion labels the onnx runtime. Unlike the llama backend, onnx is
// a pure-Go inference engine compiled into the quesadilla binary (via
// github.com/oramasearch/onnx-go, executed with Gorgonia), so there is
// exactly one version of it: the one this binary was built with.
const builtInVersion = "built-in"

// OnnxBackend builds the onnx backend: a pure-Go ONNX inference engine that
// serves models over an HTTP inference API, in-process.
func OnnxBackend() definitions.Backend {
	return &onnxBackend{}
}

type onnxBackend struct{}

// Description implements [definitions.Backend].
func (this *onnxBackend) Description() string {
	return "Pure-Go ONNX inference engine, built into the app (github.com/oramasearch/onnx-go)"
}

// Id implements [definitions.Backend].
func (this *onnxBackend) Id() string {
	return providerId
}

// InstallVersion implements [definitions.Backend]. The runtime is compiled
// into the binary, so there is nothing to install.
func (this *onnxBackend) InstallVersion(version string) error {
	return fmt.Errorf("%s is built into the quesadilla binary - there is nothing to install", providerId)
}

// InstallableVersions implements [definitions.Backend].
func (this *onnxBackend) InstallableVersions() []string {
	return nil
}

// InstalledVersions implements [definitions.Backend].
func (this *onnxBackend) InstalledVersions() []string {
	return []string{builtInVersion}
}

// LatestVersion implements [definitions.Backend].
func (this *onnxBackend) LatestVersion() string {
	return builtInVersion
}

// ModelTypes implements [definitions.Backend].
func (this *onnxBackend) ModelTypes() []string {
	return []string{string(modelDefinitions.ModelTypeONNX)}
}

// RemoveVersion implements [definitions.Backend]. The runtime is compiled
// into the binary, so there is nothing to remove.
func (this *onnxBackend) RemoveVersion(version string) error {
	return fmt.Errorf("%s is built into the quesadilla binary - there is nothing to remove", providerId)
}

// Start implements [definitions.Backend].
func (this *onnxBackend) Start(m *modelDefinitions.Model) (runnerDefinitions.RunningModel, error) {
	return startOnnxServer(m)
}
