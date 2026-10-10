package qhttp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"
)

// TestDownloadFile verifies a successful download writes the response body to
// the named file in the destination directory.
func TestDownloadFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()

	dest := t.TempDir()
	if err := DownloadFile(server.URL+"/f.txt", dest, "f.txt"); err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	data, err := os.ReadFile(path.Join(dest, "f.txt"))
	if err != nil {
		t.Fatalf("reading the downloaded file: %v", err)
	}
	if string(data) != "payload" {
		t.Errorf("contents = %q, want payload", data)
	}
}

// TestDownloadFileNonOKStatus verifies a non-200 response is reported as an
// error and writes no file, so an error page (eg a 404 for a deleted model)
// is never stored as if it were the downloaded content.
func TestDownloadFileNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer server.Close()

	dest := t.TempDir()
	err := DownloadFile(server.URL+"/f.txt", dest, "f.txt")
	if err == nil {
		t.Fatal("expected an error for a non-200 status")
	}
	if _, err := os.Stat(path.Join(dest, "f.txt")); !os.IsNotExist(err) {
		t.Error("expected no file to be written for a non-200 status")
	}
}
