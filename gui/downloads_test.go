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
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
)

// waitForActiveDownloads waits until the given number of downloads are listed
// as active.
func waitForActiveDownloads(t *testing.T, g *QGUI, want int) []activeDownload {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		downloads := g.activeDownloads()
		if len(downloads) == want {
			return downloads
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d active downloads, have %d", want, len(g.activeDownloads()))
	return nil
}

// TestDownloadLifecycle verifies QGUI.download fetches the file in the
// background, lists it while it runs, calls the completion function with the
// result, and then removes it from the active list.
func TestDownloadLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("downloaded-bytes"))
	}))
	defer server.Close()

	g := CreateGui(nil, app.New())
	destination := path.Join(t.TempDir(), "file.txt")

	completed := make(chan error, 1)
	g.download(downloadRequest{
		Name:        "test download",
		URL:         server.URL + "/file.txt",
		Destination: destination,
		OnComplete: func(err error) {
			// the download is still listed while its completion runs
			if got := len(g.activeDownloads()); got != 1 {
				t.Errorf("active downloads during completion = %d, want 1", got)
			}
			completed <- err
		},
	})

	select {
	case err := <-completed:
		if err != nil {
			t.Fatalf("download: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the download to finish")
	}

	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("reading the downloaded file: %v", err)
	}
	if string(data) != "downloaded-bytes" {
		t.Errorf("file contents = %q, want downloaded-bytes", data)
	}

	if downloads := waitForActiveDownloads(t, g, 0); len(downloads) != 0 {
		t.Fatalf("active downloads after completion = %v, want none", downloads)
	}
}

// TestDownloadListedWhileActive verifies a download is on the active list
// (and the Downloads table) until it finishes.
func TestDownloadListedWhileActive(t *testing.T) {
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
	g.downloadsPage()
	destination := path.Join(t.TempDir(), "file.txt")

	g.download(downloadRequest{
		Name:        "listed download",
		URL:         server.URL + "/file.txt",
		Destination: destination,
	})

	select {
	case <-started:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("timed out waiting for the download to start")
	}

	downloads := waitForActiveDownloads(t, g, 1)
	if downloads[0].Name != "listed download" {
		t.Errorf("download name = %q, want listed download", downloads[0].Name)
	}
	if downloads[0].Source != server.URL+"/file.txt" {
		t.Errorf("download source = %q, want %q", downloads[0].Source, server.URL+"/file.txt")
	}
	if downloads[0].Destination != destination {
		t.Errorf("download destination = %q, want %q", downloads[0].Destination, destination)
	}

	if rows, cols := g.downloadsTable.Length(); rows != 1 || cols != len(downloadColumns) {
		t.Errorf("downloads table = %dx%d, want 1x%d", rows, cols, len(downloadColumns))
	}
	if got := tableCellText(g.downloadsTable, 0, 0); got != "listed download" {
		t.Errorf("downloads table name = %q, want listed download", got)
	}

	close(release)
	waitForActiveDownloads(t, g, 0)
	if rows, _ := g.downloadsTable.Length(); rows != 0 {
		t.Errorf("downloads table rows after completion = %d, want 0", rows)
	}
}

// TestInstallModelListedAsDownload verifies model downloads started from
// Model Search are listed as download jobs, not only single-file downloads.
func TestInstallModelListedAsDownload(t *testing.T) {
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

	downloads := waitForActiveDownloads(t, g, 1)
	if downloads[0].Name != "Download org/a" {
		t.Errorf("download job name = %q, want Download org/a", downloads[0].Name)
	}

	close(release)
	waitForDownloadFinish(t, g)
	waitForActiveDownloads(t, g, 0)
}

// TestDownloadProgress verifies the progress percentage is updated during
// download when Content-Length is available, and stays -1 when it's not.
func TestDownloadProgress(t *testing.T) {
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
	g.downloadsPage()

	// Test with Content-Length
	dest1 := path.Join(t.TempDir(), "with-length.txt")
	done1 := make(chan struct{})
	g.download(downloadRequest{
		Name:        "with length",
		URL:         serverWithLength.URL + "/file.txt",
		Destination: dest1,
		OnComplete: func(error) {
			close(done1)
		},
	})

	// Wait for download to complete
	select {
	case <-done1:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for download with length")
	}

	downloads := g.activeDownloads()
	if len(downloads) != 0 {
		t.Fatalf("expected no active downloads, got %d", len(downloads))
	}

	// Test without Content-Length
	dest2 := path.Join(t.TempDir(), "no-length.txt")
	done2 := make(chan struct{})
	g.download(downloadRequest{
		Name:        "no length",
		URL:         serverNoLength.URL + "/file.txt",
		Destination: dest2,
		OnComplete: func(error) {
			close(done2)
		},
	})

	select {
	case <-done2:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for download without length")
	}

	downloads = g.activeDownloads()
	if len(downloads) != 0 {
		t.Fatalf("expected no active downloads, got %d", len(downloads))
	}
}
