# Quesadilla

A crunchy golang wrapper around a delicious AI Filling.


## Why?
Wraps llama.cpp in a set of easy controls.
Implemented as a CLI interface, will upgrade to include a GUI as well (probably fyne)


## Backends
- `llama-binary` - runs the llama.cpp `llama-server` binary, downloaded from the
  ggml-org/llama.cpp GitHub releases.
- `gogguf` - runs GGUF models with the pure-Go gogguf inference engine, compiled
  into the app (no download; its single `built-in` version is whatever the binary
  was built with). Serves an OpenAI-compatible HTTP API on localhost:9932
  (llama-server uses 9931).

gogguf is consumed from the `feat/asm-rewrite` branch of
`github.com/nkrul/gogguf` (a fork of magomedcoder/gogguf, carrying the
feat/arm64-darwin-support patches plus a rewrite of the non-darwin arm64 NEON
kernels into Go assembly syntax; not yet merged upstream), pinned in `go.mod`
via a `replace` directive to the branch's latest commit. To bump the pin when
the branch advances:
`go mod edit -replace=github.com/magomedcoder/gogguf=github.com/nkrul/gogguf@<commit> && go mod tidy`.
When the patches land upstream, drop the `replace` and require
`github.com/magomedcoder/gogguf` directly:
`go mod edit -dropreplace=github.com/magomedcoder/gogguf && go get github.com/magomedcoder/gogguf@latest`.

## Running
The apps (CLI and macOS GUI) are in the 'app/' directory.
Use `go run cmd/cli/main.go` to run the CLI client
Use `go run cmd/macos/main.go` to run the MacOS client
Use `./run.sh build` to build all targets into `.build/<type>/` (CLI for every supported OS/ARCH, macOS GUI as `.build/macos/Quesadilla.app` for the host)
