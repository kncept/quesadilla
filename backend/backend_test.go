package backend

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kncept/quesadilla/backend/definitions"
	"github.com/kncept/quesadilla/config"
	modelDefinitions "github.com/kncept/quesadilla/model/definitions"
	runnerDefinitions "github.com/kncept/quesadilla/runner/definitions"
)

// testBackend is a definitions.Backend with canned values for the order
// tests.
type testBackend struct {
	id         string
	modelTypes []string
	installed  []string
}

func (b *testBackend) Id() string                    { return b.id }
func (b *testBackend) Description() string           { return b.id }
func (b *testBackend) ModelTypes() []string          { return b.modelTypes }
func (b *testBackend) InstalledVersions() []string   { return b.installed }
func (b *testBackend) InstallableVersions() []string { return nil }
func (b *testBackend) LatestVersion() string         { return "" }
func (b *testBackend) InstallVersion(string) error   { return nil }
func (b *testBackend) RemoveVersion(string) error    { return nil }
func (b *testBackend) Start(*modelDefinitions.Model) (runnerDefinitions.RunningModel, error) {
	return nil, nil
}

var _ definitions.Backend = (*testBackend)(nil)

// newOrderRepository builds a repository over the given backends, with the
// order persisted to a temporary config file.
func newOrderRepository(t *testing.T, backends []definitions.Backend) *Repository {
	t.Helper()
	return NewRepositoryWithConfig(backends, filepath.Join(t.TempDir(), "config.json"))
}

// TestOrderFromBackends verifies the default order: model types grouped in
// first-appearance order, and within each type the backends in scan order.
func TestOrderFromBackends(t *testing.T) {
	backends := []definitions.Backend{
		&testBackend{id: "a", modelTypes: []string{"gguf", "onnx"}},
		&testBackend{id: "b", modelTypes: []string{"gguf"}},
	}

	want := []config.BackendOrderEntry{
		{ModelType: "gguf", BackendId: "a"},
		{ModelType: "gguf", BackendId: "b"},
		{ModelType: "onnx", BackendId: "a"},
	}
	if got := orderFromBackends(backends); !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// TestReconcileOrder verifies a saved order is kept in line with the current
// backends: stale and duplicate entries are dropped, and backends that
// appeared since are appended to the end of their model type's group.
func TestReconcileOrder(t *testing.T) {
	backends := []definitions.Backend{
		&testBackend{id: "a", modelTypes: []string{"gguf", "onnx"}},
		&testBackend{id: "b", modelTypes: []string{"gguf"}},
		&testBackend{id: "c", modelTypes: []string{"gguf"}}, // new since the order was saved
	}

	order := []config.BackendOrderEntry{
		{ModelType: "gguf", BackendId: "b"},
		{ModelType: "gguf", BackendId: "a"},
		{ModelType: "gguf", BackendId: "gone"}, // stale backend
		{ModelType: "old", BackendId: "a"},     // stale model type
		{ModelType: "onnx", BackendId: "a"},
		{ModelType: "gguf", BackendId: "b"}, // duplicate
	}

	want := []config.BackendOrderEntry{
		{ModelType: "gguf", BackendId: "b"},
		{ModelType: "gguf", BackendId: "a"},
		{ModelType: "gguf", BackendId: "c"}, // appended within its group
		{ModelType: "onnx", BackendId: "a"},
	}
	if got := reconcileOrder(order, backends); !reflect.DeepEqual(got, want) {
		t.Errorf("reconciled = %v, want %v", got, want)
	}
}

// TestMoveBackend verifies MoveBackend swaps adjacent rows within a model
// type group only, refuses to move across groups or to unknown rows, and
// leaves the order untouched when nothing moved.
func TestMoveBackend(t *testing.T) {
	repo := newOrderRepository(t, []definitions.Backend{
		&testBackend{id: "a", modelTypes: []string{"gguf", "onnx"}},
		&testBackend{id: "b", modelTypes: []string{"onnx"}},
	})
	// default order: [gguf/a] [onnx/a onnx/b]

	if repo.MoveBackend("gguf", "a", true) {
		t.Error("expected moving the only gguf row up to be refused")
	}
	if repo.MoveBackend("gguf", "a", false) {
		t.Error("expected moving a row across model type groups to be refused")
	}
	if repo.MoveBackend("gguf", "nope", true) {
		t.Error("expected moving an unknown backend to be refused")
	}

	if !repo.MoveBackend("onnx", "b", true) {
		t.Fatal("expected moving b up in the onnx group")
	}
	want := []config.BackendOrderEntry{
		{ModelType: "gguf", BackendId: "a"},
		{ModelType: "onnx", BackendId: "b"},
		{ModelType: "onnx", BackendId: "a"},
	}
	if got := repo.BackendOrder(); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}

	if repo.MoveBackend("onnx", "b", true) {
		t.Error("expected moving the group's top row up to be refused")
	}
}

// TestBackendForModelTypeFollowsOrder verifies the default backend for a
// model type is the topmost one in the order that has a version installed,
// and follows the order when it changes.
func TestBackendForModelTypeFollowsOrder(t *testing.T) {
	a := &testBackend{id: "a", modelTypes: []string{"gguf"}, installed: []string{"v1"}}
	b := &testBackend{id: "b", modelTypes: []string{"gguf"}, installed: []string{"v2"}}
	repo := newOrderRepository(t, []definitions.Backend{a, b})

	if got := repo.BackendForModelType("gguf"); got != a {
		t.Fatalf("default = %v, want a", got.Id())
	}

	if !repo.MoveBackend("gguf", "b", true) {
		t.Fatal("expected moving b to the top of the gguf group")
	}
	if got := repo.BackendForModelType("gguf"); got != b {
		t.Fatalf("default after move = %v, want b", got.Id())
	}
}

// TestBackendForModelTypeSkipsUninstalled verifies a topmost backend without
// an installed version is skipped in favor of the next one.
func TestBackendForModelTypeSkipsUninstalled(t *testing.T) {
	a := &testBackend{id: "a", modelTypes: []string{"gguf"}} // nothing installed
	b := &testBackend{id: "b", modelTypes: []string{"gguf"}, installed: []string{"v2"}}
	repo := newOrderRepository(t, []definitions.Backend{a, b})

	if got := repo.BackendForModelType("gguf"); got != b {
		t.Fatalf("default = %v, want b", got.Id())
	}
	if got := repo.BackendForModelType("onnx"); got != nil {
		t.Fatalf("default for an unsupported type = %v, want nil", got.Id())
	}
}

// TestOrderPersistsAcrossRepositories verifies a moved order is saved to the
// config and comes back when a new repository loads the same path.
func TestOrderPersistsAcrossRepositories(t *testing.T) {
	backends := []definitions.Backend{
		&testBackend{id: "a", modelTypes: []string{"gguf"}},
		&testBackend{id: "b", modelTypes: []string{"gguf"}},
	}
	configPath := filepath.Join(t.TempDir(), "config.json")

	first := NewRepositoryWithConfig(backends, configPath)
	if !first.MoveBackend("gguf", "b", true) {
		t.Fatal("expected moving b up")
	}

	second := NewRepositoryWithConfig(backends, configPath)
	want := []config.BackendOrderEntry{
		{ModelType: "gguf", BackendId: "b"},
		{ModelType: "gguf", BackendId: "a"},
	}
	if got := second.BackendOrder(); !reflect.DeepEqual(got, want) {
		t.Errorf("reloaded order = %v, want %v", got, want)
	}
}
