# Backends

A backend is a pluggable inference runtime Quesadilla can run models with.
Backends are selected per model type: each backend declares which model types
it supports, and the persisted backend order (reorderable on the [Backends
screen](GUI.md#backends-page)) decides which backend is the default for each
type (the topmost backend of the type's group).

## The backend interface

Every backend implements `definitions.Backend` (backend/definitions/backend.go):

- `Id()` / `Description()` - its identity, shown in the GUI and the CLI
- `ModelTypes()` - the model types it can run
- `InstalledVersions()` / `InstallableVersions()` / `LatestVersion()` - the
  runtime versions that are installed, available to install, and the newest
- `InstallVersion(v)` / `RemoveVersion(v)` - manage installed versions
- `Start(model)` - run the model, returning a `RunningModel` - the handle the
  app uses to wait on, stop, and read the logs of the run

## Known backends

### `llama-binary` (llama.cpp)

Runs the llama.cpp `llama-server` binary as a child process. Versions are
downloaded from the ggml-org/llama.cpp GitHub releases and installed under
`~/.quesadilla/runners/llama-binary/binaries/<version>/`. Serves an
OpenAI-compatible API on localhost:9931. Supports `gguf` models.

### `gogguf` (WIP)

Runs GGUF models with the pure-Go gogguf inference engine, compiled into the
app (no download; its single `built-in` version is whatever the binary was
built with). Serves an OpenAI-compatible HTTP API on localhost:9932.
Work in progress: it is consumed from the `feat/asm-rewrite` branch of
`github.com/nkrul/gogguf` (a fork of magomedcoder/gogguf), pinned in `go.mod`
via a `replace` directive - see the README for how to bump the pin.

### `onnx`

Runs ONNX models with the pure-Go onnx-go inference engine
(github.com/oramasearch/onnx-go, executed with Gorgonia), compiled into the
app. Like gogguf it has a single `built-in` version (nothing to install).
Serves a small HTTP inference API on localhost:9933:

- `GET /` - a JSON summary of what is being served
- `POST /inference` - takes one flat list of float values per model input
  (in the model's input order; each input may carry its shape, otherwise the
  shape the model declared for it is used) and replies with the model's
  outputs, one flat list per output

Only one model can be served per in-process backend at a time (a second start
fails with "address already in use"); each model the llama backend runs is its
own process.

## Adding a backend

1. Create `backend/<name>/` implementing `definitions.Backend` - the gogguf
   and onnx backends show the in-process pattern (load the model, serve an
   HTTP API, return a `RunningModel`), the llama backend the child-process
   pattern.
2. Register it in `defaultBackends()` in backend/backend.go.

Its model types then show up on the Backends screen and in
`QApp.SupportedModelTypes()` (the set of all model types the app can run)
automatically.
