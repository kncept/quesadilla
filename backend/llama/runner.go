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

	modelVariant := m.GetDefaultVariant()
	variantPath := m.VariantPath(modelVariant)

	args := []string{
		"", // why do we need this to have and blank (or llama-server)??
		"--model", variantPath,

		// "--jinja", //?
	}

	// Check if we should add mmproj for multimodal support
	if modelVariant.SupportsVision {
		mmprojPath := m.VariantPath(modelVariant)
		args = append(args, "--mmproj", mmprojPath)
	}
	// todo: hadd a get default vision model variant to the model

	args = append(args,
		"--alias", m.ModelName,
		"--host", "localhost",
		"--port", "9931", // 9931 --> planned defult port in the future

		// if it's set larger, llama snipt is back :(
		// "--ctx-size", "524288", // qwen 3.8 is 262,144 - just double it

		// GPU offload *everything possible*
		"--n-gpu-layers", "999",
	)

	cmd := &exec.Cmd{
		Path: "llama-server",
		Args: args,
		Dir:  path.Join(versionedBinDir, fmt.Sprintf("llama-%s", version)),
	}
	cmd.Stdout = logs.Writer()
	cmd.Stderr = logs.Writer()
	fmt.Fprintf(logs.Writer(), "CMD: %+v\n", cmd)

	err := cmd.Start()
	if err != nil {
		fmt.Printf("Error Starting: %v\n", err)
		return nil, err
	}
	return runnerDefinitions.RunDetailsFromCmd(cmd, m.FullModelName(), providerId, version, logs), nil
}
