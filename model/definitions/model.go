package definitions

import (
	"os"
)

type ModelType string

// const ModelType_GGUF ModelType = "gguf"

type Model struct {
	ModelName string
	ModelType string // implies the backend type required
	ModelFile string //fully qualified model file
}

type RemoteModel struct {
	Model
	ScannerName  string
	MetadataFile string // basically a 'self reference'
}

type ModelScanner interface {
	ScannerName() string
	ScanForModels() ([]RemoteModel, error)

	// verifies the model exists, then returns the metatadata
	// returns nil if absent or
	GetModel(modelName string) *RemoteModel
}

func SingleFileContents(dir string) string {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	if len(dirEntries) != 1 {
		return ""
	}
	return dirEntries[0].Name()
}
