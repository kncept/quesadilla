package qhttp

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
)

// ProgressFunc is called with the current download progress.
// current is the number of bytes downloaded so far.
// total is the total file size (or -1 if unknown).
type ProgressFunc func(current int64, total int64)

func DownloadFile(url string, destinationDirectory string, filename string) error {
	return DownloadFileWithProgress(url, destinationDirectory, filename, nil)
}

func DownloadFileWithProgress(url string, destinationDirectory string, filename string, progress ProgressFunc) error {
	client := &http.Client{}

	getRequest, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(getRequest)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d downloading %s", resp.StatusCode, url)
	}

	totalSize := resp.ContentLength
	if totalSize <= 0 {
		totalSize = -1
	}

	err = os.MkdirAll(destinationDirectory, 0755)
	if err != nil {
		return err
	}
	out, err := os.Create(path.Join(destinationDirectory, filename))
	if err != nil {
		return err
	}
	defer out.Close()

	if progress != nil {
		// Wrap the reader to track progress
		reader := &progressReader{
			reader:  resp.Body,
			total:   totalSize,
			current: 0,
			progress: progress,
		}
		_, err = io.Copy(out, reader)
		// Call progress one final time with complete status
		if progress != nil && totalSize > 0 {
			progress(totalSize, totalSize)
		}
	} else {
		_, err = io.Copy(out, resp.Body)
	}
	return err
}

type progressReader struct {
	reader   io.Reader
	total    int64
	current  int64
	progress ProgressFunc
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.reader.Read(p)
	pr.current += int64(n)
	if pr.progress != nil {
		pr.progress(pr.current, pr.total)
	}
	return n, err
}
