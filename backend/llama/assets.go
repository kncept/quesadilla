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
	// Determine the asset pattern to match for the current platform/arch.
	// llama.cpp releases use specific naming conventions:
	// - Linux: ubuntu-x64, ubuntu-arm64 (not linux-amd64)
	// - macOS: macos-x64, macos-arm64
	// - Windows: win-cpu-x64, win-cpu-arm64
	// We match the basic CPU variant (no CUDA, ROCm, Vulkan, etc.)
	var archPattern string
	switch runtime.GOARCH {
	case "amd64":
		archPattern = "x64"
	case "arm64":
		archPattern = "arm64"
	default:
		return fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}

	var osPattern string
	switch runtime.GOOS {
	case "linux":
		osPattern = "ubuntu"
	case "darwin":
		osPattern = "macos"
	case "windows":
		osPattern = "win-cpu"
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	// Match pattern: llama-<version>-bin-<osPattern>-<archPattern>
	// e.g., llama-b4000-bin-ubuntu-x64.tar.gz
	assetPrefix := fmt.Sprintf("llama-%s-bin-%s-%s", version, osPattern, archPattern)

	assetsToDownload := make([]string, 0)
	for name := range nameToDownloadUrl {
		if strings.HasPrefix(name, assetPrefix) {
			// Skip specialized variants (cuda, rocm, vulkan, openvino, etc.)
			// by ensuring the next char after prefix is . or - (end of base name)
			// or it's the exact base pattern
			if len(name) == len(assetPrefix) ||
				strings.HasSuffix(name, ".tar.gz") ||
				strings.HasSuffix(name, ".zip") {
				// Additional check: avoid specialized variants
				suffix := name[len(assetPrefix):]
				if suffix == "" || strings.HasPrefix(suffix, ".") ||
					// Allow .tar.gz, .zip extensions
					(suffix == ".tar.gz" || suffix == ".zip") {
					assetsToDownload = append(assetsToDownload, name)
				}
			}
		}
	}

	downloadsDirectory := qenv.QBinariesDirectory(providerId)
	if len(assetsToDownload) == 0 {
		// Fallback: try to match any asset with the os/arch pattern
		for name := range nameToDownloadUrl {
			if strings.Contains(name, osPattern) && strings.Contains(name, archPattern) &&
				!strings.Contains(name, "cuda") && !strings.Contains(name, "rocm") &&
				!strings.Contains(name, "vulkan") && !strings.Contains(name, "openvino") &&
				!strings.Contains(name, "sycl") && !strings.Contains(name, "openblas") &&
				!strings.Contains(name, "opencl") && !strings.Contains(name, "hip") &&
				!strings.Contains(name, "kompute") && !strings.Contains(name, "llvm") &&
				!strings.Contains(name, "msvc") && !strings.Contains(name, "zdnn") {
				assetsToDownload = append(assetsToDownload, name)
			}
		}
	}
	if len(assetsToDownload) == 0 {
		panic(fmt.Sprintf("Unable to work out what assets to download for %s/%s (tried prefix: %s)", runtime.GOOS, runtime.GOARCH, assetPrefix))
	}

	// Prefer .tar.gz over .zip for Linux/macOS, .zip for Windows
	// But usually there's only one match anyway
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
		} else if strings.HasSuffix(assetName, ".zip") {
			err = compress.Unzip(path.Join(downloadDirectory, assetName), downloadDirectory)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("unknown file format to handle: %s", assetName)
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
