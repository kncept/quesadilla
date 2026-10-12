# Models

A model is what Quesadilla runs: a family and a name (together it is
identified as "family/name"; "none/name" when the family is unknown), a type
that selects the backend required to run it, and the variants (files) that
can be run.

## Model types

The model types are `gguf` and `onnx` (model/definitions/model.go). A model's
type implies the backend type required to run it. The full set of model types
the app can run is the union of the types its backends support -
`QApp.SupportedModelTypes()` - so a backend registered for a new model type
makes that type supported automatically.

## Variants

A variant is one runnable file of a model: for gguf, typically one
quantization (a single .gguf file); for onnx, a single .onnx file.

- A variant stored in the model's own directory holds only its file name.
- A linked variant (`LinkedModel`) points at a file elsewhere on the
  filesystem (or a URL), held in its `Location`; nothing is copied.

The model's first variant is its `ModelFile` - the file the backends run.

## The local repository

Installed models live under
`~/.quesadilla/models/<modelType>/<ModelFamily>/<ModelName>/`
(model/localrepository). Each model directory holds its metadata in
`qmodel.json` (family, name, source, type, variants, and licence if one was
stored) alongside the variant files.

The repository scans this tree when it is created and after each
install/remove. Model files added to a model's directory later are picked up
as variants automatically; models without a single runnable variant are not
listed.

Models enter the local repository by being **downloaded** (the files are
fetched into the model's directory), **linked** (only the metadata is stored;
the files stay where they are), or **imported** (a linked model's files are
copied in) - see [Model Scanning](MODEL_SCANNING.md).
