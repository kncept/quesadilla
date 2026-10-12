//go:build !cgo

package embeddedllama

import (
	"fmt"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// runtimeAvailable reports whether the embedded llama.cpp runtime is compiled
// into this binary. This build was made with cgo disabled (CGO_ENABLED=0), so
// it is not: the backend still appears on the Backends screen with its
// built-in version, but it cannot start a model.
const runtimeAvailable = false

// startEmbeddedLlamaServer fails: the embedded runtime is cgo, and this binary
// was built with CGO_ENABLED=0. Building it needs cgo enabled plus the
// llama.cpp static libraries (see docs/BACKENDS.md).
func startEmbeddedLlamaServer(m *modelDefinitions.Model) (runnerDefinitions.RunningModel, error) {
	return nil, fmt.Errorf("%s is unavailable in this build: it needs cgo, but this binary was built with CGO_ENABLED=0 - build with CGO_ENABLED=1 and the llama.cpp static libraries (see docs/BACKENDS.md)", providerId)
}
