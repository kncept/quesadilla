package gui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/quesadilla/app"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
)

// newTestQAppWithLocalAiModel points HOME at a temporary directory holding
// one localai model, so the LocalAI scanner (a local, hermetic scanner)
// finds a controlled set of models.
func newTestQAppWithLocalAiModel(t *testing.T, modelName string) *app.QApp {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	modelDir := path.Join(home, ".localai", "models", "llama-cpp", "models", modelName)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(modelDir, modelName+".gguf"), []byte("stub"), 0644); err != nil {
		t.Fatal(err)
	}
	return app.New()
}

// fakeSearchScanner is a definitions.ModelScanner with canned results, used
// to test the search screen without touching the network. When its block
// channel is non-nil, ScanForModels waits on it, so a test can observe the
// screen while a scan is in flight. When its get function is non-nil,
// GetModel delegates to it.
type fakeSearchScanner struct {
	name    string
	models  []modelDefinitions.RemoteModel
	err     error
	block   chan struct{}
	scanned int
	get     func(string) *modelDefinitions.RemoteModel
}

func (f *fakeSearchScanner) ScannerName() string { return f.name }

func (f *fakeSearchScanner) ScanForModels() ([]modelDefinitions.RemoteModel, error) {
	f.scanned++
	if f.block != nil {
		<-f.block
	}
	return f.models, f.err
}

func (f *fakeSearchScanner) GetModel(modelName string) *modelDefinitions.RemoteModel {
	if f.get != nil {
		return f.get(modelName)
	}
	return nil
}

var _ modelDefinitions.ModelScanner = (*fakeSearchScanner)(nil)

// scannerRow finds the row of the named item in the search screen's scanner
// list.
func scannerRow(t *testing.T, g *QGUI, name string) int {
	t.Helper()
	for i := 0; i < g.searchScannersList.Length(); i++ {
		row := g.searchScannersList.CreateItem().(*fyne.Container)
		g.searchScannersList.UpdateItem(i, row)
		if row.Objects[0].(*widget.Label).Text == name {
			return i
		}
	}
	t.Fatalf("row %q not in the scanner list", name)
	return -1
}

// waitForSearchFinish blocks until a model search in progress has finished.
func waitForSearchFinish(t *testing.T, g *QGUI) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !g.searchRunning {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the model search to finish")
}

// waitForDownloadFinish blocks until a model download in progress has
// finished (the status label leaves its "Downloading..." state).
func waitForDownloadFinish(t *testing.T, g *QGUI) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !strings.HasPrefix(g.searchStatusLabel.Text, "Downloading ") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the model download to finish")
}

// TestModelSearchPageListsScanners verifies the search screen lists the
// overview plus the available scanners, sorted by name.
func TestModelSearchPageListsScanners(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, newTestQAppWithLocalAiModel(t, "tiny-llama"))
	g.modelSearchPage()

	want := []string{"Overview", "HuggingFace", "LocalAI"}
	if got := g.searchScannersList.Length(); got != len(want) {
		t.Fatalf("rows = %d, want %d", got, len(want))
	}
	for i, w := range want {
		row := g.searchScannersList.CreateItem().(*fyne.Container)
		g.searchScannersList.UpdateItem(i, row)
		if got := row.Objects[0].(*widget.Label).Text; got != w {
			t.Errorf("row %d = %q, want %q", i, got, w)
		}
	}
}

// TestModelSearchScannerColumnWidth verifies the scanner column is at least
// as wide as the longest scanner name, so the names are not clipped.
func TestModelSearchScannerColumnWidth(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, newTestQAppWithLocalAiModel(t, "tiny-llama"))
	page := g.modelSearchPage()

	win := test.NewWindow(page)
	win.Resize(fyne.NewSize(800, 600))

	name := widget.NewLabel("HuggingFace")
	name.Wrapping = fyne.TextWrapOff
	if got, want := g.searchScannersList.Size().Width, name.MinSize().Width; got < want {
		t.Errorf("scanner column width %v < longest name width %v (the names would clip)", got, want)
	}
}

// TestModelSearchOverviewInitiallyShown verifies the screen starts on the
// overview, explaining that the screen is for finding models to install.
func TestModelSearchOverviewInitiallyShown(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, newTestQAppWithLocalAiModel(t, "tiny-llama"))
	g.modelSearchPage()

	if !g.searchOverview.Visible() {
		t.Error("expected the overview to be visible initially")
	}
	if g.searchResultsTable.Visible() {
		t.Error("expected the results table to be hidden initially")
	}
	if g.searchSpinner.Visible() {
		t.Error("expected the spinner to be hidden initially")
	}
	if got := g.searchOverview.Text; got != modelSearchOverviewText {
		t.Errorf("overview text = %q, want the overview explanation", got)
	}
}

// TestModelSearchSelectOverview verifies the overview can be shown again
// after a scan has run.
func TestModelSearchSelectOverview(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, newTestQAppWithLocalAiModel(t, "tiny-llama"))
	fake := &fakeSearchScanner{
		name:   "ZTest",
		models: []modelDefinitions.RemoteModel{{Model: modelDefinitions.Model{ModelName: "org/a"}}},
	}
	g.QApp.RemoteModels.Register(fake)
	g.modelSearchPage()

	// run a scan, so the results (not the overview) are showing
	g.searchScannersList.Select(scannerRow(t, g, "ZTest"))
	waitForSearchFinish(t, g)
	if !g.searchResultsTable.Visible() {
		t.Fatal("expected the results table to be visible after the scan")
	}

	// back to the overview
	g.searchScannersList.Select(scannerRow(t, g, "Overview"))
	if !g.searchOverview.Visible() {
		t.Error("expected the overview to be visible after selecting it")
	}
	if g.searchResultsTable.Visible() {
		t.Error("expected the results table to be hidden after selecting the overview")
	}
}

// TestModelSearchRunScan verifies selecting a scanner runs its scan, shows
// the spinner while it is active, and then lists the results.
func TestModelSearchRunScan(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, newTestQAppWithLocalAiModel(t, "tiny-llama"))

	block := make(chan struct{})
	fake := &fakeSearchScanner{
		name:  "ZTest",
		block: block,
		models: []modelDefinitions.RemoteModel{
			{Model: modelDefinitions.Model{ModelName: "org/a", ModelType: "gguf", ModelFile: "https://x.org/a"}},
			{Model: modelDefinitions.Model{ModelName: "org/b", ModelType: "hf", ModelFile: "https://x.org/b"}},
		},
	}
	g.QApp.RemoteModels.Register(fake)

	g.modelSearchPage()

	g.searchScannersList.Select(scannerRow(t, g, "ZTest"))

	// the scan is in flight: spinner shown, state set, results hidden
	if !g.searchRunning {
		t.Error("expected the search to be marked as running")
	}
	if !g.searchSpinner.Visible() {
		t.Error("expected the spinner to be visible while scanning")
	}
	if got, want := g.searchStatusLabel.Text, "Searching with ZTest..."; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
	if g.searchResultsTable.Visible() {
		t.Error("expected the results table to be hidden while scanning")
	}

	// picking another scanner while the scan is in progress is ignored
	g.searchScannersList.Select(scannerRow(t, g, "LocalAI"))

	// let the scan finish
	close(block)
	waitForSearchFinish(t, g)

	// the ZTest results (not the LocalAI ones) are shown, which proves the
	// selection made while the scan was in progress was ignored
	if fake.scanned != 1 {
		t.Errorf("scanner ran %d times, want 1", fake.scanned)
	}
	if g.searchSpinner.Visible() {
		t.Error("expected the spinner to be hidden after the scan")
	}
	if g.searchRunning {
		t.Error("expected the search to no longer be marked as running")
	}
	if got, want := g.searchStatusLabel.Text, "2 models found"; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
	if g.searchOverview.Visible() {
		t.Error("expected the overview to be hidden after the scan")
	}
	if !g.searchResultsTable.Visible() {
		t.Error("expected the results table to be visible after the scan")
	}
	if rows, cols := g.searchResultsTable.Length(); rows != 2 || cols != len(modelSearchColumns) {
		t.Fatalf("results = %dx%d, want 2x%d", rows, cols, len(modelSearchColumns))
	}
	if button := tableCellButton(g.searchResultsTable, 0, 0); button == nil || button.Text != "Download" {
		t.Errorf("first column of row 0 = %v, want a Download button", tableCellObject(g.searchResultsTable, 0, 0))
	}
	if got := tableCellText(g.searchResultsTable, 0, 1); got != "org/a" {
		t.Errorf("result name = %q, want org/a", got)
	}
	if got := tableCellText(g.searchResultsTable, 1, 2); got != "hf" {
		t.Errorf("result type = %q, want hf", got)
	}
	if got := tableCellText(g.searchResultsTable, 0, 3); got != "https://x.org/a" {
		t.Errorf("result file = %q, want https://x.org/a", got)
	}
}

// TestModelSearchRunScanAgain verifies a finished scan can be re-run by
// picking the same scanner again.
func TestModelSearchRunScanAgain(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, newTestQAppWithLocalAiModel(t, "tiny-llama"))
	fake := &fakeSearchScanner{name: "ZTest"}
	g.QApp.RemoteModels.Register(fake)

	g.modelSearchPage()
	row := scannerRow(t, g, "ZTest")

	g.searchScannersList.Select(row)
	waitForSearchFinish(t, g)
	g.searchScannersList.Select(row)
	waitForSearchFinish(t, g)

	if fake.scanned != 2 {
		t.Errorf("scanner ran %d times, want 2", fake.scanned)
	}
}

// TestModelSearchScanError verifies a failed scan shows the error and falls
// back to the overview.
func TestModelSearchScanError(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, newTestQAppWithLocalAiModel(t, "tiny-llama"))
	fake := &fakeSearchScanner{name: "ZTest", err: errors.New("boom")}
	g.QApp.RemoteModels.Register(fake)

	g.modelSearchPage()

	g.searchScannersList.Select(scannerRow(t, g, "ZTest"))
	waitForSearchFinish(t, g)

	if g.searchSpinner.Visible() {
		t.Error("expected the spinner to be hidden after the scan")
	}
	if got, want := g.searchStatusLabel.Text, "Search failed: boom"; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
	if g.searchResultsTable.Visible() {
		t.Error("expected the results table to be hidden after a failed scan")
	}
	if !g.searchOverview.Visible() {
		t.Error("expected the overview to be visible after a failed scan")
	}
}

// TestModelSearchScanNoResults verifies a scan that finds nothing is
// reported as such.
func TestModelSearchScanNoResults(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, newTestQAppWithLocalAiModel(t, "tiny-llama"))
	fake := &fakeSearchScanner{name: "ZTest"}
	g.QApp.RemoteModels.Register(fake)

	g.modelSearchPage()

	g.searchScannersList.Select(scannerRow(t, g, "ZTest"))
	waitForSearchFinish(t, g)

	if got, want := g.searchStatusLabel.Text, "No models found"; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
	if g.searchResultsTable.Visible() {
		t.Error("expected the results table to be hidden with no results")
	}
	if !g.searchOverview.Visible() {
		t.Error("expected the overview to be visible with no results")
	}
}

// TestModelSearchRunScanLocalAi runs the screen's scan flow against the real
// (local, hermetic) LocalAI scanner end to end.
func TestModelSearchRunScanLocalAi(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, newTestQAppWithLocalAiModel(t, "tiny-llama"))
	g.modelSearchPage()

	g.searchScannersList.Select(scannerRow(t, g, "LocalAI"))
	waitForSearchFinish(t, g)

	if got, want := g.searchStatusLabel.Text, "1 model found"; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
	if !g.searchResultsTable.Visible() {
		t.Fatal("expected the results table to be visible")
	}
	if got := tableCellText(g.searchResultsTable, 0, 1); got != "tiny-llama" {
		t.Errorf("result name = %q, want tiny-llama", got)
	}
	if got := tableCellText(g.searchResultsTable, 0, 2); got != "gguf" {
		t.Errorf("result type = %q, want gguf", got)
	}
}

// TestModelSearchDownloadButtonLabels verifies each result row's first cell
// holds a download button, reading "Download" for models that are not
// installed locally and "Redownload" for models that already are (matched by
// name).
func TestModelSearchDownloadButtonLabels(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, newTestQApp(t, "installed-model"))
	fake := &fakeSearchScanner{
		name: "ZTest",
		models: []modelDefinitions.RemoteModel{
			{Model: modelDefinitions.Model{ModelName: "installed-model", ModelType: "gguf", ModelFile: "https://x.org/installed-model"}},
			{Model: modelDefinitions.Model{ModelName: "org/a", ModelType: "gguf", ModelFile: "https://x.org/org/a"}},
		},
	}
	g.QApp.RemoteModels.Register(fake)
	g.modelSearchPage()

	g.searchScannersList.Select(scannerRow(t, g, "ZTest"))
	waitForSearchFinish(t, g)

	if button := tableCellButton(g.searchResultsTable, 0, 0); button == nil || button.Text != "Redownload" {
		t.Errorf("installed row button = %v, want a Redownload button", tableCellObject(g.searchResultsTable, 0, 0))
	}
	if button := tableCellButton(g.searchResultsTable, 1, 0); button == nil || button.Text != "Download" {
		t.Errorf("uninstalled row button = %v, want a Download button", tableCellObject(g.searchResultsTable, 1, 0))
	}
}

// TestModelSearchDownloadButtonTap verifies a real tap on the rendered
// download button copies the model into local storage, reports the download
// in the status label, and then the button reads "Redownload". This uses the
// real (local, hermetic) LocalAI scanner, whose models are local files, so
// the download is a copy.
func TestModelSearchDownloadButtonTap(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, newTestQAppWithLocalAiModel(t, "tiny-llama"))
	page := g.modelSearchPage()

	win := test.NewWindow(page)
	win.Resize(fyne.NewSize(1000, 600))

	g.searchScannersList.Select(scannerRow(t, g, "LocalAI"))
	waitForSearchFinish(t, g)

	buttons := findButtons(page, nil)
	if len(buttons) != 1 {
		t.Fatalf("found %d buttons in the rendered page, want 1", len(buttons))
	}
	if got := buttons[0].Text; got != "Download" {
		t.Fatalf("button text = %q, want Download", got)
	}

	tapButtonCenter(t, buttons[0])
	waitForDownloadFinish(t, g)

	if got := g.searchStatusLabel.Text; got != "Downloaded tiny-llama" {
		t.Errorf("status = %q, want Downloaded tiny-llama", got)
	}
	if g.QApp.LocalModels.GetModel("tiny-llama") == nil {
		t.Fatal("expected tiny-llama to be installed after the download")
	}
	if button := tableCellButton(g.searchResultsTable, 0, 0); button == nil || button.Text != "Redownload" {
		t.Errorf("button after the download = %v, want Redownload", tableCellObject(g.searchResultsTable, 0, 0))
	}
}

// TestModelSearchDownloadFromURL verifies a real tap on the rendered download
// button fetches the model file over HTTP into local storage, where the
// repository picks it up and the button reads "Redownload".
func TestModelSearchDownloadFromURL(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, newTestQApp(t))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("gguf-bytes"))
	}))
	defer server.Close()

	fake := &fakeSearchScanner{
		name: "ZTest",
		models: []modelDefinitions.RemoteModel{
			{Model: modelDefinitions.Model{ModelName: "org/a", ModelType: "gguf", ModelFile: server.URL}},
		},
		get: func(name string) *modelDefinitions.RemoteModel {
			if name != "org/a" {
				return nil
			}
			return &modelDefinitions.RemoteModel{
				Model: modelDefinitions.Model{
					ModelName: "org/a",
					ModelType: "gguf",
					ModelFile: server.URL + "/a.gguf",
				},
				ScannerName: "ZTest",
			}
		},
	}
	g.QApp.RemoteModels.Register(fake)
	page := g.modelSearchPage()

	win := test.NewWindow(page)
	win.Resize(fyne.NewSize(1000, 600))

	g.searchScannersList.Select(scannerRow(t, g, "ZTest"))
	waitForSearchFinish(t, g)

	buttons := findButtons(page, nil)
	if len(buttons) != 1 {
		t.Fatalf("found %d buttons in the rendered page, want 1", len(buttons))
	}
	tapButtonCenter(t, buttons[0])
	waitForDownloadFinish(t, g)

	if got := g.searchStatusLabel.Text; got != "Downloaded org/a" {
		t.Errorf("status = %q, want Downloaded org/a", got)
	}

	m := g.QApp.LocalModels.GetModel("org/a")
	if m == nil {
		t.Fatal("expected org/a to be installed after the download")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	wantFile := path.Join(home, ".quesadilla", "models", "gguf", "org", "a", "a.gguf")
	if m.ModelFile != wantFile {
		t.Errorf("ModelFile = %q, want %q", m.ModelFile, wantFile)
	}
	data, err := os.ReadFile(wantFile)
	if err != nil {
		t.Fatalf("reading the downloaded file: %v", err)
	}
	if string(data) != "gguf-bytes" {
		t.Errorf("file contents = %q, want gguf-bytes", data)
	}

	if button := tableCellButton(g.searchResultsTable, 0, 0); button == nil || button.Text != "Redownload" {
		t.Errorf("button after the download = %v, want Redownload", tableCellObject(g.searchResultsTable, 0, 0))
	}
}

// TestModelSearchDownloadModelNotFound verifies tapping download on a model
// the scanner can no longer resolve reports the failure and installs nothing.
func TestModelSearchDownloadModelNotFound(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, newTestQApp(t))
	fake := &fakeSearchScanner{
		name: "ZTest",
		models: []modelDefinitions.RemoteModel{
			{Model: modelDefinitions.Model{ModelName: "org/a", ModelType: "gguf", ModelFile: "https://x.org"}},
		},
	}
	g.QApp.RemoteModels.Register(fake)
	page := g.modelSearchPage()

	win := test.NewWindow(page)
	win.Resize(fyne.NewSize(1000, 600))

	g.searchScannersList.Select(scannerRow(t, g, "ZTest"))
	waitForSearchFinish(t, g)

	buttons := findButtons(page, nil)
	if len(buttons) != 1 {
		t.Fatalf("found %d buttons in the rendered page, want 1", len(buttons))
	}
	tapButtonCenter(t, buttons[0])
	waitForDownloadFinish(t, g)

	if got := g.searchStatusLabel.Text; got != "Download failed: ZTest model org/a not found" {
		t.Errorf("status = %q, want the failure reported", got)
	}
	if g.QApp.LocalModels.GetModel("org/a") != nil {
		t.Error("expected nothing to be installed after a failed download")
	}
	if button := tableCellButton(g.searchResultsTable, 0, 0); button == nil || button.Text != "Download" {
		t.Errorf("button after a failed download = %v, want Download", tableCellObject(g.searchResultsTable, 0, 0))
	}
}
