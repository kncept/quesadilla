package llama

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"

	"github.com/kncept/quesadilla/backend/running"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	"github.com/kncept/quesadilla/utils/qenv"
)

// runLlamaServer serves the model with the given installed version of
// llama.cpp, blocking until the server exits.
func runLlamaServer(m *modelDefinitions.Model, version string) error {
	fmt.Printf("RUNNING: %+v\n", m)

	qWorkir, err := os.Getwd()
	if err != nil {
		return err
	}

	versionedBinDir := path.Join(qenv.QBinariesDirectory(providerId), version)

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
		Dir: path.Join(versionedBinDir, fmt.Sprintf("llama-%s", version)),
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
	// report the model as running for as long as the server lives
	stopTracking := running.Default().Track(*m)
	defer stopTracking()
	err = cmd.Wait()
	fmt.Println(stdout.String(), stderr.String())
	return err
}
