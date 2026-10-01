package model

import (
	"fmt"

	"github.com/kncept/quesadilla/model/definitions"
	"github.com/kncept/quesadilla/model/scanner"
)

var _ ScannerRegistry = (*scannerRegistry)(nil)

type ScannerRegistry interface {
	ScanForModels() ([]definitions.RemoteModel, error)
	GetScanner(scannerName string) definitions.ModelScanner
}

func NewScannerRegistry() ScannerRegistry {
	registry := &scannerRegistry{}
	registry.register(&scanner.LocalAiScanner{})
	return registry
}

type scannerRegistry struct {
	Scanners map[string]definitions.ModelScanner
}

func (this *scannerRegistry) register(scanner definitions.ModelScanner) {
	if this.Scanners == nil {
		this.Scanners = make(map[string]definitions.ModelScanner)
		this.Scanners[scanner.ScannerName()] = scanner
	} else {
		if this.Scanners[scanner.ScannerName()] != nil {
			panic(fmt.Sprintf("Already Registered: %s", scanner.ScannerName()))
		}
		this.Scanners[scanner.ScannerName()] = scanner
	}

}

// GetScanner implements [ScannerRegistry].
func (this *scannerRegistry) GetScanner(scannerName string) definitions.ModelScanner {
	return this.Scanners[scannerName]
}

// ScanForModels implements [ScannerRegistry].
func (this *scannerRegistry) ScanForModels() ([]definitions.RemoteModel, error) {
	models := make([]definitions.RemoteModel, 0)
	for _, scanner := range this.Scanners {
		subModels, err := scanner.ScanForModels()
		if err != nil {
			return nil, err
		}
		models = append(models, subModels...)
	}
	return models, nil
}
