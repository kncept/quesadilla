package llama

import (
	"fmt"

	"github.com/kncept/quesadilla/backend/definitions"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	githubbinary "github.com/kncept/quesadilla/runner/github-binary"
)

const providerId string = "llama"

// llamaCppRepoUrl is where the llama.cpp backend downloads its binaries from.
const llamaCppRepoUrl = "https://github.com/ggml-org/llama.cpp"

// LlamaBackend builds the llama.cpp backend: a github binary runner that
// provides its versions, plus the logic to serve models with them.
func LlamaBackend() definitions.Backend {
	binaryRunner := githubbinary.NewGithubBinaryRunnerFromUrl(
		providerId,
		llamaCppRepoUrl,
		installLlamaAssets,
	)

	llamaBackend := definitions.NewStandardBackend(
		providerId, "Llama.cpp",
		"See https://llama.app/ for details",
		"gguf",
	)
	llamaBackend.RegisterRunner(binaryRunner)

	llamaBackend.Runner = func(m *modelDefinitions.Model) error {
		versions := binaryRunner.InstalledVersions()
		if len(versions) == 0 {
			return fmt.Errorf("no version of %s installed - install one first", llamaBackend.Name())
		}
		return runLlamaServer(m, versions[0])
	}
	return llamaBackend
}
