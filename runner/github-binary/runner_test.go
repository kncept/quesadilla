package githubbinary

import "testing"

const repoOwner = "kncept"
const repoName = "quesadilla"

func TestUrlParsing(t *testing.T) {
	// http page
	verifyIsLlamaCpp("https://github.com/kncept/quesadilla", t)

	// http clone
	verifyIsLlamaCpp("https://github.com/kncept/quesadilla.git", t)

	// git clone
	verifyIsLlamaCpp("git@github.com:kncept/quesadilla.git", t)

	//ssh clone
	verifyIsLlamaCpp("ssh://git@github.com/kncept/quesadilla.git", t)

}
func verifyIsLlamaCpp(rawUrl string, t *testing.T) {
	owner, repo, err := extractOwnerAndRepositoryFromUrl(rawUrl)
	if err != nil {
		t.Fatalf("Unexpected error parsing url: %v", err)
	}
	if repoOwner != owner {
		t.Errorf("Invalid Owner: Expected %v but got %v", repoOwner, owner)
	}
	if repoName != repo {
		t.Errorf("Invalid Repository: Expected %v byt git %v", repoName, repo)
	}
}
