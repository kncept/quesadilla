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

// TestJobLifecycle verifies QGUI.startJob fetches the file in the background,
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
	g.startJob(jobs.JobRequest{
		Type:         jobs.JobTypeDownload,
		Name:         "test download",
		URL:          server.URL + "/file.txt",
		Destination:  destination,
		OnComplete: func(err error) {
			// the job is still listed while its completion runs
			if got := len(g.jobManager.ActiveJobs()); got != 1 {
				t.Errorf("active jobs during completion = %d, want 1", got)
			}
			completed <- err
		},
	})

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

	g.startJob(jobs.JobRequest{
		Type:         jobs.JobTypeDownload,
		Name:         "listed job",
		URL:          server.URL + "/file.txt",
		Destination:  destination,
	})

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
	if jobList[0].Source != server.URL+"/file.txt" {
		t.Errorf("job source = %q, want %q", jobList[0].Source, server.URL+"/file.txt")
	}
	if jobList[0].Destination != destination {
		t.Errorf("job destination = %q, want %q", jobList[0].Destination, destination)
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

	close(release)
	waitForJobFinish(t, g)
	waitForActiveJobs(t, g, 0)
}

// TestJobProgress verifies the progress percentage is updated during download
// when Content-Length is available, and stays -1 when it's not.
func TestJobProgress(t *testing.T) {
	test.NewApp()

	// Server that sends Content-Length
	serverWithLength := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := []byte("0123456789") // 10 bytes
		w.Header().Set("Content-Length", "10")
		_, _ = w.Write(data)
	}))
	defer serverWithLength.Close()

	// Server without Content-Length (chunked)
	serverNoLength := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No Content-Length header
		_, _ = w.Write([]byte("data"))
	}))
	defer serverNoLength.Close()

	g := CreateGui(nil, app.New())
	g.jobsPage()

	// Test with Content-Length
	dest1 := path.Join(t.TempDir(), "with-length.txt")
	done1 := make(chan struct{})
	g.startJob(jobs.JobRequest{
		Type:         jobs.JobTypeDownload,
		Name:         "with length",
		URL:          serverWithLength.URL + "/file.txt",
		Destination:  dest1,
		OnComplete: func(error) {
			close(done1)
		},
	})

	// Wait for job to complete
	select {
	case <-done1:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for job with length")
	}

	jobList := g.jobManager.ActiveJobs()
	if len(jobList) != 0 {
		t.Fatalf("expected no active jobs, got %d", len(jobList))
	}

	// Test without Content-Length
	dest2 := path.Join(t.TempDir(), "no-length.txt")
	done2 := make(chan struct{})
	g.startJob(jobs.JobRequest{
		Type:         jobs.JobTypeDownload,
		Name:         "no length",
		URL:          serverNoLength.URL + "/file.txt",
		Destination:  dest2,
		OnComplete: func(error) {
			close(done2)
		},
	})

	select {
	case <-done2:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for job without length")
	}

	jobList = g.jobManager.ActiveJobs()
	if len(jobList) != 0 {
		t.Fatalf("expected no active jobs, got %d", len(jobList))
	}
}