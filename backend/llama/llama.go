package llama

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"strings"

	"github.com/kncept/quesadilla/backend/definitions"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	githubbinary "github.com/kncept/quesadilla/runner/github-binary"
	"github.com/kncept/quesadilla/utils/compress"
	"github.com/kncept/quesadilla/utils/qenv"
	"github.com/kncept/quesadilla/utils/qhttp"
)

const providerId string = "llama"

func LlamaBackend() definitions.Backend {
	llamaBackend := definitions.NewStandardBackend(
		providerId, "Llama.cpp",
		"See https://llama.app/ for details",
		"gguf",
	)
	llamaBackend.RegisterRunner(
		githubbinary.NewGithubBinaryRunnerFromUrl(providerId, "https://github.com/ggml-org/llama.cpp", func(version string, nameToDownloadUrl map[string]string) error {
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
								err = os.Symlink(path.Join(binDir, name), path.Join(binDir, newname))
								if err != nil {
									return err
								}
							}
						}
					}
				}
			}
			return nil
		}),
	)

	llamaBackend.Runner = func(m *modelDefinitions.Model) error {
		fmt.Printf("RUNNING: %+v\n", m)
		binaryRunner := llamaBackend.Runners()[0]
		versionToRun := binaryRunner.InstalledVersions()[0]

		qWorkir, err := os.Getwd()
		if err != nil {
			return err
		}

		versionedBinDir := path.Join(qenv.QBinariesDirectory(providerId), versionToRun)

		cmd := &exec.Cmd{
			Path: "llama-server",
			Args: []string{
				"", // why do we need this to have and blank (or llama-server)??
				"--model", m.ModelFile,
				"--host", "localhost",
				"--port", "8080", // 9931 --> planned defult port in the future
				"--n-gpu-layers", "999",
				"--log-file", path.Join(qWorkir, "llama.log"),
			},
			Dir: path.Join(versionedBinDir, fmt.Sprintf("llama-%s", versionToRun)),
		}

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		fmt.Printf("CMD: %+v\n", cmd)

		err = cmd.Start()
		if err != nil {
			fmt.Printf("Error Starting: %v\n", err)
			fmt.Println(stdout.String(), stderr.String())
			return err
		}
		err = cmd.Wait()
		fmt.Println(stdout.String(), stderr.String())
		return err

	}
	return llamaBackend
}

func shouldSymlinkFile(filename string) bool {
	return strings.HasSuffix(filename, ".dylib")
}
func newFileName(filename string) string {
	versionRenameRegex := regexp.MustCompile(`\.(\d+)\.\d+\.\d+`)
	return versionRenameRegex.ReplaceAllString(filename, ".$1")
}
