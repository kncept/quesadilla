package main

import (
	"fmt"
	"slices"

	"github.com/alecthomas/kong"
	"github.com/kncept/quesadilla/backend"
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
	Configure struct {
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
	Run struct {
		Backend string `arg:"" help:"Backend id to run"`
		Model   string `arg:""`
	} `cmd:"" help:"run some AI"`
}

type BackendAndRunnerFlags struct {
	Backend string `flag:""`
	Runner  string `flag:""`
}

func main() {
	ctx := kong.Parse(&CLI)
	switch ctx.Command() {
	case "configure list":
		backendId := CLI.Configure.List.Backend
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

		runnerId := CLI.Configure.List.Runner
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

		if CLI.Configure.List.Remote {
			fmt.Printf("Installable Versions\n")
			for _, v := range r.InstallableVersions() {
				fmt.Printf("%v\n", v)
			}
		}

		return
	case "configure install":
		backendId := CLI.Configure.Install.Backend
		runnerId := CLI.Configure.Install.Runner
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

		version := CLI.Configure.Install.Version
		if CLI.Configure.Install.Latest {
			version = r.LatestVersion() // find latest version
			fmt.Printf("Auto-Detected latest version as: %s\n", version)
		}
		r.RemoveVersion(version)
		r.InstallVersion(version)
	case "configure remove":
		backendId := CLI.Configure.Remove.Backend
		runnerId := CLI.Configure.Remove.Runner
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
		version := CLI.Configure.Remove.Version
		if version == "" {
			fmt.Printf("Please specify a version\n")
			return
		}
		if !slices.Contains(r.InstalledVersions(), version) {
			fmt.Printf("Version not installed:%s\n", version)
			return
		}
		r.RemoveVersion(version)

	case "run <backend> <model>":
		fmt.Printf("RUN SOMETHING\n")
	default:
		fmt.Printf("Fall through\n%+v\n%v\n", CLI, ctx.Command())
		panic(ctx.Command())
	}

}
