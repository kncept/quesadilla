package gui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/kncept/quesadilla/app"
	"github.com/kncept/quesadilla/gui/jobs"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	"github.com/kncept/quesadilla/utils/qhttp"
)

// waitForActiveJobs waits until the given number of jobs are listed as active.
func waitForActiveJobs(t *testing.T, g *QGUI, want int) []jobs.Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		jobList := g.jobManager.ActiveJobs()
		if len(jobList) == want {
			return jobList
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d active jobs, have %d", want, len(g.jobManager.ActiveJobs()))
	return nil
}

// TestJobLifecycle verifies a started job fetches the file in the background,
// lists it while it runs, calls the completion function with the result, and
// then removes it from the active list.
func TestJobLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("downloaded-bytes"))
	}))
	defer server.Close()

	g := CreateGui(nil, app.New())
	destination := path.Join(t.TempDir(), "file.txt")

	completed := make(chan error, 1)
	err := g.jobManager.StartJob(
		"test download",
		jobs.JobTypeDownload,
		"download a test file",
		func() error {
			return qhttp.DownloadFileWithProgress(server.URL+"/file.txt", path.Dir(destination), path.Base(destination), nil)
		},
		func(err error) {
			// the job is still listed while its completion runs
			if got := len(g.jobManager.ActiveJobs()); got != 1 {
				t.Errorf("active jobs during completion = %d, want 1", got)
			}
			completed <- err
		},
		nil,
	)
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}

	select {
	case err := <-completed:
		if err != nil {
			t.Fatalf("job: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the job to finish")
	}

	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("reading the downloaded file: %v", err)
	}
	if string(data) != "downloaded-bytes" {
		t.Errorf("file contents = %q, want downloaded-bytes", data)
	}

	if jobList := waitForActiveJobs(t, g, 0); len(jobList) != 0 {
		t.Fatalf("active jobs after completion = %v, want none", jobList)
	}
}

// TestJobListedWhileActive verifies a job is on the active list (and the Jobs
// screen) until it finishes.
func TestJobListedWhileActive(t *testing.T) {
	test.NewApp()

	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("done"))
	}))
	defer server.Close()

	g := CreateGui(nil, app.New())
	g.jobsPage()
	destination := path.Join(t.TempDir(), "file.txt")

	err := g.jobManager.StartJob(
		"listed job",
		jobs.JobTypeDownload,
		"a listed job",
		func() error {
			return qhttp.DownloadFileWithProgress(server.URL+"/file.txt", path.Dir(destination), path.Base(destination), nil)
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}

	select {
	case <-started:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("timed out waiting for the job to start")
	}

	jobList := waitForActiveJobs(t, g, 1)
	if jobList[0].Name != "listed job" {
		t.Errorf("job name = %q, want listed job", jobList[0].Name)
	}
	if jobList[0].Type != jobs.JobTypeDownload {
		t.Errorf("job type = %q, want %q", jobList[0].Type, jobs.JobTypeDownload)
	}
	if jobList[0].Description != "a listed job" {
		t.Errorf("job description = %q, want a listed job", jobList[0].Description)
	}

	close(release)
	waitForActiveJobs(t, g, 0)
}

// TestInstallModelListedAsJob verifies model downloads started from Model Search
// are listed as jobs, not only single-file downloads.
func TestInstallModelListedAsJob(t *testing.T) {
	test.NewApp()

	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("gguf-bytes"))
	}))
	defer server.Close()

	g := CreateGui(nil, newTestQApp(t))
	g.modelSearchPage()

	remote := &modelDefinitions.RemoteModel{
		ModelFamily: "org",
		ModelName:   "a",
		ModelType:   "gguf",
		ModelSource: "ZTest",
		Variants: []modelDefinitions.ModelVariant{
			{FileName: "a.gguf", Location: server.URL + "/a.gguf"},
		},
	}
	g.QApp.RemoteModels.Register(&fakeSearchScanner{
		name: "ZTest",
		get:  func(string) *modelDefinitions.RemoteModel { return remote },
	})

	g.installModel(*remote, "", installDownload)

	select {
	case <-started:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("timed out waiting for the model download to start")
	}

	jobList := waitForActiveJobs(t, g, 1)
	if jobList[0].Name != "Download org/a" {
		t.Errorf("job name = %q, want Download org/a", jobList[0].Name)
	}
	if jobList[0].Type != jobs.JobTypeDownload {
		t.Errorf("job type = %q, want %q", jobList[0].Type, jobs.JobTypeDownload)
	}
	if jobList[0].Description != "Download org/a" {
		t.Errorf("job description = %q, want Download org/a", jobList[0].Description)
	}

	close(release)
	waitForJobFinish(t, g)
	waitForActiveJobs(t, g, 0)
}

// TestProgressTracker verifies the byte-based progress callbacks are converted
// to a percentage, with an unknown total reported as -1.
func TestProgressTracker(t *testing.T) {
	p := &progressTracker{}
	if got := p.percent(); got != -1 {
		t.Errorf("percent with no progress = %d, want -1", got)
	}
	p.update(5, 10)
	if got := p.percent(); got != 50 {
		t.Errorf("percent = %d, want 50", got)
	}
	p.update(10, 10)
	if got := p.percent(); got != 100 {
		t.Errorf("percent = %d, want 100", got)
	}
}
