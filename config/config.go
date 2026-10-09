// Package config holds the application's persisted configuration: the
// ordered list of (model type, backend id) pairs that lays out the backends
// screen and decides which backend is the default for each model type.
package config

import (
	"encoding/json"
	"os"
	"path"

	"github.com/kncept/quesadilla/utils/qenv"
)

// BackendOrderEntry is one row of the backend order: a (model type, backend
// id) pair. A backend that supports several model types appears once per
// type. For each model type, the first entry in the list is that type's
// default backend; the first entry overall is the topmost (default) backend.
type BackendOrderEntry struct {
	ModelType string `json:"modelType"`
	BackendId string `json:"backendId"`
}

// Config is the application's persisted configuration.
type Config struct {
	// BackendOrder is the ordered, model-type-grouped list of backends.
	BackendOrder []BackendOrderEntry `json:"backendOrder"`
}

// DefaultPath is where the config file lives.
func DefaultPath() string {
	return path.Join(qenv.QDir(), "config.json")
}

// Load reads the config from the given path. A missing file is not an error:
// it yields an empty config, so a first run and an absent config are treated
// alike.
func Load(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save writes the config to the given path, creating any missing parent
// directories.
func (c *Config) Save(filePath string) error {
	if err := os.MkdirAll(path.Dir(filePath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, append(data, '\n'), 0644)
}
