package qhttp

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
)

func DownloadFile(url string, destinationDirectory string, filename string) error {
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

	err = os.MkdirAll(destinationDirectory, 0755)
	if err != nil {
		return err
	}
	out, err := os.Create(path.Join(destinationDirectory, filename))
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
