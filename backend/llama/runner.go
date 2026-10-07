package llama

import (
	"fmt"
	"os/exec"
	"path"

	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
	"github.com/kncept/quesadilla/utils/qenv"
)

// runLlamaServer serves the model with the given installed version of
// llama.cpp, blocking until the server exits.
func startLlamaServer(m *modelDefinitions.Model, version string) (runnerDefinitions.RunningModel, error) {
	// fmt.Printf("RUNNING: %+v\n", m)

	// qWorkir, err := os.Getwd()
	// if err != nil {
	// 	return nil, err
	// }

	versionedBinDir := path.Join(qenv.QBinariesDirectory(providerId), version)

	// Keep the last N lines of the server's output in memory, per model, for
	// the log viewer. Both stdout and stderr feed the same buffer (via
	// independent writers, so their partial lines don't interleave).
	logs := runnerDefinitions.NewLogBuffer(runnerDefinitions.DefaultMaxLogLines)

	cmd := &exec.Cmd{
		Path: "llama-server",
		Args: []string{
			"", // why do we need this to have and blank (or llama-server)??
			"--model", m.ModelFile,
			"--host", "localhost",
			"--port", "8080", // 9931 --> planned defult port in the future

			// GPU offload *everything possible*
			"--n-gpu-layers", "999",
		},
		Dir: path.Join(versionedBinDir, fmt.Sprintf("llama-%s", version)),
	}
	cmd.Stdout = logs.Writer()
	cmd.Stderr = logs.Writer()

	fmt.Printf("CMD: %+v\n", cmd)

	err := cmd.Start()
	if err != nil {
		fmt.Printf("Error Starting: %v\n", err)
		return nil, err
	}
	return runnerDefinitions.RunDetailsFromCmd(cmd, m.ModelName, providerId, version, logs), err
}
