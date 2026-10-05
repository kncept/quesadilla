# Quesadilla

A crunchy golang wrapper around a delicious AI Filling.


## Why?
Wraps llama.cpp in a set of easy controls.
Implemented as a CLI interface, will upgrade to include a GUI as well (probably fyne)


## Running
The apps (CLI and macOS GUI) are in the 'app/' directory.
Use `go run app/cli/main.go` to run the CLI client
Use `go run app/macos/main.go` to run the MacOS client
Use `./run.sh build` to build all targets into `.build/<type>/` (CLI for every supported OS/ARCH, macOS GUI for the host)
