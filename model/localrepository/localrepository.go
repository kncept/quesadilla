package localrepository

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/kncept/quesadilla/model/definitions"
	"github.com/kncept/quesadilla/utils/qenv"
)

// LocalRepository maintains the application's locally-installed models.
// Models are scanned once when the repository is created, and the loaded set
// is retained from then on. Local models are those stored under
// ~/.quesadilla/models/<modelType>.
type LocalRepository struct {
	models []definitions.Model
}

// NewLocalRepository creates a local model repository, scanning for available
// local models as it is created.
func NewLocalRepository() *LocalRepository {
	repository := &LocalRepository{}
	repository.scan()
	return repository
}

// scan refreshes the repository's view of the installed local models.
func (this *LocalRepository) scan() {
	models, err := listAvailableModels()
	if err != nil {
		// A scan failure should not take down the app; fall back to an
		// empty set rather than leaving a partially populated one.
		log.Printf("local repository: scan failed: %v", err)
		this.models = nil
		return
	}
	this.models = models
}

// Models returns the local models currently known to the repository.
func (this *LocalRepository) Models() []definitions.Model {
	return this.models
}

// GetModel returns the named local model, or nil when absent.
func (this *LocalRepository) GetModel(modelName string) *definitions.Model {
	for i := range this.models {
		if this.models[i].ModelName == modelName {
			return &this.models[i]
		}
	}
	return nil
}

// RemoveModel removes the named local model, if present, and rescans.
func (this *LocalRepository) RemoveModel(modelName string) error {
	m := this.GetModel(modelName)
	if m == nil {
		return fmt.Errorf("No such local model to remove: %s", modelName)
	}
	modelsDir := qenv.QModelsDirectory(m.ModelType)
	namedModelDirectory := path.Join(modelsDir, modelName)
	if err := os.RemoveAll(namedModelDirectory); err != nil {
		return err
	}
	this.scan()
	return nil
}

// LinkScannedModel links a model found by a scanner into local storage,
// then rescans.
func (this *LocalRepository) LinkScannedModel(scannedModel *definitions.RemoteModel) error {
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
