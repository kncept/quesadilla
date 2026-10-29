package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/kncept/quesadilla/app"
	"github.com/kncept/quesadilla/model/definitions"
	"github.com/kncept/quesadilla/model/metadata"
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
		List struct {
			Type string `flag:"type" help:"Filter by model type (gguf, onnx)"`
		} `cmd:"" help:"List installed models, optionally filtered by type"`
		Scanners struct{} `cmd:"" help:"list the available model scanners"`
		Scan struct {
			ScannerName string `arg:"" help:"name of the scanner to run (see 'model scanners')"`
		} `cmd:"" help:"list the models found by a scanner"`
		Info struct {
			ModelName string `arg:"" help:"Model name (family/name or just name)"`
		} `cmd:"" help:"Print model metadata from qmodel.json"`
		GGUFInfo struct {
			ModelName string `arg:"" help:"Model name (family/name or just name)"`
			Variant   string `flag:"variant" help:"Specific variant file to inspect (defaults to first variant)"`
		} `cmd:"" help:"Print GGUF metadata from the model file"`
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
				fmt.Printf("%v\t%v\n", b.Id(), b.Description())
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
	case "model scanners":
		fmt.Printf("Scanners:\n")
		for _, scanner := range qApp.RemoteModels.Scanners() {
			fmt.Printf("%s\n", scanner.ScannerName())
		}
	case "model scan <scanner-name>":
		scanner := qApp.RemoteModels.GetScanner(CLI.Model.Scan.ScannerName)
		if scanner == nil {
			fmt.Printf("Scanner not found: %s\n", CLI.Model.Scan.ScannerName)
			fmt.Printf("Available scanners:\n")
			for _, s := range qApp.RemoteModels.Scanners() {
				fmt.Printf("  %s\n", s.ScannerName())
			}
			return
		}
		scannedModels, err := scanner.ScanForModels()
		if err != nil {
			panic(err)
		}

		installedModelsByName := make(map[string]bool)
		availableModels := qApp.LocalModels.Models()
		for _, m := range availableModels {
			installedModelsByName[m.FullModelName()] = true
		}

		fmt.Printf("LinkID\tLinked\tName\tVariants\n")
		for _, m := range scannedModels {
			fmt.Printf("%s-%s\t%t\t%s\t%d\n", m.ModelSource, m.FullModelName(), installedModelsByName[m.FullModelName()], m.FullModelName(), len(m.Variants))
			// fmt.Printf("%+v\n", model)
		}
	case "model list":
		availableModels := qApp.LocalModels.Models()
		// Filter by type if specified
		if CLI.Model.List.Type != "" {
			// Validate type
			if CLI.Model.List.Type != string(definitions.ModelTypeGGUF) && CLI.Model.List.Type != string(definitions.ModelTypeONNX) {
				log.Fatalf("Invalid model type: %s (must be gguf or onnx)", CLI.Model.List.Type)
			}
			filtered := make([]definitions.Model, 0)
			for _, model := range availableModels {
				if model.ModelType == CLI.Model.List.Type {
					filtered = append(filtered, model)
				}
			}
			availableModels = filtered
		}
		if len(availableModels) == 0 {
			fmt.Printf("No Models Available\n")
		} else {
			fmt.Printf("Models:\n")
			for _, model := range availableModels {
				fmt.Printf("%s\t(%s, %d variant(s))\n", model.FullModelName(), model.ModelType, len(model.Variants))
			}
		}
	case "model info <model-name>":
		model := qApp.LocalModels.GetModel(CLI.Model.Info.ModelName)
		if model == nil {
			log.Fatalf("No such model: %s", CLI.Model.Info.ModelName)
		}
		// Read the qmodel.json directly
		qmodelPath := model.ModelDirectory + "/qmodel.json"
		data, err := os.ReadFile(qmodelPath)
		if err != nil {
			log.Fatalf("Failed to read qmodel.json: %v", err)
		}
		// Pretty print the JSON
		var prettyJSON interface{}
		if err := json.Unmarshal(data, &prettyJSON); err != nil {
			log.Fatalf("Failed to parse qmodel.json: %v", err)
		}
		jsonData, err := json.MarshalIndent(prettyJSON, "", "  ")
		if err != nil {
			log.Fatalf("Failed to format JSON: %v", err)
		}
		fmt.Println(string(jsonData))
	case "model gguf-info <model-name>":
		model := qApp.LocalModels.GetModel(CLI.Model.GGUFInfo.ModelName)
		if model == nil {
			log.Fatalf("No such model: %s", CLI.Model.GGUFInfo.ModelName)
		}
		if model.ModelType != string(definitions.ModelTypeGGUF) {
			log.Fatalf("Model %s is not a GGUF model (type: %s)", model.FullModelName(), model.ModelType)
		}
		// Determine which variant to inspect
		variantPath := ""
		if CLI.Model.GGUFInfo.Variant != "" {
			// Find the specific variant
			found := false
			for _, v := range model.Variants {
				if v.FileName == CLI.Model.GGUFInfo.Variant {
					variantPath = model.VariantPath(v)
					found = true
					break
				}
			}
			if !found {
				log.Fatalf("Variant %s not found in model %s", CLI.Model.GGUFInfo.Variant, model.FullModelName())
			}
		} else {
			// Use the first variant
			variantPath = model.GetDefaultVariantPath()
		}
		if variantPath == "" {
			log.Fatalf("No variant file found for model %s", model.FullModelName())
		}
		ggufMeta, err := metadata.ReadGGUFMetadata(variantPath)
		if err != nil {
			log.Fatalf("Failed to read GGUF metadata: %v", err)
		}
		jsonData, err := json.MarshalIndent(ggufMeta, "", "  ")
		if err != nil {
			log.Fatalf("Failed to marshal GGUF metadata: %v", err)
		}
		fmt.Println(string(jsonData))
	case "model link <link-id>":
		installedModelsByName := make(map[string]bool)
		availableModels := qApp.LocalModels.Models()
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
		scanner := qApp.RemoteModels.GetScanner(scannerName)
		if scanner == nil {
			fmt.Printf("Scanner not found: %s\n", scannerName)
			return
		}
		externalModel := scanner.GetModel(modelName)
		if externalModel == nil {
			fmt.Printf("No %s model %s found", scannerName, modelName)
			return
		}
		err := qApp.LocalModels.LinkScannedModel(externalModel)
		if err != nil {
			panic(err)
		}
	case "model remove <model-name>":
		err := qApp.LocalModels.RemoveModel(CLI.Model.Remove.ModelName)
		if err != nil {
			log.Fatal(err)
		}
	case "run <model-name>":
		m := qApp.LocalModels.GetModel(CLI.Run.ModelName)
		if m == nil {
			log.Fatalf("No such model: %s", CLI.Run.ModelName)
		}
		b := qApp.Backends.BackendForModelType(m.ModelType)
		if b == nil {
			log.Fatalf("No backends available for model of type %s", m.ModelType)
		}
		versions := b.InstalledVersions()
		if len(versions) == 0 {
			log.Fatalf("No installed version of %s - install one first", b.Id())
		}
		fmt.Printf("Running %s with %s (version %s)\n", m.ModelName, b.Id(), versions[0])
		qApp.Start(b, m).Wait()
	default:
		fmt.Printf("Fall through\n%+v\n%v\n", CLI, ctx.Command())
		panic(ctx.Command())
	}
}
