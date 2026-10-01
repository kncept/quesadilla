package definitions

import (
	"os"
	"path"
)

type ModelType string

// const ModelType_GGUF ModelType = "gguf"

type Model struct {
	ModelName string
	ModelType string // implies the backend type required
	ModelFile string
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

func DirSingleFilePathOrNothing(dir string) string {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	if len(dirEntries) != 1 {
		return ""
	}
	return path.Join(dir, dirEntries[0].Name())
}
