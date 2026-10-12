# Model Scanning

Model scanners find models that are installed or hosted elsewhere - on a model
hub, in another application's model directory, or in a local folder - and
report them as remote models (model/remoterepository).

## The scanner interface

A scanner implements `definitions.ModelScanner`:

- `ScannerName()` - its name, as listed on the Model Search screen
- `ScanForModels()` - the models it finds by default
- `GetModel(name)` - the metadata of one named model, nil when absent

A scanner whose search can also be driven by a user-supplied query (eg the
search box on the Model Search screen) implements `QueryModelScanner` with
`ScanForQuery(query)`; an empty query behaves like `ScanForModels`.

Scanners register with the remote repository in
`remoterepository.NewRemoteRepository()`; the [Model Search
screen](GUI.md#model-search-page) lists the scanners on the left and runs the
selected one, showing its results on the right.

## The scanners

- **HuggingFace** - lists popular GGUF models from the HuggingFace hub, and
  searches the hub for matching ones. A repository holding several GGUF files
  is listed as one row per file, so a specific quantization can be downloaded.
- **LocalAI** - lists models installed by localai.
- **Filesystem** - searches a directory of your choosing (with a depth limit,
  or unlimited) for `.gguf` files. Every directory holding at least one is
  listed as one model, named by its path relative to the search root, with
  each `.gguf` file as a variant.

## Remote models

A remote model (`definitions.RemoteModel`) carries where its variants' files
can be fetched from (each variant's `Location`: a URL or a local path), the
model's page at its source (`ModelURL`, eg the model's card on a hub), and its
licence, if known.

From the Model Search screen, a found model can be:

- **Download / Install** - its files are fetched (or copied, when the scanner
  points at local files) into the local repository, merged with the variants
  already installed for the model.
- **Link** - only the model's metadata is stored locally; the variants are
  marked linked and point at the files where they already are (nothing is
  copied).
- **Import** - a linked model's files are copied into the local repository,
  turning the link into a local copy.
