# Config

Quesadilla keeps its state under `~/.quesadilla/`:

```
~/.quesadilla/
  config.json                     the application config
  models/<type>/<family>/<name>/  installed models (their qmodel.json + files)
  runners/<backend>/binaries/     downloaded backend runtimes (eg llama.cpp)
```

## config.json

The config (config/config.go) is the ordered list of (model type, backend id)
pairs that lays out the [Backends screen](GUI.md#backends-page) and decides
which backend is the default for each model type - the topmost backend of a
type's group:

```json
{
  "backendOrder": [
    {"modelType": "gguf", "backendId": "llama-binary"},
    {"modelType": "gguf", "backendId": "gogguf"},
    {"modelType": "onnx", "backendId": "onnx"}
  ]
}
```

The app writes the file when the order changes (eg a row is moved on the
Backends screen). A missing file is not an error - the app falls back to the
default order, which is the backends' scan order.

When the config is loaded it is reconciled against the current backends:
entries for backends (or model types) that no longer exist are dropped,
duplicates are collapsed, and backends that appeared since the order was
saved are appended to the end of their model type's group - so a saved order
never lists a backend that is gone, and never misses one that is new.
