package config

import (
	"os"
	"path"
	"reflect"
	"testing"
)

// TestLoadMissingFileIsEmpty verifies a missing config file is not an error:
// it yields an empty config.
func TestLoadMissingFileIsEmpty(t *testing.T) {
	cfg, err := Load(path.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("Load of a missing file: %v", err)
	}
	if cfg == nil || len(cfg.BackendOrder) != 0 {
		t.Fatalf("missing file = %+v, want an empty config", cfg)
	}
}

// TestSaveLoadRoundTrip verifies Save writes the config in the documented
// shape and Load reads it back unchanged.
func TestSaveLoadRoundTrip(t *testing.T) {
	filePath := path.Join(t.TempDir(), "config.json")
	cfg := &Config{BackendOrder: []BackendOrderEntry{
		{ModelType: "gguf", BackendId: "llama-binary"},
		{ModelType: "gguf", BackendId: "gogguf"},
		{ModelType: "onnx", BackendId: "other"},
	}}
	if err := cfg.Save(filePath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(filePath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got.BackendOrder, cfg.BackendOrder) {
		t.Errorf("round trip = %+v, want %+v", got.BackendOrder, cfg.BackendOrder)
	}
}

// TestSaveCreatesParentDirs verifies Save creates missing parent directories.
func TestSaveCreatesParentDirs(t *testing.T) {
	filePath := path.Join(t.TempDir(), "nested", "dir", "config.json")
	cfg := &Config{BackendOrder: []BackendOrderEntry{{ModelType: "gguf", BackendId: "llama-binary"}}}
	if err := cfg.Save(filePath); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := Load(filePath); err != nil {
		t.Fatalf("Load of the saved file: %v", err)
	}
}

// TestLoadCorruptFile verifies a config file that is not valid JSON is
// reported as an error, not silently treated as empty.
func TestLoadCorruptFile(t *testing.T) {
	filePath := path.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(filePath, []byte("{ not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filePath); err == nil {
		t.Fatal("expected an error loading a corrupt config")
	}
}
