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
	modelDir := path.Join(this.modelsRoot(homeDir), modelName)
	return this.scanModelDir(modelDir, modelName)
}

// ScanForModels implements [definitions.ModelScanner].
func (this *LocalAiScanner) ScanForModels() ([]definitions.RemoteModel, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := this.modelsRoot(homeDir)
	dirEntries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	models := make([]definitions.RemoteModel, 0)
	for _, dir := range dirEntries {
		if !dir.IsDir() {
			continue
		}
		if m := this.scanModelDir(path.Join(root, dir.Name()), dir.Name()); m != nil {
			models = append(models, *m)
		}
	}
	return models, nil
}

// modelsRoot returns the directory the scanner looks for models in.
func (this *LocalAiScanner) modelsRoot(homeDir string) string {
	return path.Join(homeDir, ".localai", "models", "llama-cpp", "models")
}

// scanModelDir builds the RemoteModel for the model stored in modelDir:
// every .gguf file in the directory is one of the model's variants. A
// directory without a single one holds no runnable model and yields nil.
func (this *LocalAiScanner) scanModelDir(modelDir, modelName string) *definitions.RemoteModel {
	entries, err := os.ReadDir(modelDir)
	if err != nil {
		return nil
	}

	variants := make([]definitions.ModelVariant, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".gguf") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		variants = append(variants, definitions.ModelVariant{
			FileName: entry.Name(),
			Location: path.Join(modelDir, entry.Name()),
			FileSize: info.Size(),
		})
	}
	if len(variants) == 0 {
		return nil
	}

	return &definitions.RemoteModel{
		ModelFamily: definitions.NoneFamily,
		ModelName:   modelName,
		ModelSource: this.ScannerName(),
		ModelType:   string(definitions.ModelTypeGGUF),
		Variants:    variants,
	}
}
