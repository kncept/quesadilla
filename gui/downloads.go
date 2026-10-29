package gui

import (
	"fmt"
	"path"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/quesadilla/utils/qhttp"
)

// downloadRequest describes one download started with [QGUI.download].
type downloadRequest struct {
	// Name is what the download is called on the Downloads screen.
	Name string
	// URL is the source to download from.
	URL string
	// Destination is the file to write the download to.
	Destination string
	// OnComplete runs after the download finishes. It receives the
	// download's error, or nil when it succeeded.
	OnComplete func(error)
}

// activeDownload is one download (or multi-file download job) currently in
// progress. It is what the Downloads screen lists.
type activeDownload struct {
	Name        string
	Source      string
	Destination string
	Started     time.Time
}

// downloadColumns are the table headers shown on the Downloads screen.
var downloadColumns = []string{"Name", "Source", "Destination", "Elapsed"}

// downloadsPage renders the Downloads screen: every download and download job
// currently in progress. The table is refreshed by the status ticker, so
// elapsed times keep counting up and finished downloads disappear.
func (this *QGUI) downloadsPage() fyne.CanvasObject {
	header := container.NewVBox(
		headingLabel("Downloads"),
		wrappedLabel("Downloads and download jobs currently in progress."),
		widget.NewSeparator(),
	)

	table := widget.NewTable(
		func() (int, int) {
			return len(this.activeDownloads()), len(downloadColumns)
		},
		downloadsCellFactory,
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			downloads := this.activeDownloads()
			if id.Row < 0 || id.Row >= len(downloads) {
				return
			}
			this.updateDownloadCell(downloads[id.Row], id, cell.(*fyne.Container))
		},
	)

	table.ShowHeaderRow = true
	table.CreateHeader = func() fyne.CanvasObject {
		return widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}
	table.UpdateHeader = func(id widget.TableCellID, cell fyne.CanvasObject) {
		if id.Col >= 0 && id.Col < len(downloadColumns) {
			cell.(*widget.Label).SetText(downloadColumns[id.Col])
		}
	}

	table.SetColumnWidth(0, 220) // Name
	table.SetColumnWidth(1, 280) // Source
	table.SetColumnWidth(2, 300) // Destination
	table.SetColumnWidth(3, 90)  // Elapsed

	empty := widget.NewLabel("No downloads in progress")
	this.downloadsTable = table
	this.noDownloadsLabel = empty
	this.updateDownloadsVisibility()
	return container.NewPadded(container.NewBorder(header, nil, nil, nil, container.NewStack(table, empty)))
}

// downloadsCellFactory builds a cell of the Downloads table. Like the other
// tables it needs a real layout, so the cell is a container rather than a bare
// &fyne.Container{}.
func downloadsCellFactory() fyne.CanvasObject {
	return container.NewHBox(widget.NewLabel("Download"))
}

// updateDownloadCell fills one cell of the Downloads table.
func (this *QGUI) updateDownloadCell(d activeDownload, id widget.TableCellID, cell *fyne.Container) {
	cell.RemoveAll()
	label := widget.NewLabel(downloadCell(d, id.Col))
	label.Wrapping = fyne.TextWrapOff
	cell.Add(label)
	cell.Refresh()
}

// downloadCell returns the display value for one column of the Downloads
// table.
func downloadCell(d activeDownload, col int) string {
	switch col {
	case 0:
		return d.Name
	case 1:
		return d.Source
	case 2:
		return d.Destination
	case 3:
		return formatUptime(time.Since(d.Started))
	default:
		return ""
	}
}

// updateDownloadsVisibility shows the table while downloads are active, and
// the placeholder otherwise.
func (this *QGUI) updateDownloadsVisibility() {
	hasDownloads := len(this.activeDownloads()) > 0
	if this.downloadsTable != nil {
		if hasDownloads {
			this.downloadsTable.Show()
		} else {
			this.downloadsTable.Hide()
		}
	}
	if this.noDownloadsLabel != nil {
		if hasDownloads {
			this.noDownloadsLabel.Hide()
		} else {
			this.noDownloadsLabel.Show()
		}
	}
}

// download starts downloading request.URL to request.Destination in the
// background. The download is listed on the Downloads screen while it runs;
// when it finishes its completion function is called, and the download is
// then removed from the list.
func (this *QGUI) download(request downloadRequest) {
	download := this.addDownload(request.Name, request.URL, request.Destination)
	go func() {
		defer this.removeDownload(download)

		directory, fileName := path.Split(request.Destination)
		if directory == "" {
			directory = "."
		}
		err := qhttp.DownloadFile(request.URL, directory, fileName)
		if request.OnComplete != nil {
			request.OnComplete(err)
		}
	}()
}

// addDownload records a new download as active and returns it, so its job can
// remove it again once it finishes. It is also used by downloads that are
// handled by the local repository, so every job shows on the Downloads screen.
func (this *QGUI) addDownload(name, source, destination string) *activeDownload {
	download := &activeDownload{
		Name:        name,
		Source:      source,
		Destination: destination,
		Started:     time.Now(),
	}
	this.downloadsMu.Lock()
	this.downloads = append(this.downloads, download)
	this.downloadsMu.Unlock()
	this.refreshDownloads()
	return download
}

// removeDownload drops a finished download from the active list. Removing an
// unknown download is a no-op.
func (this *QGUI) removeDownload(download *activeDownload) {
	if download == nil {
		return
	}
	this.downloadsMu.Lock()
	for i, active := range this.downloads {
		if active == download {
			this.downloads = append(this.downloads[:i], this.downloads[i+1:]...)
			break
		}
	}
	this.downloadsMu.Unlock()
	this.refreshDownloads()
}

// activeDownloads returns a snapshot of the downloads currently in progress.
func (this *QGUI) activeDownloads() []activeDownload {
	this.downloadsMu.Lock()
	defer this.downloadsMu.Unlock()
	downloads := make([]activeDownload, len(this.downloads))
	for i, download := range this.downloads {
		downloads[i] = *download
	}
	return downloads
}

// refreshDownloads re-renders the Downloads table after the active list
// changed. Widgets that are not attached to a window are no-ops.
func (this *QGUI) refreshDownloads() {
	if this.downloadsTable == nil {
		return
	}
	fyne.Do(func() {
		this.downloadsTable.Refresh()
		this.updateDownloadsVisibility()
	})
}

// downloadJobName returns the name shown on the Downloads screen for a model
// download job, including the single variant when one is being fetched.
func downloadJobName(action modelInstallAction, modelName string, variantFile string) string {
	if variantFile != "" {
		return fmt.Sprintf("%s %s (%s)", action.verb, modelName, variantFile)
	}
	return fmt.Sprintf("%s %s", action.verb, modelName)
}
