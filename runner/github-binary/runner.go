package githubbinary

import (
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
	"github.com/kncept/quesadilla/utils/github"
	"github.com/kncept/quesadilla/utils/qenv"
)

var _ runnerDefinitions.Runner = (*githubBinaryRunner)(nil)

func NewGithubBinaryRunnerFromUrl(providerId string, rawURL string, assetBinaryHandler AssetBinaryHandler) runnerDefinitions.Runner {
	owner, repository, err := extractOwnerAndRepositoryFromUrl(rawURL)
	if err != nil {
		panic(err)
	}
	return NewGithubBinaryRunnerFromRepoOwnerAndName(providerId, owner, repository, assetBinaryHandler)
}
func extractOwnerAndRepositoryFromUrl(rawURL string) (string, string, error) {
	// Regex matches both HTTPS and SSH git URLs
	re := regexp.MustCompile(`(?:https?://|git@)(?:[^:/]+)[/:]([^/]+)/([^/]+?)(?:\.git)?$`)
	matches := re.FindStringSubmatch(rawURL)

	if len(matches) < 3 {
		return "", "", fmt.Errorf("unable to parse URL: %s", rawURL)
	}

	return matches[1], matches[2], nil
}
func NewGithubBinaryRunnerFromRepoOwnerAndName(providerId string, repoOwner string, repoName string, assetBinaryHandler AssetBinaryHandler) runnerDefinitions.Runner {
	return &githubBinaryRunner{
		providerId:         providerId,
		repoOwner:          repoOwner,
		repoName:           repoName,
		assetBinaryHandler: assetBinaryHandler,
	}
}

// filter the FULL list of assets to a list of just what we need to download
type AssetBinaryHandler func(version string, nameToDownloadUrl map[string]string) error

type githubBinaryRunner struct {
	providerId         string
	repoOwner          string
	repoName           string
	assetBinaryHandler AssetBinaryHandler
}

// Run implements [definitions.Runner].
func (this *githubBinaryRunner) Run(*modelDefinitions.Model) error {

	panic("unimplemented")
}

// RemoveVersion implements [definitions.Runner].
func (this *githubBinaryRunner) RemoveVersion(version string) error {
	downloadsDirectory := qenv.QBinariesDirectory(this.providerId)
	return os.RemoveAll(path.Join(downloadsDirectory, version))
}

// LatestVersion implements [definitions.Runner].
func (this *githubBinaryRunner) LatestVersion() string {
	releaseFinder := github.NewGithubReleaseFinder(this.repoOwner, this.repoName)

	release, err := releaseFinder.FindLatestRelease()
	// release, err := releaseFinder.FindLatestReleaseWithTagPrefix("v")
	if err != nil {
		panic(err)
	}
	if release != nil {
		return release.TagName
	}
	return ""
}

// InstallVersion implements [definitions.Runner].
func (this *githubBinaryRunner) InstallVersion(version string) error {
	releaseFinder := github.NewGithubReleaseFinder(this.repoOwner, this.repoName)

	release, err := releaseFinder.ReleaseDetailsFromTag(version)
	if err != nil {
		panic(err)
	}
	// fmt.Printf("release:  %+v\n\n", release)
	assetNames := make([]string, 0)
	nameToDownloadUrl := make(map[string]string)
	for _, asset := range release.Assets {
		assetNames = append(assetNames, asset.Name)
		nameToDownloadUrl[asset.Name] = asset.BrowserDownloadUrl
	}

	return this.assetBinaryHandler(version, nameToDownloadUrl)
}

// InstallableVersions implements [definitions.Runner].
func (this *githubBinaryRunner) InstallableVersions() []string {
	releaseFinder := github.NewGithubReleaseFinder(this.repoOwner, this.repoName)

	releases, err := releaseFinder.SmartListReleases("v") //
	if err != nil {
		panic(err)
	}
	versions := make([]string, 0)
	for _, release := range releases {
		versions = append(versions, release.TagName)
	}

	return versions
}

// InstalledVersions implements [definitions.Runner].
func (this *githubBinaryRunner) InstalledVersions() []string {
	downloadsDirectory := qenv.QBinariesDirectory(this.providerId)
	versions := make([]string, 0)
	entries, err := os.ReadDir(downloadsDirectory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}
		}
		panic(err)
	}

	for _, entry := range entries {
		if entry.Name() != "temp" && entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}
	return versions
}

// ProviderId implements [definitions.Runner].
func (this *githubBinaryRunner) ProviderId() string {
	return this.providerId
}

// Description implements [definition.Runner].
func (this *githubBinaryRunner) Description() string {
	return "runs a downloaded binary"
}

// Id implements [definition.Runner].
func (this *githubBinaryRunner) Id() string {
	return "binary"
}

// Name implements [definition.Runner].
func (this *githubBinaryRunner) Name() string {
	return "Binary (from Github)"
}
