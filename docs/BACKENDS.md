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

### `embedded-llama`

Runs GGUF models with llama.cpp compiled into the app through the
`github.com/tcpipuk/llama-go` cgo bindings. Like gogguf and onnx it has a
single `built-in` version (nothing to install) and serves models in-process, on
localhost:9934, over an OpenAI-compatible API:

- `GET /` - a JSON summary of what is being served
- `GET /v1/models` - the served model, in OpenAI's list shape
- `POST /v1/chat/completions` - OpenAI chat completions; `"stream": true`
  streams server-sent events
- `POST /v1/completions` - OpenAI text completions from a raw prompt; also
  supports streaming
- `POST /v1/responses` - the OpenAI Responses API, with tool (function) calling;
  `"stream": true` streams its typed server-sent events

#### Tool calling on `/v1/responses`

llama-go has no native tool support, so this endpoint layers a small text
protocol over ordinary role/content chat completions:

- the tool schemas are advertised in a system message inside `<tools>`;
- the model asks to call a tool by emitting a JSON object wrapped in
  `<tool_call></tool_call>` (each call becomes a `function_call` output item);
- tool results are fed back as a `<tool_response call_id="...">...</tool_response>`
  message.

This follows the convention popularised by Qwen/Hermes models, so an
instruction-tuned model usually complies. It is an instruction-level protocol
rather than grammar-constrained decoding, so a model may ignore it and return
plain text instead; the endpoint returns whatever it produced. The endpoint
implements a pragmatic subset of the Responses API: `input` (a string or an
array of message/`function_call`/`function_call_output` items), `instructions`,
`tools`, `tool_choice`, `max_output_tokens`, `temperature`, `top_p` and
`stream`. `previous_response_id`, `store` and reasoning output items are not
supported.

Unlike the pure-Go backends this one is cgo: it links llama.cpp through
llama-go, which cannot be used as a plain module dependency because its cgo
flags look for llama.cpp's static libraries (`libbinding.a` and friends) beside
the package. To keep that self-contained, llama-go is vendored as a git
submodule under `third_party/llama-go` (with llama.cpp as its own nested
submodule) and `go.mod` replaces the module with it. The libraries are built
in-repo:

```sh
# Fetch the submodules and build the static libraries beside llama-go's Go
# files. ./run.sh build:llama initialises the submodules itself (a no-op once
# they are present, and ./run.sh dependencies does it too); it needs cmake and
# a C/C++ toolchain, and on macOS also builds the Metal and Accelerate
# archives.
./run.sh build:llama
```

After that, any cgo build embeds the backend:

```sh
CGO_ENABLED=1 go build ./cmd/macos   # or ./cmd/cli
```

A build with cgo disabled (`CGO_ENABLED=0`, such as the cross-platform CLI
built by `./run.sh build:cli`) is pure Go: it still lists the backend with its
`built-in` version, but its `Start` fails with a message pointing here.
llama-go's own build docs cover the GPU backends (CUDA, Metal, and the rest)
and its `shared_lib` build tag; its generated `libbinding.a` must sit next to
the llama-go Go files, which the in-repo submodule provides.

Only one model can be served per in-process backend at a time (a second start
fails with "address already in use"); that applies to gogguf, onnx and
embedded-llama. Each model the llama backend runs is its own process.

## Adding a backend

1. Create `backend/<name>/` implementing `definitions.Backend` - the gogguf
   and onnx backends show the in-process pattern (load the model, serve an
   HTTP API, return a `RunningModel`), the llama backend the child-process
   pattern, and embedded-llama the in-process cgo pattern (a cgo implementation
   plus a stub for CGO_ENABLED=0 builds).
2. Register it in `defaultBackends()` in backend/backend.go.

Its model types then show up on the Backends screen and in
`QApp.SupportedModelTypes()` (the set of all model types the app can run)
automatically.
