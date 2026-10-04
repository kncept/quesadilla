package main

import (
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/kncept/quesadilla/backend"
	"github.com/kncept/quesadilla/model"
	"github.com/kncept/quesadilla/model/repository"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// var CLI struct {
// 	Configure struct {
// 		Backend string `flag"" `
// 		Runner  string `flag:""`

// 		// Id     string `arg:"" name:"backend-id" default:""`
// 		// Runner string `flag:"" help:"Runner ID"`
// 	} `cmd:"" help:"Backend Configuration."`
// 	Run struct {
// 		Backend string `arg:"" help:"Backend id to run"`
// 		Model   string `arg:""`
// 	} `cmd:"" help:"run some AI"`
// }

var CLI struct {
	// backend struct {
	Backend struct {
		List struct {
			BackendAndRunnerFlags
			Remote bool `flag:""`
		} `cmd:""`
		Install struct {
			BackendAndRunnerFlags
			Version string `flag:"" xor:"version"`
			Latest  bool   `flab:"" xor:"version"`
		} `cmd:""`
		Remove struct {
			BackendAndRunnerFlags
			Version string `flag:"" xor:"version"`
		} `cmd:""`
	} `cmd:"" help:"Backend Configuration."`
	Model struct {
		List struct{} `cmd:""`
		Scan struct{} `cmd:""`
		Link struct {
			LinkId string `arg:""`
		} `cmd:""`
		Remove struct {
			ModelName string `arg:""`
		} `cmd:""`
	} `cmd:"Model Configuration"`
	Run struct {
		ModelName string `arg:""`
	} `cmd:"" help:"run some AI"`
}

type BackendAndRunnerFlags struct {
	Id     string `flag:""`
	Runner string `flag:""`
}

func main() {
	ctx := kong.Parse(&CLI)
	switch ctx.Command() {
	case "backend list":
		backendId := CLI.Backend.List.Id
		if backendId == "" { // no backend specified, just list them
			fmt.Printf("Available Backends:\n%v\t%v\n", "Backend", "Name")
			for _, b := range backend.Backends() {
				fmt.Printf("%v\t%v\n", b.Id(), b.Name())
			}
			return
		}
		b := backend.Backend(backendId)
		if b == nil {
			fmt.Printf("No Such Backend: %v\n", backendId)
			return
		}

		runnerId := CLI.Backend.List.Runner
		if runnerId == "" {
			fmt.Printf("Runner:\n%v\t%v\n", "Runner", "Name")
			for _, r := range b.Runners() {
				fmt.Printf("%v\t%v\n", r.Id(), r.Name())
			}
			return
		}

		r := runnerDefinitions.GetRunner(b.Runners(), runnerId)
		if r == nil {
			fmt.Printf("No Such Runner: %v\n", runnerId)
			return
		}

		installedVersions := r.InstalledVersions()
		installedCount := len(installedVersions)
		if installedCount == 0 {
			fmt.Printf("No Installed Versions\n")
		} else {
			fmt.Printf("Installed Versions:\n")
			for _, v := range r.InstalledVersions() {
				fmt.Printf("%v\n", v)
			}
		}

		if CLI.Backend.List.Remote {
			fmt.Printf("Installable Versions\n")
			for _, v := range r.InstallableVersions() {
				fmt.Printf("%v\n", v)
			}
		}

		return
	case "backend install":
		backendId := CLI.Backend.Install.Id
		runnerId := CLI.Backend.Install.Runner
		b := backend.Backend(backendId)
		if b == nil {
			fmt.Printf("No Such Backend: %v\n", backendId)
			return
		}
		r := runnerDefinitions.GetRunner(b.Runners(), runnerId)
		if r == nil {
			fmt.Printf("No Such Runner: %v\n", runnerId)
			return
		}

		version := CLI.Backend.Install.Version
		if CLI.Backend.Install.Latest {
			version = r.LatestVersion() // find latest version
			fmt.Printf("Auto-Detected latest version as: %s\n", version)
		}
		r.RemoveVersion(version)
		r.InstallVersion(version)
	case "backend remove":
		backendId := CLI.Backend.Remove.Id
		runnerId := CLI.Backend.Remove.Runner
		b := backend.Backend(backendId)
		if b == nil {
			fmt.Printf("No Such Backend: %v\n", backendId)
			return
		}
		r := runnerDefinitions.GetRunner(b.Runners(), runnerId)
		if r == nil {
			fmt.Printf("No Such Runner: %v\n", runnerId)
			return
		}
		version := CLI.Backend.Remove.Version
		if version == "" {
			fmt.Printf("Please specify a version\n")
			return
		}
		if !slices.Contains(r.InstalledVersions(), version) {
			fmt.Printf("Version not installed:%s\n", version)
			return
		}
		r.RemoveVersion(version)
	case "model scan":
		scanner := model.NewScannerRegistry()
		scannedModels, err := scanner.ScanForModels()
		if err != nil {
			panic(err)
		}

		installedModelsByName := make(map[string]bool)
		availableModels, err := repository.ListAvailableModels()
		if err != nil {
			panic(err)
		}
		for _, m := range availableModels {
			installedModelsByName[m.ModelName] = true
		}

		fmt.Printf("LinkID\tLinked\tName\n")
		for _, m := range scannedModels {
			fmt.Printf("%s-%s\t%t\t%s\n", m.ScannerName, m.ModelName, installedModelsByName[m.ModelName], m.ModelName)
			// fmt.Printf("%+v\n", model)
		}
	case "model list":
		availableModels, err := repository.ListAvailableModels()
		if err != nil {
			panic(err)
		}
		if len(availableModels) == 0 {
			fmt.Printf("No Models Available\n")
		} else {
			fmt.Printf("Models:\n")
			for _, model := range availableModels {
				fmt.Printf("%s\n", model.ModelName)
			}
		}
	case "model link <link-id>":
		installedModelsByName := make(map[string]bool)
		availableModels, err := repository.ListAvailableModels()
		if err != nil {
			panic(err)
		}
		for _, m := range availableModels {
			installedModelsByName[m.ModelName] = true
		}
		scannerName, modelName, found := strings.Cut(CLI.Model.Link.LinkId, "-")
		if !found {
			fmt.Printf("Unable to parse %s\n", CLI.Model.Link.LinkId)
			return
		}
		if installedModelsByName[modelName] {
			fmt.Printf("Model already present: %s\n", modelName)
			return
		}
		scanners := model.NewScannerRegistry()
		scanner := scanners.GetScanner(scannerName)
		if scanner == nil {
			fmt.Printf("Scanner not found: %s\n", scannerName)
			return
		}
		externalModel := scanner.GetModel(modelName)
		if externalModel == nil {
			fmt.Printf("No %s model %s found", scannerName, modelName)
			return
		}
		err = repository.LinkScannedModel(externalModel)
		if err != nil {
			panic(err)
		}
	case "model remove <model-name>":
		err := repository.RemoveModel(CLI.Model.Remove.ModelName)
		if err != nil {
			log.Fatal(err)
		}
	case "run <model-name>":
		m := repository.GetModel(CLI.Run.ModelName)
		if m == nil {
			log.Fatalf("No such model: %s", CLI.Run.ModelName)
		}
		b := backend.BackendForModelType(m.ModelType)
		if b == nil {
			log.Fatalf("No backends available for model of type %s", m.ModelType)
		}
		err := b.Run(m)
		if err != nil {
			log.Fatal(err)
		}

	default:
		fmt.Printf("Fall through\n%+v\n%v\n", CLI, ctx.Command())
		panic(ctx.Command())
	}
}
