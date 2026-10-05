package llama

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"

	"github.com/kncept/quesadilla/utils/compress"
	"github.com/kncept/quesadilla/utils/qenv"
	"github.com/kncept/quesadilla/utils/qhttp"
)

// installLlamaAssets is the binary runner's asset handler for the llama.cpp
// backend: it selects the release assets for the current platform, downloads
// and unpacks them, and tidies up the result.
func installLlamaAssets(version string, nameToDownloadUrl map[string]string) error {
	assetsToDownload := make([]string, 0)

	matchers := make([]func(string) bool, 0)
	matchers = append(matchers, func(name string) bool {
		return strings.Contains(name, runtime.GOARCH)
	})
	matchers = append(matchers, func(name string) bool {
		// handle darwin/macos definition
		if runtime.GOOS == "darwin" {
			return strings.Contains(name, "darwin") || strings.Contains(name, "macos")
		}
		return strings.Contains(name, runtime.GOOS)
	})

	for name, _ := range nameToDownloadUrl {
		match := true
		for _, matcher := range matchers {
			match = match && matcher(name)
		}
		if match {
			assetsToDownload = append(assetsToDownload, name)
		}
	}

	downloadsDirectory := qenv.QBinariesDirectory(providerId)
	if len(assetsToDownload) == 0 {
		panic("Unable to work out what assets to download")
	}
	for _, assetName := range assetsToDownload {
		browserDownloadUrl := nameToDownloadUrl[assetName]

		downloadDirectory := path.Join(downloadsDirectory, version)
		err := qhttp.DownloadFile(browserDownloadUrl, downloadDirectory, assetName)
		if err != nil {
			return err
		}

		if strings.HasSuffix(assetName, ".tar") {
			err = compress.Untar(path.Join(downloadDirectory, assetName), downloadDirectory)
			if err != nil {
				return err
			}
		} else if strings.HasSuffix(assetName, ".tar.gz") || strings.HasSuffix(assetName, ".tgz") {
			err = compress.UntarGz(path.Join(downloadDirectory, assetName), downloadDirectory)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("unknown file format to handle")
		}
		err = os.Remove(path.Join(downloadDirectory, assetName))
		if err != nil {
			return err
		}

		// do linking here?

		binDir := path.Join(downloadDirectory, fmt.Sprintf("llama-%s", version))
		dirEntries, err := os.ReadDir(binDir)
		if err != nil {
			return err
		}

		if runtime.GOOS == "darwin" {
			for _, dirEntry := range dirEntries {
				name := dirEntry.Name()
				if shouldSymlinkFile(name) {
					newname := newFileName(name)
					if dirEntry.Name() != newname {
						err = os.Symlink(name, path.Join(binDir, newname))
						if err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}

// shouldSymlinkFile reports whether the extracted dylib needs a symlink
// alongside it so llama-server can locate it.
func shouldSymlinkFile(filename string) bool {
	return strings.HasSuffix(filename, ".dylib")
}

// newFileName strips the full version from a dylib name, keeping only the
// major version (the version the dynamic loader looks for).
func newFileName(filename string) string {
	versionRenameRegex := regexp.MustCompile(`\.(\d+)\.\d+\.\d+`)
	return versionRenameRegex.ReplaceAllString(filename, ".$1")
}
