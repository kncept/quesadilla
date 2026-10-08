package remoterepository

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kncept/quesadilla/model/definitions"
)

// RemoteRepository maintains the application's remote model sources: the
// scanners that discover models installed elsewhere (such as LocalAI) or
// published on a model hub (such as HuggingFace).
type RemoteRepository struct {
	scanners map[string]definitions.ModelScanner
}

// NewRemoteRepository creates a remote model repository with the standard
// set of scanners registered.
func NewRemoteRepository() *RemoteRepository {
	repository := &RemoteRepository{}
	repository.register(NewLocalAiScanner())
	repository.register(NewHuggingFaceScanner())
	return repository
}

// Register adds a scanner to the repository, panicking if a scanner with the
// same name is already registered.
func (this *RemoteRepository) Register(scanner definitions.ModelScanner) {
	this.register(scanner)
}

// register adds a scanner to the repository's map, panicking if a scanner
// with the same name is already registered.
func (this *RemoteRepository) register(scanner definitions.ModelScanner) {
	if this.scanners == nil {
		this.scanners = make(map[string]definitions.ModelScanner)
	}
	if this.scanners[scanner.ScannerName()] != nil {
		panic(fmt.Sprintf("Already Registered: %s", scanner.ScannerName()))
	}
	this.scanners[scanner.ScannerName()] = scanner
}

// Scanners returns the scanners registered with the repository, sorted by
// name.
func (this *RemoteRepository) Scanners() []definitions.ModelScanner {
	scanners := make([]definitions.ModelScanner, 0, len(this.scanners))
	for _, scanner := range this.scanners {
		scanners = append(scanners, scanner)
	}
	slices.SortFunc(scanners, func(a, b definitions.ModelScanner) int {
		return strings.Compare(a.ScannerName(), b.ScannerName())
	})
	return scanners
}

// GetScanner returns the named scanner, or nil when no such scanner is
// registered.
func (this *RemoteRepository) GetScanner(scannerName string) definitions.ModelScanner {
	return this.scanners[scannerName]
}
