# Quesadilla

A crunchy golang wrapper around a delicious AI Filling.

## Quickstart

```sh
git clone https://github.com/kncept/quesadilla
cd quesadilla
./run.sh build
```

Then run it:

```sh
open .build/macos/Quesadilla.app      # the macOS GUI (on a macOS host)
# and/or
.build/cli/quesadilla-cli_darwin_arm64 backend list   # the CLI
```

That's all there is to it — nothing else to install. `./run.sh build` builds
the CLI for every supported OS/ARCH into `.build/cli/`, and (on a macOS host)
the macOS GUI as `.build/macos/Quesadilla.app`.

## Why?

Wraps llama.cpp and other inference runtimes in a set of easy controls, with
a GUI and a CLI.

## Backends

- `llama-binary` - runs the llama.cpp `llama-server` binary, downloaded from
  the ggml-org/llama.cpp GitHub releases.
- `gogguf` (WIP) - runs GGUF models with the pure-Go gogguf inference engine,
  compiled into the app (no download; its single `built-in` version is
  whatever the binary was built with). Serves an OpenAI-compatible HTTP API
  on localhost:9932 (llama-server uses 9931).
- `onnx` - runs ONNX models with the pure-Go onnx-go inference engine
  (github.com/oramasearch/onnx-go), compiled into the app. Serves a small
  HTTP inference API on localhost:9933.

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

## Documentation

- [GUI](docs/GUI.md)
- [Backends](docs/BACKENDS.md)
- [Models](docs/MODELS.md)
- [Model Scanning](docs/MODEL_SCANNING.md)
- [Config](docs/CONFIG.md)
- [Contributing](docs/CONTRIBUTING.md)

## Development

Use `go run cmd/cli/main.go` to run the CLI client.
Use `go run cmd/macos/main.go` to run the MacOS client.
Use `./run.sh test` to run the tests.
