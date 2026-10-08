package remoterepository

import (
	"strings"
	"testing"
)

// TestRemoteRepositoryScannerNames verifies every registered scanner is
// stored under its own name, and that scanner names contain no dash (the
// scanner name prefixes the model name in a link id, which is parsed on the
// first dash).
func TestRemoteRepositoryScannerNames(t *testing.T) {
	repository := NewRemoteRepository()
	for scannerName, scanner := range repository.scanners {
		if scannerName != scanner.ScannerName() {
			t.Errorf("Scanner name mismatch: %s != %s", scannerName, scanner.ScannerName())
		}
		if strings.Contains(scannerName, "-") {
			t.Errorf("Scanner name cannot include a dash: %s", scannerName)
		}
	}
}

// TestRemoteRepositoryGetScanner verifies the standard scanners are
// registered, and that unknown names come back as nil.
func TestRemoteRepositoryGetScanner(t *testing.T) {
	repository := NewRemoteRepository()
	for _, name := range []string{"LocalAI", "HuggingFace"} {
		if repository.GetScanner(name) == nil {
			t.Errorf("expected the %s scanner to be registered", name)
		}
	}
	if repository.GetScanner("NoSuchScanner") != nil {
		t.Error("expected nil for an unregistered scanner")
	}
}

// TestRemoteRepositoryScanners verifies Scanners returns every registered
// scanner, sorted by name.
func TestRemoteRepositoryScanners(t *testing.T) {
	scanners := NewRemoteRepository().Scanners()
	var names []string
	for _, scanner := range scanners {
		names = append(names, scanner.ScannerName())
	}
	want := []string{"HuggingFace", "LocalAI"}
	if len(names) != len(want) {
		t.Fatalf("scanners = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("scanner %d = %q, want %q", i, names[i], want[i])
		}
	}
}
