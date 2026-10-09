package llama

import (
	"fmt"

	"github.com/kncept/quesadilla/backend/definitions"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
	githubbinary "github.com/kncept/quesadilla/runner/github-binary"
)

var _ definitions.Backend = (*llamaBackend)(nil)

const providerId string = "llama-binary"

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

	return &llamaBackend{
		GithubBinaryDownloader: binaryRunner,
	}
}

type llamaBackend struct {
	GithubBinaryDownloader *githubbinary.GithubBinaryDownloader
}

// Description implements [definitions.Backend].
func (this *llamaBackend) Description() string {
	return "See https://llama.app/ for details"
}

// Id implements [definitions.Backend].
func (this *llamaBackend) Id() string {
	return providerId
}

// InstallVersion implements [definitions.Backend].
func (this *llamaBackend) InstallVersion(version string) error {
	return this.GithubBinaryDownloader.InstallVersion(version)
}

// InstallableVersions implements [definitions.Backend].
func (this *llamaBackend) InstallableVersions() []string {
	return this.GithubBinaryDownloader.InstallableVersions()
}

// InstalledVersions implements [definitions.Backend].
func (this *llamaBackend) InstalledVersions() []string {
	return this.GithubBinaryDownloader.InstalledVersions()
}

// LatestVersion implements [definitions.Backend].
func (this *llamaBackend) LatestVersion() string {
	return this.GithubBinaryDownloader.LatestVersion()
}

// ModelTypes implements [definitions.Backend].
func (l *llamaBackend) ModelTypes() []string {
	return []string{"gguf", "hf"}
}

// RemoveVersion implements [definitions.Backend].
func (this *llamaBackend) RemoveVersion(version string) error {
	return this.GithubBinaryDownloader.RemoveVersion(version)
}

// Start implements [definitions.Backend].
func (this *llamaBackend) Start(m *modelDefinitions.Model) (runnerDefinitions.RunningModel, error) {
	versions := this.GithubBinaryDownloader.InstalledVersions()
	if len(versions) == 0 {
		return nil, fmt.Errorf("no version of %s installed - install one first", providerId)
	}
	return startLlamaServer(m, versions[0])
}
