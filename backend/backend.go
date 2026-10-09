package backend

import (
	"log"
	"slices"
	"sync"

	"github.com/kncept/quesadilla/backend/definitions"
	"github.com/kncept/quesadilla/backend/gogguf"
	"github.com/kncept/quesadilla/backend/llama"
	"github.com/kncept/quesadilla/config"
)

// Repository maintains the application's backends. The backends (and their
// runners) are created once, when the repository is created, and shared from
// then on rather than being rebuilt on every lookup. The repository also
// owns the backends' order - the ordered, model-type-grouped list of
// (model type, backend id) pairs kept in the config - which decides how the
// backends screen lists them and which backend is the default for each model
// type (the topmost of its group).
type Repository struct {
	backends []definitions.Backend

	mu           sync.Mutex
	backendOrder []config.BackendOrderEntry
	configPath   string
}

// NewRepository creates the backend repository: the known backends, with
// their order loaded from the default config path.
func NewRepository() *Repository {
	return NewRepositoryWithConfig(defaultBackends(), config.DefaultPath())
}

// NewRepositoryWithConfig creates a backend repository over the given
// backends, with their order loaded from (and saved to) the given config
// path.
func NewRepositoryWithConfig(backends []definitions.Backend, configPath string) *Repository {
	repository := &Repository{
		backends:   backends,
		configPath: configPath,
	}
	repository.backendOrder = repository.loadOrder()
	return repository
}

// defaultBackends is the set of known backends, in scan order. The order
// only decides the defaults: it is the order the backends get in the config
// when there is none yet.
func defaultBackends() []definitions.Backend {
	return []definitions.Backend{
		llama.LlamaBackend(),
		gogguf.GoggufBackend(),
	}
}

// loadOrder builds the effective backend order: the config's order,
// reconciled against the current backends so it never lists a backend (or
// type) that no longer exists and never misses one that appeared. When the
// config is absent or unreadable it falls back to the default order.
func (this *Repository) loadOrder() []config.BackendOrderEntry {
	cfg, err := config.Load(this.configPath)
	if err != nil {
		log.Printf("backend: loading config %s: %v", this.configPath, err)
		return orderFromBackends(this.backends)
	}
	return reconcileOrder(cfg.BackendOrder, this.backends)
}

// orderFromBackends builds the default order: the model types grouped, in
// the order they first appear, and within each type the backends in scan
// order - so the first backend to support a type is that type's default.
func orderFromBackends(backends []definitions.Backend) []config.BackendOrderEntry {
	order := make([]config.BackendOrderEntry, 0, len(backends))
	for _, t := range typesOf(backends) {
		for _, b := range backends {
			if slices.Contains(b.ModelTypes(), t) {
				order = append(order, config.BackendOrderEntry{ModelType: t, BackendId: b.Id()})
			}
		}
	}
	return order
}

// typesOf lists the model types the backends support, in first-appearance
// (scan) order.
func typesOf(backends []definitions.Backend) []string {
	seen := make(map[string]bool)
	var types []string
	for _, b := range backends {
		for _, t := range b.ModelTypes() {
			if !seen[t] {
				seen[t] = true
				types = append(types, t)
			}
		}
	}
	return types
}

// reconcileOrder keeps the order the user saved, and brings it in line with
// the current backends: entries for backends (or types) that no longer exist
// are dropped, duplicates are collapsed, and backends that appeared since are
// appended to the end of their model type's group, in scan order.
func reconcileOrder(order []config.BackendOrderEntry, backends []definitions.Backend) []config.BackendOrderEntry {
	byType := make(map[string][]config.BackendOrderEntry)
	typeOrder := make([]string, 0, len(order))
	seenType := make(map[string]bool)
	seenEntry := make(map[string]bool, len(order))
	for _, e := range order {
		key := e.ModelType + "\x00" + e.BackendId
		b := findBackend(backends, e.BackendId)
		if b == nil || !slices.Contains(b.ModelTypes(), e.ModelType) || seenEntry[key] {
			continue // stale or duplicate entry
		}
		seenEntry[key] = true
		if !seenType[e.ModelType] {
			seenType[e.ModelType] = true
			typeOrder = append(typeOrder, e.ModelType)
		}
		byType[e.ModelType] = append(byType[e.ModelType], e)
	}

	// types that appeared since the order was saved come after, in scan order
	for _, t := range typesOf(backends) {
		if !seenType[t] {
			seenType[t] = true
			typeOrder = append(typeOrder, t)
		}
	}

	reconciled := make([]config.BackendOrderEntry, 0, len(order)+len(backends))
	for _, t := range typeOrder {
		reconciled = append(reconciled, byType[t]...)
		for _, b := range backends {
			if !slices.Contains(b.ModelTypes(), t) {
				continue
			}
			key := t + "\x00" + b.Id()
			if !seenEntry[key] {
				seenEntry[key] = true
				reconciled = append(reconciled, config.BackendOrderEntry{ModelType: t, BackendId: b.Id()})
			}
		}
	}
	return reconciled
}

// Backends returns every known backend.
func (this *Repository) Backends() []definitions.Backend {
	return this.backends
}

// Backend returns the backend with the given id, or nil when absent.
func (this *Repository) Backend(id string) definitions.Backend {
	return findBackend(this.backends, id)
}

// BackendOrder returns a copy of the current (model type, backend id) order.
func (this *Repository) BackendOrder() []config.BackendOrderEntry {
	this.mu.Lock()
	defer this.mu.Unlock()
	return slices.Clone(this.backendOrder)
}

// BackendForModelType returns the default backend for the given model type:
// the topmost backend in the order that supports it and has at least one
// runner installed.
func (this *Repository) BackendForModelType(modelType string) definitions.Backend {
	this.mu.Lock()
	order := this.backendOrder
	this.mu.Unlock()
	for _, e := range order {
		if e.ModelType != modelType {
			continue
		}
		if b := this.Backend(e.BackendId); b != nil && len(b.InstalledVersions()) > 0 {
			return b
		}
	}
	return nil
}

// MoveBackend moves the given backend one step up or down within its model
// type's group - the topmost backend of a group is that type's default - and
// saves the new order to the config. It reports whether the order changed.
func (this *Repository) MoveBackend(modelType, backendId string, up bool) bool {
	this.mu.Lock()
	order := this.backendOrder
	from := -1
	for i, e := range order {
		if e.ModelType == modelType && e.BackendId == backendId {
			from = i
			break
		}
	}
	if from < 0 {
		this.mu.Unlock()
		return false
	}

	to := from
	if up {
		// the group's top: no row of the same type above
		if from == 0 || order[from-1].ModelType != modelType {
			this.mu.Unlock()
			return false
		}
		to = from - 1
	} else {
		// the group's bottom: no row of the same type below
		if from == len(order)-1 || order[from+1].ModelType != modelType {
			this.mu.Unlock()
			return false
		}
		to = from + 1
	}

	order[from], order[to] = order[to], order[from]
	if err := (&config.Config{BackendOrder: order}).Save(this.configPath); err != nil {
		log.Printf("backend: saving config %s: %v", this.configPath, err)
	}
	this.mu.Unlock()
	return true
}

// findBackend returns the backend with the given id, or nil when absent.
func findBackend(backends []definitions.Backend, id string) definitions.Backend {
	for _, b := range backends {
		if b.Id() == id {
			return b
		}
	}
	return nil
}
