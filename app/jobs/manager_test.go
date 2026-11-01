package jobs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kncept/quesadilla/utils/qhttp"
)

// waitForJobCount waits until the active job list holds want jobs.
func waitForJobCount(t *testing.T, m *JobManager, want int) []Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if jobs := m.ActiveJobs(); len(jobs) == want {
			return jobs
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d active jobs, have %d", want, len(m.ActiveJobs()))
	return nil
}

func TestJobManagerStartJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("downloaded-bytes"))
	}))
	defer server.Close()

	m := NewJobManager()
	destination := path.Join(t.TempDir(), "file.txt")

	completed := make(chan error, 1)
	err := m.StartJob(
		"test download",
		JobTypeDownload,
		"download a test file",
		func() error {
			return qhttp.DownloadFileWithProgress(server.URL+"/file.txt", path.Dir(destination), path.Base(destination), nil)
		},
		func(err error) {
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

	waitForJobCount(t, m, 0)
}

func TestJobManagerStartJobDetails(t *testing.T) {
	m := NewJobManager()
	started := make(chan struct{})
	release := make(chan struct{})
	err := m.StartJob(
		"named job",
		JobTypeDownload,
		"the description",
		func() error {
			close(started)
			<-release
			return nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("StartJob: %v", err)
	}
	<-started

	jobs := waitForJobCount(t, m, 1)
	if jobs[0].Name != "named job" {
		t.Errorf("name = %q, want named job", jobs[0].Name)
	}
	if jobs[0].Type != JobTypeDownload {
		t.Errorf("type = %q, want %q", jobs[0].Type, JobTypeDownload)
	}
	if jobs[0].Description != "the description" {
		t.Errorf("description = %q, want the description", jobs[0].Description)
	}
	// With a nil progress function the job reports unknown progress.
	if jobs[0].Progress != -1 {
		t.Errorf("progress = %d, want -1", jobs[0].Progress)
	}

	close(release)
	waitForJobCount(t, m, 0)
}

func TestJobManagerStartJobDuplicate(t *testing.T) {
	m := NewJobManager()
	started := make(chan struct{})
	release := make(chan struct{})

	if err := m.StartJob("dup", JobTypeDownload, "first", func() error {
		close(started)
		<-release
		return nil
	}, nil, nil); err != nil {
		t.Fatalf("first StartJob: %v", err)
	}
	<-started

	ran := make(chan struct{}, 1)
	err := m.StartJob("dup", JobTypeDownload, "second", func() error {
		ran <- struct{}{}
		return nil
	}, nil, nil)
	if err == nil {
		t.Fatal("expected a duplicate-job error")
	}

	select {
	case <-ran:
		t.Fatal("the duplicate job must not run")
	case <-time.After(100 * time.Millisecond):
	}

	// The same name with a different type is a different job.
	if err := m.StartJob("dup", JobTypeGeneric, "other type", func() error { return nil }, nil, nil); err != nil {
		t.Fatalf("different-type StartJob: %v", err)
	}

	close(release)
	waitForJobCount(t, m, 0)
}

func TestJobManagerStartJobProgress(t *testing.T) {
	m := NewJobManager()
	started := make(chan struct{})
	release := make(chan struct{})
	var progress atomic.Int64
	progress.Store(42)

	if err := m.StartJob("progress", JobTypeDownload, "poll progress", func() error {
		close(started)
		<-release
		return nil
	}, nil, func() int { return int(progress.Load()) }); err != nil {
		t.Fatalf("StartJob: %v", err)
	}
	<-started

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		jobs := m.ActiveJobs()
		if len(jobs) == 1 && jobs[0].Progress == 42 {
			close(release)
			waitForJobCount(t, m, 0)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
	t.Fatalf("progress was not polled: %v", m.ActiveJobs())
}
