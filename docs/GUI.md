# GUI

There are two GUI components that wrap the main application.

The first is a lightweight 'system tray' or 'menu bar' style that allows launching the main GUI, and potentially offers a few basic but useful controls.

The second is a full control suite app.

# Lightweight Indicator

- Allows launching the main control suite app
- has a 'no models running', or a 'models running' and a list of running models.
  Selecting a running model's name opens that model's log viewer (or focuses it,
  when it is already open).

# Control Suite

Main control suite.
Essentially a collapsable menu on the left of the screen, and a content view for the remainder of the window.
The following screens are available:

## Overview
An overview page, with a lot of stats in it.
Includes a list of running models

### Running Models section
Shows all running models, and some stats, including uptime.
Has a 'stop model' button that will top the model
Has a 'view logs' button that opens the log viewer for that model

## Log Viewer
A simple, drill-in window opened from a running model's 'view logs' button, or
from the model's name under 'Models running:' in the tray. Each running model
has at most one log window: opening the logs again brings the existing window
to the front instead of opening a second one.
It shows the model's recent captured log lines (kept in memory, up to a
configurable maximum per model, default 200 lines) in a scrollable area, and
keeps updating while the model is still running.
Has a 'follow' checkbox (starts checked) that keeps the view pinned to the
newest lines, plus a 'refresh' button to reload on demand
Has a 'back' button at the bottom that closes the window

## Models Page
Installed Models (including config and running stats)
Each row's actions are in the leftmost column: a start/stop button, a delete
button (which asks for confirmation before removing the model), a logs
button that is enabled only while the model is running, a Details button that
opens the Model Details page for that model, and - for models whose metadata
records a page at their source (stored when the model was downloaded or linked)
- an Open button that opens the model's page in the system browser, the same
behaviour as the Open button on the Model Search page.

## Model Details Page
Metadata for one installed model at a time. A dropdown at the top selects the
model to inspect. Below it the page is laid out like the Model Search page: a
menu on the left, with the selected view on the right.

The left menu holds **Model**, then **Download** (only for models that can fetch
more variants, eg HuggingFace models), then one row per installed variant.
Selecting **Model** or a variant shows its metadata on the right. Selecting
**Download** shows a table of the model's downloadable variants, with a
**Download** button for each variant that is not installed yet; variants that
are already downloaded are listed without a button.

### Scan Models Section
Includes a 'scan' section where Models can be scanned, and can be linked or unlinked.
This shows as a table, with a row per found model, listing out the details and operations

### Available Models Section
Includes an 'available' section which shows model metadata, if it's running or not, and can start/stop models from running
This shows as a table, with a row per found model, listing out the details and operations

## Backends Page
Backends broken out by the model types they support: one row per (model type, backend), so a backend that supports several model types appears once per type.
The rows follow the persisted (model type, backend id) order, where the topmost backend of a model type is the default backend for that type.
Each row has move up/down buttons to reorder the backends within their model type; the order is persisted in the config (~/.quesadilla/config.json), so it (and its default) survive a restart.

## Model Search Page
Find models to install from several sources. The scanners are listed on the left;
selecting one runs its search and lists the results in a table on the right.
Each result row lists the model's **Name** and **Type**, then its action buttons
(between the type and the file, so the buttons sit next to the details they act
on), then the **File** they act on.

- **HuggingFace** lists popular GGUF models from the HuggingFace hub, and shows
  a search box above the results: submitting a query (or pressing Enter) runs a
  hub search for matching GGUF models, while an empty query lists the popular
  ones again. A query that names a model directly - a model page URL (eg
  "https://huggingface.co/unsloth/Qwen3.8-27B-GGUF") or the model's hub id (eg
  "unsloth/Qwen3.8-27B-GGUF") - fetches that model on its own instead of
  searching. A repository holding several GGUF files is listed as one row per
  file, so a specific quantization can be downloaded; the model's name and type
  are shown only on the group's first row, so the group reads as one model.
- **LocalAI** lists models installed by localai.
- **Filesystem** searches a directory of your choosing for `.gguf` files. It
  shows a small form: the directory to search, a numeric `Depth` (how many
  levels below the directory to descend, `0` being the directory itself), and
  an `Unlimited` checkbox. Checking `Unlimited` disables the depth entry and
  shows it as `-1`. The search button stays disabled until a directory is
  entered and the depth is valid (a number `>= 0`, or unlimited). Every
  directory holding at least one `.gguf` file is listed as one model, named by
  its path relative to the search root, with each `.gguf` file as a variant.

Results carry action buttons in the action column:

- **HuggingFace** models have the usual **Download** (or **Redownload**)
  button, plus an **Open** button to its right that opens the model's card in
  the system browser. When a model's files each get their own row, the download
  button fetches just that row's file.
- **LocalAI** models follow their link/install state: a model that is not
  installed locally shows **Link** (point at the localai files, copying
  nothing) and **Install** (copy the files into quesadilla's data directory);
  a linked model shows **Unlink** (drop the link, leaving the source files
  alone) and **Install** (turn the link into a local copy); an installed model
  shows only **Uninstall** (remove the model and its files).
- **Filesystem** models have a **Link** button, which installs the model by
  pointing at its files where they are (nothing is copied), and an **Import**
  button, which copies the model's files into quesadilla's data directory like
  a normal download.

## Downloads Page
Lists every download and download job currently in progress, with its name,
source, destination and elapsed time. Downloads start in the background and
leave the list once they finish, whether they succeeded or failed; the screen
refreshes automatically while it is open.

Downloads started from the Model Search and Model Details screens are listed
here too: a model download job stays listed until all of its files have been
fetched and its screen has been updated. Linking a model does not download
anything, so it is not listed.

# Technology

The GUI is written in https://github.com/fyne-io/fyne
