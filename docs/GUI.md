# GUI

There are two GUI components that wrap the main application.

The first is a lightweight 'system tray' or 'menu bar' style that allows launching the main GUI, and potentially offers a few basic but useful controls.

The second is a full control suite app.

# Lightweight Indicator

- Allows launching the main control suite app
- has a 'no models running', or a 'models running' and a list of running models.

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

## Models Page
Installed Models (including config and running stats)

### Scan Models Section
Includes a 'scan' section where Models can be scanned, and can be linked or unlinked.
This shows as a table, with a row per found model, listing out the details and operations

### Available Models Section
Includes an 'available' section which shows model metadata, if it's running or not, and can start/stop models from running
This shows as a table, with a row per found model, listing out the details and operations

## Backends Page
A list of Installed Backends, where each backend also shows a list of it's running models

# Technology

The GUI is written in https://github.com/fyne-io/fyne
