package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/kncept/quesadilla/model"
	"github.com/kncept/quesadilla/model/definitions"
	"github.com/kncept/quesadilla/utils/qenv"
)

// Repository maintains the application's models, along with the scanners
// that find models installed elsewhere. Models are scanned once when the
// repository is created, and the loaded set is retained from then on.
type Repository struct {
	models   []definitions.Model
	scanners model.ScannerRegistry
}

// NewRepository creates a model repository, scanning for available models
// as it is created.
func NewRepository() *Repository {
	repository := &Repository{
		scanners: model.NewScannerRegistry(),
	}
	repository.scan()
	return repository
}

// scan refreshes the repository's view of the installed models.
func (this *Repository) scan() {
	models, err := listAvailableModels()
	if err != nil {
		// A scan failure should not take down the app; fall back to an
		// empty set rather than leaving a partially populated one.
		log.Printf("model repository: scan failed: %v", err)
		this.models = nil
		return
	}
	this.models = models
}

// Models returns the models currently known to the repository.
func (this *Repository) Models() []definitions.Model {
	return this.models
}

// GetModel returns the named model, or nil when absent.
func (this *Repository) GetModel(modelName string) *definitions.Model {
	for i := range this.models {
		if this.models[i].ModelName == modelName {
			return &this.models[i]
		}
	}
	return nil
}

// RemoveModel removes the named model, if present, and rescans.
func (this *Repository) RemoveModel(modelName string) error {
	m := this.GetModel(modelName)
	if m == nil {
		return fmt.Errorf("No such model to remove: %s", modelName)
	}
	modelsDir := qenv.QModelsDirectory(m.ModelType)
	namedModelDirectory := path.Join(modelsDir, modelName)
	if err := os.RemoveAll(namedModelDirectory); err != nil {
		return err
	}
	this.scan()
	return nil
}

// GetScanner returns the named scanner, or nil when absent.
func (this *Repository) GetScanner(scannerName string) definitions.ModelScanner {
	return this.scanners.GetScanner(scannerName)
}

// ScanForModels returns every model found by the registered scanners.
func (this *Repository) ScanForModels() ([]definitions.RemoteModel, error) {
	return this.scanners.ScanForModels()
}

// LinkScannedModel links a model found by a scanner, then rescans.
func (this *Repository) LinkScannedModel(scannedModel *definitions.RemoteModel) error {
	switch scannedModel.ModelType {
	case "gguf":
		modelsDir := qenv.QModelsDirectory("gguf")
		namedModelDirectory := path.Join(modelsDir, scannedModel.ModelName)
		err := os.MkdirAll(namedModelDirectory, 0755)
		if err != nil {
			return err
		}
		// write in the metadata (self) location
		scannedModel.MetadataFile = path.Join(namedModelDirectory, "model.json")

		jsonData, err := json.MarshalIndent(scannedModel, "", " ")
		if err != nil {
			return err
		}
		jsonData = append(jsonData, '\n')
		err = os.WriteFile(scannedModel.MetadataFile, jsonData, 0644)
		if err != nil {
			return err
		}
		this.scan()
		return nil
	}

	return fmt.Errorf("Unknown model type: %s", scannedModel.ModelType)
}

// Unlink removes the named model, then rescans.
func (this *Repository) Unlink(modelName string) error {
	return this.RemoveModel(modelName)
}

func listAvailableModels() ([]definitions.Model, error) {
	models := make([]definitions.Model, 0)
	typedModels, err := listModelsOfType("gguf")
	if err != nil {
		return nil, err
	}
	models = append(models, typedModels...)
	return models, nil
}

func listModelsOfType(modelType string) ([]definitions.Model, error) {
	models := make([]definitions.Model, 0)

	// listing models of type gguf
	modelsDir := qenv.QModelsDirectory("gguf")
	dirEntries, err := os.ReadDir(modelsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, dirEntry := range dirEntries {
		modelDir := path.Join(modelsDir, dirEntry.Name())
		filename := definitions.SingleFileContents(modelDir)
		if filename == "model.json" {
			remoteModel := definitions.RemoteModel{}
			data, err := os.ReadFile(path.Join(modelDir, filename))
			if err != nil {
				return nil, err
			}
			err = json.Unmarshal(data, &remoteModel)
			if err != nil {
				return nil, err
			}
			models = append(models, remoteModel.Model)
		} else if filename != "" {
			models = append(models, definitions.Model{
				ModelName: dirEntry.Name(),
				ModelType: modelType,
				ModelFile: path.Join(modelDir, filename),
			})
		}
	}
	return models, nil
}
