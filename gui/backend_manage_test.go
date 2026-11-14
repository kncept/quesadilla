package gui

import (
	"slices"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/kncept/quesadilla/app"
	backendDefinitions "github.com/kncept/quesadilla/backend/definitions"
)

// fakeManageableBackend is a fakeBackend that opts in to version management
// and records the versions installed and removed through it.
type fakeManageableBackend struct {
	fakeBackend

	mu                sync.Mutex
	latestVersion     string
	availableVersions []string
	installedVersions []string
	installCalls      []string
	removeCalls       []string
	installErr        error
}

func (f *fakeManageableBackend) Manageable() {}

func (f *fakeManageableBackend) LatestVersion() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.latestVersion
}

func (f *fakeManageableBackend) InstallableVersions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installableCalls++
	return slices.Clone(f.availableVersions)
}

func (f *fakeManageableBackend) InstalledVersions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.installedVersions)
}

func (f *fakeManageableBackend) InstallVersion(version string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installCalls = append(f.installCalls, version)
	if f.installErr != nil {
		return f.installErr
	}
	if !slices.Contains(f.installedVersions, version) {
		f.installedVersions = append(f.installedVersions, version)
	}
	return nil
}

func (f *fakeManageableBackend) RemoveVersion(version string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removeCalls = append(f.removeCalls, version)
	f.installedVersions = slices.DeleteFunc(f.installedVersions, func(v string) bool {
		return v == version
	})
	return nil
}

func (f *fakeManageableBackend) calls() (installed []string, removed []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.installCalls), slices.Clone(f.removeCalls)
}

func (f *fakeManageableBackend) installed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.installedVersions)
}

var _ backendDefinitions.ManageableBackend = (*fakeManageableBackend)(nil)

func newFakeManageableBackend() *fakeManageableBackend {
	return &fakeManageableBackend{
		fakeBackend: fakeBackend{
			id:         "llama-binary",
			modelTypes: []string{"gguf"},
		},
		latestVersion:     "b3",
		availableVersions: []string{"b3", "b2", "b1"},
		installedVersions: []string{"b1"},
	}
}

// waitForBackendManager polls until cond is true, or the test times out.
func waitForBackendManager(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the backend manager")
}

// TestBackendsTableManageButton verifies only backends that opt in to runtime
// version management get a Manage button.
func TestBackendsTableManageButton(t *testing.T) {
	test.NewApp()

	manageable := newFakeManageableBackend()
	plain := &fakeBackend{id: "gogguf", modelTypes: []string{"gguf"}}
	g, _ := newBackendsGui(t, []backendDefinitions.Backend{manageable, plain})
	table := g.newBackendsTable()
	actionCol := len(backendColumns) - 1

	if findButtonByText(tableCellButtons(table, 0, actionCol), "Manage") == nil {
		t.Error("expected a Manage button for the manageable backend")
	}
	if findButtonByText(tableCellButtons(table, 1, actionCol), "Manage") != nil {
		t.Error("did not expect a Manage button for a backend with built-in versions")
	}
}

// TestShowBackendManagerIsTracked verifies opening a backend's manager twice
// focuses the same window instead of stacking up duplicates, and that closing
// it lets the next open create a fresh one.
func TestShowBackendManagerIsTracked(t *testing.T) {
	test.NewApp()
	backend := newFakeManageableBackend()
	g := CreateGui(nil, &app.QApp{})
	g.app = test.NewApp()

	first := g.showBackendManager(backend)
	if first == nil {
		t.Fatal("expected a Manage window to be created")
	}
	if got, want := first.Title(), "Manage llama-binary"; got != want {
		t.Fatalf("window title = %q, want %q", got, want)
	}
	firstManager := g.manageManagers[backend.Id()]
	waitForBackendManager(t, func() bool { return firstManager != nil && !firstManager.fetchingAvailable })

	second := g.showBackendManager(backend)
	if second != first {
		t.Fatal("expected the existing Manage window to be reused")
	}

	first.Close()
	third := g.showBackendManager(backend)
	if third == nil {
		t.Fatal("expected a new Manage window after the previous one closed")
	}
	if third == first {
		t.Fatal("expected a fresh window, got the closed one")
	}
	thirdManager := g.manageManagers[backend.Id()]
	waitForBackendManager(t, func() bool { return thirdManager != nil && !thirdManager.fetchingAvailable })
}

// TestBackendManagerListsVersions verifies the Manage window shows the
// backend's installed versions and the versions available to install.
func TestBackendManagerListsVersions(t *testing.T) {
	test.NewApp()
	backend := newFakeManageableBackend()
	g := CreateGui(nil, &app.QApp{})
	g.app = test.NewApp()

	win := g.app.NewWindow("test")
	content, manager := newBackendManager(g, backend, win, nil)
	win.SetContent(content)
	manager.refreshAvailable()

	if got := manager.installed; !slices.Equal(got, []string{"b1"}) {
		t.Fatalf("installed versions = %v, want [b1]", got)
	}

	waitForBackendManager(t, func() bool {
		return !manager.fetchingAvailable && len(manager.available) == 3
	})
	if got := manager.available; !slices.Equal(got, []string{"b3", "b2", "b1"}) {
		t.Fatalf("available versions = %v, want [b3 b2 b1]", got)
	}

	// The available table's B1 row is already installed, so it offers
	// Reinstall; B2 is not, so it offers Install.
	cell := manager.availableTable.CreateCell()
	manager.availableTable.UpdateCell(widget.TableCellID{Row: 2, Col: 1}, cell)
	if button := findButtonByText(findButtons(cell, nil), "Reinstall"); button == nil {
		t.Error("expected an installed available version to offer Reinstall")
	}
	cell = manager.availableTable.CreateCell()
	manager.availableTable.UpdateCell(widget.TableCellID{Row: 1, Col: 1}, cell)
	if button := findButtonByText(findButtons(cell, nil), "Install"); button == nil {
		t.Error("expected an uninstalled available version to offer Install")
	}
}

// TestBackendManagerInstallAndRemove verifies installing and removing
// versions through the manager calls the backend and refreshes its lists.
func TestBackendManagerInstallAndRemove(t *testing.T) {
	test.NewApp()
	backend := newFakeManageableBackend()
	g := CreateGui(nil, &app.QApp{})
	g.app = test.NewApp()

	win := g.app.NewWindow("test")
	_, manager := newBackendManager(g, backend, win, nil)

	manager.installVersion("b2")
	waitForBackendManager(t, func() bool {
		return slices.Contains(backend.installed(), "b2")
	})
	waitForActiveJobs(t, g, 0)
	if got := backend.installed(); !slices.Equal(got, []string{"b1", "b2"}) {
		t.Fatalf("installed after install = %v, want [b1 b2]", got)
	}

	manager.removeVersion("b1")
	if got := backend.installed(); !slices.Equal(got, []string{"b2"}) {
		t.Fatalf("installed after remove = %v, want [b2]", got)
	}

	installCalls, removeCalls := backend.calls()
	if !slices.Equal(installCalls, []string{"b2"}) {
		t.Errorf("install calls = %v, want [b2]", installCalls)
	}
	if !slices.Equal(removeCalls, []string{"b1"}) {
		t.Errorf("remove calls = %v, want [b1]", removeCalls)
	}
}

// TestBackendManagerInstallLatest verifies the Install Latest action resolves
// the backend's latest version and installs it.
func TestBackendManagerInstallLatest(t *testing.T) {
	test.NewApp()
	backend := newFakeManageableBackend()
	g := CreateGui(nil, &app.QApp{})
	g.app = test.NewApp()

	win := g.app.NewWindow("test")
	_, manager := newBackendManager(g, backend, win, nil)

	manager.installLatestVersion()
	waitForBackendManager(t, func() bool {
		return slices.Contains(backend.installed(), "b3")
	})
	waitForActiveJobs(t, g, 0)

	installCalls, _ := backend.calls()
	if !slices.Contains(installCalls, "b3") {
		t.Errorf("install calls = %v, want b3 installed", installCalls)
	}
}

// TestBackendManagerShowsActions verifies the window offers the install-latest
// and refresh actions.
func TestBackendManagerShowsActions(t *testing.T) {
	test.NewApp()
	backend := newFakeManageableBackend()
	g := CreateGui(nil, &app.QApp{})
	g.app = test.NewApp()

	win := g.app.NewWindow("test")
	content, _ := newBackendManager(g, backend, win, nil)

	buttons := findButtons(content, nil)
	if findButtonByText(buttons, "Install Latest") == nil {
		t.Error("expected an Install Latest button")
	}
	if findButtonByText(buttons, "Refresh") == nil {
		t.Error("expected a Refresh button")
	}
	if got := findLabels(content, nil); len(got) == 0 {
		t.Error("expected headings and summaries in the Manage window")
	}
}
