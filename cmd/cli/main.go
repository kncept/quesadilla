package main

import (
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/kncept/quesadilla/app"
)

var CLI struct {
	Backend struct {
		List struct {
			Id     string `flag:"id" help:"Backend ID (omit to list all backends)"`
			Remote bool   `flag:"remote" help:"Also list installable versions"`
		} `cmd:"" help:"List backends, or the versions of one backend"`
		Install struct {
			Id      string `flag:"id" help:"Backend ID"`
			Version string `flag:"version" xor:"version" help:"Version to install"`
			Latest  bool   `flag:"latest" xor:"version" help:"Install the latest version"`
		} `cmd:"" help:"Install a version of a backend"`
		Remove struct {
			Id      string `flag:"id" help:"Backend ID"`
			Version string `flag:"version" help:"Version to remove"`
		} `cmd:"" help:"Remove an installed version of a backend"`
		Scan struct {
			Id string `flag:"id" help:"Backend ID"`
		} `cmd:"" help:"List installable versions of a backend"`
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

func main() {
	ctx := kong.Parse(&CLI)
	qApp := app.New()
	switch ctx.Command() {
	case "backend list":
		backendId := CLI.Backend.List.Id
		if backendId == "" { // no backend specified, just list them
			fmt.Printf("Available Backends:\n%v\t%v\n", "Backend", "Name")
			for _, b := range qApp.Backends.Backends() {
				fmt.Printf("%v\t%v\n", b.Id(), b.Name())
			}
			return
		}
		b := qApp.Backends.Backend(backendId)
		if b == nil {
			fmt.Printf("No Such Backend: %v\n", backendId)
			return
		}

		installedVersions := b.InstalledVersions()
		if len(installedVersions) == 0 {
			fmt.Printf("No Installed Versions\n")
		} else {
			fmt.Printf("Installed Versions:\n")
			for _, v := range installedVersions {
				fmt.Printf("%v\n", v)
			}
		}

		if CLI.Backend.List.Remote {
			fmt.Printf("Installable Versions\n")
			for _, v := range b.InstallableVersions() {
				fmt.Printf("%v\n", v)
			}
		}

		return
	case "backend install":
		backendId := CLI.Backend.Install.Id
		b := qApp.Backends.Backend(backendId)
		if b == nil {
			fmt.Printf("No Such Backend: %v\n", backendId)
			return
		}

		version := CLI.Backend.Install.Version
		if CLI.Backend.Install.Latest {
			version = b.LatestVersion() // find latest version
			fmt.Printf("Auto-Detected latest version as: %s\n", version)
		}
		if version == "" {
			fmt.Printf("Please specify a version (or use --latest)\n")
			return
		}
		b.RemoveVersion(version)
		b.InstallVersion(version)
	case "backend remove":
		backendId := CLI.Backend.Remove.Id
		b := qApp.Backends.Backend(backendId)
		if b == nil {
			fmt.Printf("No Such Backend: %v\n", backendId)
			return
		}
		version := CLI.Backend.Remove.Version
		if version == "" {
			fmt.Printf("Please specify a version\n")
			return
		}
		if !slices.Contains(b.InstalledVersions(), version) {
			fmt.Printf("Version not installed:%s\n", version)
			return
		}
		b.RemoveVersion(version)
	case "backend scan":
		backendId := CLI.Backend.Scan.Id
		if backendId == "" {
			fmt.Printf("Please specify a backend ID (--id)\n")
			return
		}
		b := qApp.Backends.Backend(backendId)
		if b == nil {
			fmt.Printf("No Such Backend: %v\n", backendId)
			return
		}

		installableVersions := b.InstallableVersions()
		if len(installableVersions) == 0 {
			fmt.Printf("No Installable Versions\n")
		} else {
			fmt.Printf("Installable Versions:\n")
			for _, v := range installableVersions {
				fmt.Printf("%v\n", v)
			}
		}
		return
	case "model scan":
		scannedModels, err := qApp.Models.ScanForModels()
		if err != nil {
			panic(err)
		}

		installedModelsByName := make(map[string]bool)
		availableModels := qApp.Models.Models()
		for _, m := range availableModels {
			installedModelsByName[m.ModelName] = true
		}

		fmt.Printf("LinkID\tLinked\tName\n")
		for _, m := range scannedModels {
			fmt.Printf("%s-%s\t%t\t%s\n", m.ScannerName, m.ModelName, installedModelsByName[m.ModelName], m.ModelName)
			// fmt.Printf("%+v\n", model)
		}
	case "model list":
		availableModels := qApp.Models.Models()
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
		availableModels := qApp.Models.Models()
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
		scanner := qApp.Models.GetScanner(scannerName)
		if scanner == nil {
			fmt.Printf("Scanner not found: %s\n", scannerName)
			return
		}
		externalModel := scanner.GetModel(modelName)
		if externalModel == nil {
			fmt.Printf("No %s model %s found", scannerName, modelName)
			return
		}
		err := qApp.Models.LinkScannedModel(externalModel)
		if err != nil {
			panic(err)
		}
	case "model remove <model-name>":
		err := qApp.Models.RemoveModel(CLI.Model.Remove.ModelName)
		if err != nil {
			log.Fatal(err)
		}
	case "run <model-name>":
		m := qApp.Models.GetModel(CLI.Run.ModelName)
		if m == nil {
			log.Fatalf("No such model: %s", CLI.Run.ModelName)
		}
		b := qApp.Backends.BackendForModelType(m.ModelType)
		if b == nil {
			log.Fatalf("No backends available for model of type %s", m.ModelType)
		}
		versions := b.InstalledVersions()
		if len(versions) == 0 {
			log.Fatalf("No installed version of %s - install one first", b.Name())
		}
		fmt.Printf("Running %s with %s (version %s)\n", m.ModelName, b.Name(), versions[0])
		qApp.Start(b, m)
		qApp.AwaitAll()

	default:
		fmt.Printf("Fall through\n%+v\n%v\n", CLI, ctx.Command())
		panic(ctx.Command())
	}
}
