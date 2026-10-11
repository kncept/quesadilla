package gui

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/quesadilla/model/remoterepository"
)

// newFilesystemSearchForm builds the configuration form shown when the
// filesystem scanner is picked from the scanner list: a directory to search,
// a depth to descend below it, and a checkbox that makes the depth unlimited.
// The Search button runs the scan; it stays disabled until a directory has
// been entered and the depth is valid (a number >= 0, or unlimited).
func (this *QGUI) newFilesystemSearchForm() *fyne.Container {
	this.searchDirectoryEntry = widget.NewEntry()
	this.searchDirectoryEntry.SetPlaceHolder("Directory to search, eg /home/me/models")

	this.searchDepthEntry = widget.NewEntry()
	this.searchDepthEntry.SetPlaceHolder("Depth (0 = only this directory)")

	this.searchUnlimitedCheck = widget.NewCheck("Unlimited", func(checked bool) {
		this.setFilesystemUnlimited(checked)
	})

	this.searchFilesystemButton = widget.NewButton("Search", func() {
		this.runFilesystemSearch()
	})

	this.searchDirectoryEntry.OnChanged = func(string) {
		this.refreshFilesystemSearchState()
	}
	this.searchDepthEntry.OnChanged = func(string) {
		this.refreshFilesystemSearchState()
	}

	// a friendly default: search only the directory itself until changed
	this.searchDepthEntry.SetText("0")
	this.refreshFilesystemSearchState()

	return container.NewVBox(
		widget.NewLabelWithStyle("Filesystem", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Search a directory for .gguf models. Every directory holding an .gguf file is listed as a model."),
		widget.NewForm(
			widget.NewFormItem("Directory", this.searchDirectoryEntry),
			widget.NewFormItem("Depth", this.searchDepthEntry),
			widget.NewFormItem("", this.searchUnlimitedCheck),
		),
		container.NewHBox(this.searchFilesystemButton),
	)
}

// setFilesystemUnlimited handles the unlimited checkbox: while it is checked
// the depth entry is disabled and shows -1, and the depth entered before
// checking is remembered so it can be restored on unchecking.
func (this *QGUI) setFilesystemUnlimited(checked bool) {
	if checked {
		this.searchDepthLast = this.searchDepthEntry.Text
		this.searchDepthEntry.SetText("-1")
		this.searchDepthEntry.Disable()
	} else {
		this.searchDepthEntry.Enable()
		if this.searchDepthLast != "" && this.searchDepthLast != "-1" {
			this.searchDepthEntry.SetText(this.searchDepthLast)
		} else {
			this.searchDepthEntry.SetText("0")
		}
		this.searchDepthLast = ""
	}
	this.refreshFilesystemSearchState()
}

// filesystemSearchDirectory returns the directory entered in the form,
// trimmed of surrounding whitespace.
func (this *QGUI) filesystemSearchDirectory() string {
	return strings.TrimSpace(this.searchDirectoryEntry.Text)
}

// filesystemSearchDepth returns the form's configured depth and whether it is
// valid: -1 when the unlimited checkbox is checked (the depth entry is
// disabled then), otherwise the entry's value, which must parse as a number
// >= 0.
func (this *QGUI) filesystemSearchDepth() (int, bool) {
	if this.searchUnlimitedCheck.Checked {
		return remoterepository.FilesystemDepthUnlimited, true
	}
	n, err := strconv.Atoi(strings.TrimSpace(this.searchDepthEntry.Text))
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// refreshFilesystemSearchState enables or disables the Search button to match
// whether the form's current values are usable: a directory must be entered
// and the depth must be valid.
func (this *QGUI) refreshFilesystemSearchState() {
	if this.searchFilesystemButton == nil {
		return
	}
	_, depthOK := this.filesystemSearchDepth()
	if this.filesystemSearchDirectory() != "" && depthOK {
		this.searchFilesystemButton.Enable()
	} else {
		this.searchFilesystemButton.Disable()
	}
}

// showFilesystemSearchForm shows the filesystem scanner's configuration form
// in the results area. The form is built the first time the filesystem
// scanner is picked, so it does not sit in the page's widget tree (adding
// hidden entry and button widgets to it) until it is actually needed.
func (this *QGUI) showFilesystemSearchForm() {
	if this.searchFilesystemForm == nil {
		this.searchFilesystemForm = this.newFilesystemSearchForm()
		this.searchResultsStack.Add(this.searchFilesystemForm)
	}
	this.searchOverview.Hide()
	this.searchResultsTable.Hide()
	this.searchFilesystemForm.Show()
	this.searchStatusLabel.Hide()
	this.searchSpinner.Hide()
	// the filesystem scanner has no search box
	this.searchQueryScanner = nil
	this.setSearchQueryBarVisible(false)
}

// runFilesystemSearch configures the filesystem scanner with the form's
// current values and runs its scan.
func (this *QGUI) runFilesystemSearch() {
	scanner := this.QApp.RemoteModels.GetScanner(remoterepository.FilesystemScannerName)
	fs, ok := scanner.(*remoterepository.FilesystemScanner)
	if !ok {
		this.searchStatusLabel.Show()
		this.searchStatusLabel.SetText("Filesystem scanner is not available")
		return
	}
	depth, ok := this.filesystemSearchDepth()
	if !ok {
		return
	}
	fs.Configure(this.filesystemSearchDirectory(), depth)
	this.runModelSearch(fs)
}
