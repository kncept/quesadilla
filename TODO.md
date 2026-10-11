
Model storage (flexible, HuggingFace-like)
==========================================

[x] 1) The directory we store models in is <ModelFamily>/<ModelName> (under models/<modelType>/), using 'none' when we don't know the model family
[x] 2) We handle multiple variants (multiple .gguf files) in the directory
[x] 3) ALL models have a 'qmodel.json' (replacing the old 'model.json'), listing:
   - Model Family (matches the directory)
   - Model Name (matches the directory)
   - Model Source (name of the scanner that found the model)
   - Model Type (currently "gguf")
   - Variants (each of the variants/files we can run):
     - File name
     - Description (optional)
     - When Imported (Date and Time)
     - Filesize (bytes)
     - LinkedModel (boolean, false for 'having local files', true for 'pointing somewhere else on the filesystem)
   - Licence Name (optional - eg: apache-2.0)
   - Licence File (optional - only if present... and downloaded if we can)

Legacy model directories (a bare model file, or a single model.json) are migrated to the new
layout automatically when the local repository scans.

Still to do (this applies to LOCAL, RUNNABLE models; when listing remote models we handle the
same concerns, but they are not in our repository):
- The Model Search screen currently downloads ALL of a model's variants. When there are
  variants, we need to pick which variant to download (the repository already supports
  per-variant installs via LocalRepository.DownloadVariant, so this is a GUI change).
- Any models with a partial variant download need to be merged into the download view, so we
  can download other variants on top of what we downloaded already.
- When listing remote models, we need to filter for USEABLE models:
  - Ensure the model contains files we can work with (eg: at least one .gguf file) - models
    with no runnable variants are trimmed (done for the HuggingFace and LocalAI scanners)
  - Ensure that we list out variants (eg: when there are multiple .gguf files within the model) (done)


Update the docs
1) Add a 'contributing' section - detail the standard 'fork, branch, PR' pattern. Make sure that we ask for tests for added/changed functionality
2) Document the different application sections
   - GUI
   - Models (Local Model Handling)
   - Backends (Running Models Locally)
   - Scanning (Remote Repositories)


When listing models, we need to filter this for USEABLE models.
using Huggingface as an example, 
- Ensure the model contains files we can work with. eg: at least one .gguf file
- Ensure that we list out variants (eg: When there are multiple .gguf files withing the model)
  - WHEN there are variants, we have to pick which variant to download, since we identify models by <ModelFamily>/<ModelName> or just by <ModelName>



To summarize:
within a model, consider a .gguf file to be a variant.
If there are no variants (.gguf) files, don't show the model



And finally, when we reach the bottom of the list, there should be a 'fetch more' button that fetches more, UNLESS there are no more results (Less than pagesize models returned for previous request) (N.B. This _may_ mean we end up fetching a 0 page before disabling the button).
