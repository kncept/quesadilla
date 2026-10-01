package github

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"
)

type ReleaseDetails struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name,omitempty"` // nullable
	Prerelease  bool      `json:"prerelease"`
	Immutable   bool      `json:"immutable"`
	CreatedAt   time.Time `json:"created_at"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		ContentType        string `json:"content_type"`
		Size               int64  `json:"size"`
		BrowserDownloadUrl string `json:"browser_download_url"`
	} `json:"assets"`
}

// eg:
// {"message":"API rate limit exceeded for 127.0.0.1. (But here's the good news: Authenticated requests get a higher rate limit. Check out the documentation for more details.)","documentation_url":"https://docs.github.com/rest/overview/resources-in-the-rest-api#rate-limiting"}
type ApiErrorResponse struct {
	Message          string `json:"message"`
	DocumentationUrl string `json:"documentation_url"`
}

type GithubReleaseFinder struct {
	ClientCache *http.Client

	owner          string
	repository     string
	remotePageSize int
	remoteMaxPages int
	localPageSize  int
}

func NewGithubReleaseFinder(repoOwner string, repoName string) *GithubReleaseFinder {
	return &GithubReleaseFinder{
		owner:          repoOwner,
		repository:     repoName,
		remotePageSize: 100, // github default is 30
		remoteMaxPages: 10,
		localPageSize:  10,
	}
}

func (this *GithubReleaseFinder) getClient() *http.Client {
	if this.ClientCache == nil {
		this.ClientCache = &http.Client{}
	}
	return this.ClientCache
}

// start with 1 page NUMBER, not page INDEX
func (this *GithubReleaseFinder) paginatedReleaseDetails(pageNumber int) ([]ReleaseDetails, error) {
	client := this.getClient()
	getRequest, err := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=%d&page=%d", this.owner, this.repository, this.remotePageSize, pageNumber), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(getRequest)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		releaseDetails := make([]ReleaseDetails, 0)
		err = json.NewDecoder(resp.Body).Decode(&releaseDetails)
		if err != nil {
			return nil, err
		}
		return releaseDetails, nil
	} else if resp.StatusCode == 403 {
		apiErrorResponse := new(ApiErrorResponse)
		err = json.NewDecoder(resp.Body).Decode(&apiErrorResponse)
		if err != nil {
			return nil, err
		}

		if strings.HasPrefix(apiErrorResponse.Message, "API rate limit exceeded") {
			return nil, nil // RATE LIMITED - ignore error
		}
		panic(fmt.Sprintf("Expected Error response code and error: %d\n%s\n", resp.StatusCode, apiErrorResponse))

	} else {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}

		bodyString := string(bodyBytes)
		panic(fmt.Sprintf("Unknown response code and error: %d\n%s\n", resp.StatusCode, bodyString))
	}

}

func (this *GithubReleaseFinder) SmartListReleases(tagPrefix string) ([]ReleaseDetails, error) {
	versionTaggedReleases := make([]ReleaseDetails, 0)
	nightlyReleases := make([]ReleaseDetails, 0)

	pageNumber := 1
	for {
		releases, err := this.paginatedReleaseDetails(pageNumber)
		if err != nil {
			return nil, err
		}

		for _, release := range releases {
			if tagPrefix != "" && strings.HasPrefix(release.TagName, tagPrefix) {
				versionTaggedReleases = append(versionTaggedReleases, release)
			} else {
				nightlyReleases = append(nightlyReleases, release)
			}
		}

		if len(releases) == 0 || pageNumber >= this.remoteMaxPages {
			break
		}
		if len(versionTaggedReleases)+len(nightlyReleases) >= this.localPageSize {
			break
		}
		pageNumber++
	}

	SortByCreatedDate(versionTaggedReleases)
	if len(versionTaggedReleases) >= this.localPageSize {
		return versionTaggedReleases[:this.localPageSize], nil
	}
	SortByCreatedDate(nightlyReleases)
	// destructive add for return
	versionTaggedReleases = append(versionTaggedReleases, nightlyReleases...)
	if len(versionTaggedReleases) >= this.localPageSize {
		return versionTaggedReleases[:this.localPageSize], nil
	}
	return versionTaggedReleases, nil
}

func (this *GithubReleaseFinder) FindLatestRelease() (*ReleaseDetails, error) {
	client := this.getClient()
	getRequest, err := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", this.owner, this.repository), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(getRequest)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	release := new(ReleaseDetails)
	err = json.NewDecoder(resp.Body).Decode(&release)
	if err != nil {
		return nil, err
	}
	return release, nil
}

func (this *GithubReleaseFinder) ListAllReleases() ([]ReleaseDetails, error) {
	releaseDetails := make([]ReleaseDetails, 0)

	pageNumber := 1
	for {

		releases, err := this.paginatedReleaseDetails(pageNumber)
		if err != nil {
			return releaseDetails, err // return what progress we have
		}
		releaseDetails = append(releaseDetails, releases...)

		if len(releases) == 0 || pageNumber >= this.remoteMaxPages {
			break
		}

		pageNumber++
	}

	// fmt.Printf("Found: %v\n", len(releaseDetails))
	SortByCreatedDate(releaseDetails)

	return releaseDetails, nil
}

func (this *GithubReleaseFinder) ReleaseDetailsFromTag(tag string) (*ReleaseDetails, error) {
	client := this.getClient()
	getRequest, err := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", this.owner, this.repository, tag), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(getRequest)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	release := new(ReleaseDetails)
	err = json.NewDecoder(resp.Body).Decode(&release)
	if err != nil {
		return nil, err
	}
	return release, nil
}

func SortByCreatedDate(releaseDetails []ReleaseDetails) {
	slices.SortFunc(releaseDetails, func(a, b ReleaseDetails) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
}
