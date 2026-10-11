package gui

import (
	"net/url"
	"os"
	"path"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/kncept/quesadilla/app"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
)

// newTestQAppWithFilesystemModel points HOME at a temporary directory and
// plants a couple of .gguf models under the given root: root/tiny/tiny.gguf
// and root/nested/deep/big.gguf. The filesystem scanner scans them.
func newTestQAppWithFilesystemModel(t *testing.T) (*app.QApp, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	root := path.Join(home, "models")
	tinyDir := path.Join(root, "tiny")
	if err := os.MkdirAll(tinyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(tinyDir, "tiny.gguf"), []byte("stub"), 0644); err != nil {
		t.Fatal(err)
	}
	bigDir := path.Join(root, "nested", "deep")
	if err := os.MkdirAll(bigDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path.Join(bigDir, "big.gguf"), []byte("stub"), 0644); err != nil {
		t.Fatal(err)
	}
	return app.New(), root
}

// TestModelSearchFilesystemShowsForm verifies picking the Filesystem scanner
// shows its configuration form (directory, depth, unlimited and Search) and
// hides the overview.
func TestModelSearchFilesystemShowsForm(t *testing.T) {
	test.NewApp()
	qApp, _ := newTestQAppWithFilesystemModel(t)
	g := CreateGui(nil, qApp)
	g.modelSearchPage()

	row := scannerRow(t, g, "Filesystem")
	g.searchScannersList.Select(row)

	if g.searchFilesystemForm == nil || !g.searchFilesystemForm.Visible() {
		t.Fatal("expected the filesystem form to be visible after picking the scanner")
	}
	if g.searchOverview.Visible() {
		t.Error("expected the overview to be hidden while the form is shown")
	}
	if g.searchResultsTable.Visible() {
		t.Error("expected the results table to be hidden while the form is shown")
	}
	if g.searchFilesystemButton == nil {
		t.Fatal("expected a Search button in the form")
	}
}

// TestModelSearchFilesystemFormValidation verifies the Search button is
// disabled until a directory is entered and the depth is valid (a number >= 0
// or unlimited); the unlimited checkbox disables the depth entry and shows -1,
// then restores the previous depth when unchecked.
func TestModelSearchFilesystemFormValidation(t *testing.T) {
	test.NewApp()
	_, root := newTestQAppWithFilesystemModel(t)
	g := CreateGui(nil, app.New())
	g.modelSearchPage()

	// build the form by picking the scanner
	g.searchScannersList.Select(scannerRow(t, g, "Filesystem"))

	if !g.searchFilesystemButton.Disabled() {
		t.Error("expected Search to be disabled with no directory")
	}

	g.searchDirectoryEntry.SetText(root)
	if g.searchFilesystemButton.Disabled() {
		t.Error("expected Search to be enabled with a directory and a valid depth")
	}

	g.searchDepthEntry.SetText("abc")
	if !g.searchFilesystemButton.Disabled() {
		t.Error("expected Search to be disabled with a non-numeric depth")
	}

	g.searchDepthEntry.SetText("-1")
	if !g.searchFilesystemButton.Disabled() {
		t.Error("expected Search to be disabled with a negative depth")
	}

	g.searchDepthEntry.SetText("2")
	if g.searchFilesystemButton.Disabled() {
		t.Error("expected Search to be enabled with a depth of 2")
	}

	// unlimited: the depth is disabled and shows -1
	g.searchUnlimitedCheck.SetChecked(true)
	if g.searchDepthEntry.Text != "-1" {
		t.Errorf("depth under unlimited = %q, want -1", g.searchDepthEntry.Text)
	}
	if !g.searchDepthEntry.Disabled() {
		t.Error("expected the depth entry to be disabled under unlimited")
	}
	if g.searchFilesystemButton.Disabled() {
		t.Error("expected Search to stay enabled under unlimited")
	}
	if depth, ok := g.filesystemSearchDepth(); !ok || depth != -1 {
		t.Errorf("filesystemSearchDepth under unlimited = %d, %v; want -1, true", depth, ok)
	}

	// unchecking re-enables the depth entry and restores the previous depth
	g.searchUnlimitedCheck.SetChecked(false)
	if g.searchDepthEntry.Disabled() {
		t.Error("expected the depth entry to be re-enabled after unchecking unlimited")
	}
	if g.searchDepthEntry.Text != "2" {
		t.Errorf("depth after unchecking = %q, want the previous 2", g.searchDepthEntry.Text)
	}
}

// TestModelSearchFilesystemRunSearch verifies an end-to-end filesystem scan:
// picking the scanner, entering a directory and depth, hitting Search, and
// listing the found models with Link and Import buttons on each row.
func TestModelSearchFilesystemRunSearch(t *testing.T) {
	test.NewApp()
	_, root := newTestQAppWithFilesystemModel(t)
	g := CreateGui(nil, app.New())
	page := g.modelSearchPage()

	win := test.NewWindow(page)
	win.Resize(fyne.NewSize(1200, 700))

	g.searchScannersList.Select(scannerRow(t, g, "Filesystem"))
	g.searchDirectoryEntry.SetText(root)
	g.searchDepthEntry.SetText("1")
	tapButtonCenter(t, g.searchFilesystemButton)
	waitForSearchFinish(t, g)

	if got, want := g.searchStatusLabel.Text, "1 model found"; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
	if !g.searchResultsTable.Visible() {
		t.Fatal("expected the results table to be visible after the search")
	}
	if got := tableCellText(g.searchResultsTable, 0, searchResultNameColumn); got != "tiny" {
		t.Errorf("result name = %q, want tiny", got)
	}
	buttons := tableCellButtons(g.searchResultsTable, 0, searchResultActionColumn)
	if findButtonByText(buttons, "Import") == nil {
		t.Error("expected an Import button on the filesystem result row")
	}
	if findButtonByText(buttons, "Link") == nil {
		t.Error("expected a Link button on the filesystem result row")
	}
	if findButtonByText(buttons, "Download") != nil {
		t.Error("did not expect a Download button on a filesystem result row")
	}
}

// TestModelSearchFilesystemImport verifies a tap on a filesystem result's
// Import button copies the model's files into local storage.
func TestModelSearchFilesystemImport(t *testing.T) {
	test.NewApp()
	_, root := newTestQAppWithFilesystemModel(t)
	g := CreateGui(nil, app.New())
	page := g.modelSearchPage()

	win := test.NewWindow(page)
	win.Resize(fyne.NewSize(1200, 700))

	g.searchScannersList.Select(scannerRow(t, g, "Filesystem"))
	g.searchDirectoryEntry.SetText(root)
	g.searchDepthEntry.SetText("1")
	tapButtonCenter(t, g.searchFilesystemButton)
	waitForSearchFinish(t, g)

	buttons := findButtons(page, nil)
	importButton := findButtonByText(buttons, "Import")
	if importButton == nil {
		t.Fatal("expected an Import button on the rendered row")
	}
	tapButtonCenter(t, importButton)
	waitForDownloadFinish(t, g)

	if got, want := g.searchStatusLabel.Text, "Imported tiny"; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
	m := g.QApp.LocalModels.GetModel("tiny")
	if m == nil {
		t.Fatal("expected tiny to be installed after the import")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	wantFile := path.Join(home, ".quesadilla", "models", "gguf", "none", "tiny", "tiny.gguf")
	if m.ModelFile != wantFile {
		t.Errorf("ModelFile = %q, want %q", m.ModelFile, wantFile)
	}
	if _, err := os.Stat(wantFile); err != nil {
		t.Errorf("the imported file: %v", err)
	}
}

// TestModelSearchFilesystemLink verifies a tap on a filesystem result's Link
// button links the model into local storage without moving its file.
func TestModelSearchFilesystemLink(t *testing.T) {
	test.NewApp()
	_, root := newTestQAppWithFilesystemModel(t)
	g := CreateGui(nil, app.New())
	page := g.modelSearchPage()

	win := test.NewWindow(page)
	win.Resize(fyne.NewSize(1200, 700))

	srcFile := path.Join(root, "tiny", "tiny.gguf")

	g.searchScannersList.Select(scannerRow(t, g, "Filesystem"))
	g.searchDirectoryEntry.SetText(root)
	g.searchDepthEntry.SetText("1")
	tapButtonCenter(t, g.searchFilesystemButton)
	waitForSearchFinish(t, g)

	buttons := findButtons(page, nil)
	linkButton := findButtonByText(buttons, "Link")
	if linkButton == nil {
		t.Fatal("expected a Link button on the rendered row")
	}
	tapButtonCenter(t, linkButton)
	waitForDownloadFinish(t, g)

	if got, want := g.searchStatusLabel.Text, "Linked tiny"; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
	m := g.QApp.LocalModels.GetModel("tiny")
	if m == nil {
		t.Fatal("expected tiny to be linked after the action")
	}
	if m.ModelFile != srcFile {
		t.Errorf("ModelFile = %q, want the linked source %q", m.ModelFile, srcFile)
	}
	if len(m.Variants) != 1 || !m.Variants[0].LinkedModel {
		t.Errorf("expected a single linked variant, got %+v", m.Variants)
	}
}

// TestModelSearchFilesystemRunSearchError verifies a failed filesystem scan
// (a directory that does not exist) falls back to the overview and reports
// the error.
func TestModelSearchFilesystemRunSearchError(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, app.New())
	g.modelSearchPage()

	g.searchScannersList.Select(scannerRow(t, g, "Filesystem"))
	g.searchDirectoryEntry.SetText(path.Join(t.TempDir(), "missing"))
	g.searchDepthEntry.SetText("0")
	g.runFilesystemSearch()
	waitForSearchFinish(t, g)

	if !strings.HasPrefix(g.searchStatusLabel.Text, "Search failed:") {
		t.Errorf("status = %q, want a failure", g.searchStatusLabel.Text)
	}
	if g.searchResultsTable.Visible() {
		t.Error("expected the results table to stay hidden after a failed search")
	}
}

// TestModelSearchDownloadButtonOpensModelCard verifies a remote model with a
// known page shows an Open button to the right of its download button, and a
// tap on it opens the model's page (routed through the openURL hook).
func TestModelSearchDownloadButtonOpensModelCard(t *testing.T) {
	test.NewApp()
	g := CreateGui(nil, newTestQApp(t))
	opened := ""
	g.openURL = func(u *url.URL) error {
		opened = u.String()
		return nil
	}
	fake := &fakeSearchScanner{
		name: "ZTest",
		models: []modelDefinitions.RemoteModel{
			{
				ModelFamily: "org", ModelName: "a", ModelType: "gguf",
				ModelURL: "https://huggingface.co/org/a",
				Variants: []modelDefinitions.ModelVariant{{FileName: "a.gguf", Location: "https://x.org/a"}},
			},
			{
				ModelFamily: "org", ModelName: "bare", ModelType: "gguf",
				Variants: []modelDefinitions.ModelVariant{{FileName: "bare.gguf", Location: "https://x.org/bare"}},
			},
		},
	}
	g.QApp.RemoteModels.Register(fake)
	page := g.modelSearchPage()

	win := test.NewWindow(page)
	win.Resize(fyne.NewSize(1000, 600))

	g.searchScannersList.Select(scannerRow(t, g, "ZTest"))
	waitForSearchFinish(t, g)

	// row 0 has ModelURL: its actions are Download then Open; row 1 has no
	// ModelURL, so only Download.
	buttons := tableCellButtons(g.searchResultsTable, 0, searchResultActionColumn)
	if len(buttons) != 2 {
		t.Fatalf("row 0 buttons = %d, want 2 (Download and Open)", len(buttons))
	}
	if buttons[0].Text != "Download" {
		t.Errorf("row 0 first button = %q, want Download", buttons[0].Text)
	}
	if buttons[1].Text != "Open" {
		t.Errorf("row 0 second button = %q, want Open", buttons[1].Text)
	}
	row1 := tableCellButtons(g.searchResultsTable, 1, searchResultActionColumn)
	if len(row1) != 1 || row1[0].Text != "Download" {
		t.Errorf("row 1 buttons = %v, want a single Download", row1)
	}

	// a real tap on the rendered Open button opens the model card
	buttons = findButtons(page, nil)
	openButton := findButtonByText(buttons, "Open")
	if openButton == nil {
		t.Fatal("expected an Open button in the rendered page")
	}
	tapButtonCenter(t, openButton)
	if opened != "https://huggingface.co/org/a" {
		t.Errorf("opened URL = %q, want the model card URL", opened)
	}
}
