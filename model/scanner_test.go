package model

import (
	"strings"
	"testing"
)

func TestScannerNames(t *testing.T) {
	scannerRegistry := NewScannerRegistry().(*scannerRegistry)
	for scannerName, scanner := range scannerRegistry.Scanners {
		if scannerName != scanner.ScannerName() {
			t.Errorf("Scanner name mismatch: %s != %s", scannerName, scanner.ScannerName())
		}
		if strings.Contains(scannerName, "-") {
			t.Errorf("Scanner name cannot include a dash: %s", scannerName)
		}
	}
}
