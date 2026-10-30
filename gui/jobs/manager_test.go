package jobs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestJobManagerStartDownloadJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("downloaded-bytes"))
	}))
	defer server.Close()

	m := NewJobManager()
	destination := "/tmp/test-download-file.txt"

	completed := make(chan error, 1)
	m.StartDownloadJob(DownloadParams{
		Name:        "test download",
		URL:         server.URL + "/file.txt",
		Destination: destination,
		OnComplete: func(err error) {
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

	// Check file was downloaded
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("reading the downloaded file: %v", err)
	}
	if string(data) != "downloaded-bytes" {
		t.Errorf("file contents = %q, want downloaded-bytes", data)
	}

	// Wait for job to be removed
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(m.ActiveJobs()) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job not removed from active list: %v", m.ActiveJobs())
}
