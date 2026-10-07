package remoterepository

import (
	"errors"
	"os"
	"path"
	"strings"

	"github.com/kncept/quesadilla/model/definitions"
)

// LocalAiScanner scans for models installed via localai.
// This is considered a remote repository since it discovers models
// from ~/.localai/models/llama-cpp/models/ rather than the local
// quesadilla model directory.
var _ definitions.ModelScanner = (*LocalAiScanner)(nil)

type LocalAiScanner struct{}

// NewLocalAiScanner creates a new LocalAiScanner for discovering remote models.
func NewLocalAiScanner() *LocalAiScanner {
	return &LocalAiScanner{}
}

// ScannerName implements [definitions.ModelScanner].
func (this *LocalAiScanner) ScannerName() string {
	return "LocalAI"
}

// GetModel implements [definitions.ModelScanner].
func (this *LocalAiScanner) GetModel(modelName string) *definitions.RemoteModel {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	llamaCppModelsDirectory := path.Join(homeDir, ".localai", "models", "llama-cpp", "models")
	modelDir := path.Join(llamaCppModelsDirectory, modelName)
	modelFile := definitions.SingleFileContents(modelDir)
	if modelFile != "" && strings.HasSuffix(modelFile, ".gguf") {
		return &definitions.RemoteModel{
			ModelName:    modelName,
			ModelType:    "gguf",
			ModelFile:    path.Join(modelDir, modelFile),
			ScannerName:  this.ScannerName(),
			MetadataFile: "", // not from a metadata file, so leave it empty
		}
	}
	return nil
}

// ScanForModels implements [definitions.ModelScanner].
func (this *LocalAiScanner) ScanForModels() ([]definitions.RemoteModel, error) {
	models := make([]definitions.RemoteModel, 0)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	llamaCppModelsDirectory := path.Join(homeDir, ".localai", "models", "llama-cpp", "models")
	dirEntries, err := os.ReadDir(llamaCppModelsDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, dir := range dirEntries {
		modelDir := path.Join(llamaCppModelsDirectory, dir.Name())
		modelFile := definitions.SingleFileContents(modelDir)
		if modelFile != "" && strings.HasSuffix(modelFile, ".gguf") {
			models = append(models, definitions.RemoteModel{
				ModelName:    dir.Name(),
				ModelType:    "gguf",
				ModelFile:    path.Join(modelDir, modelFile),
				ScannerName:  this.ScannerName(),
				MetadataFile: "",
			})
		}
	}
	return models, nil
}