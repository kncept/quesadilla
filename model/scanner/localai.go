package scanner

import (
	"errors"
	"os"
	"path"
	"strings"

	"github.com/kncept/quesadilla/model/definitions"
)

var _ definitions.ModelScanner = (*LocalAiScanner)(nil)

type LocalAiScanner struct{}

// GetModel implements [definitions.ModelScanner].
func (this *LocalAiScanner) GetModel(modelName string) *definitions.RemoteModel {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	llamaCppModelsDirectory := path.Join(homeDir, ".localai", "models", "llama-cpp", "models")
	modelDir := path.Join(llamaCppModelsDirectory, modelName)
	modelFile := definitions.DirSingleFilePathOrNothing(modelDir)
	if modelFile != "" && strings.HasSuffix(modelFile, ".gguf") {
		return &definitions.RemoteModel{
			ModelName:    modelName,
			ModelType:    "gguf",
			ModelFile:    modelFile,
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
		modelFile := definitions.DirSingleFilePathOrNothing(modelDir)
		if modelFile != "" && strings.HasSuffix(modelFile, ".gguf") {
			models = append(models, definitions.RemoteModel{
				ModelName:    dir.Name(),
				ModelType:    "gguf",
				ModelFile:    modelFile,
				ScannerName:  this.ScannerName(),
				MetadataFile: "",
			})
		}
	}
	return models, nil
}

// ScannerName implements [definitions.ModelScanner].
func (this *LocalAiScanner) ScannerName() string {
	return "LocalAI"
}
