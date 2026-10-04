package repository

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

func ListAvailableModels() ([]definitions.Model, error) {
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

// eg: link a model from localai
func LinkScannedModel(scannedModel *definitions.RemoteModel) error {
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
			log.Fatal(err)
		}
		jsonData = append(jsonData, '\n')
		err = os.WriteFile(scannedModel.MetadataFile, jsonData, 0644)
		if err != nil {
			log.Fatal(err)
		}
		return nil
	}

	return fmt.Errorf("Unknown model type: %s", scannedModel.ModelType)
}
func GetModel(modelName string) *definitions.Model {
	models, err := ListAvailableModels()
	if err != nil {
		return nil
	}
	for _, m := range models {
		if m.ModelName == modelName {
			return &m
		}
	}
	return nil

}
func RemoveModel(modelName string) error {
	m := GetModel(modelName)
	if m == nil {
		return fmt.Errorf("No such model to remove: %s", modelName)
	}
	modelsDir := qenv.QModelsDirectory(m.ModelType)
	namedModelDirectory := path.Join(modelsDir, modelName)
	return os.RemoveAll(namedModelDirectory)
}

func DownloadModel() error {
	panic("unimplemented")
}
