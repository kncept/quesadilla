package localrepository

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"

	"github.com/kncept/quesadilla/model/definitions"
	"github.com/kncept/quesadilla/utils/qenv"
	"github.com/kncept/quesadilla/utils/qhttp"
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

// DownloadModel installs a scanned model into local storage: the model file
// is downloaded when the scanner's reference is a URL, or copied when it is
// a local path (eg a model found by the LocalAI scanner). The file is
// fetched to a temporary location first and moved into place only once it is
// complete, so a failed download never leaves a partial file where a model is
// expected (nor clobbers the model that is already installed, on a
// re-download). The repository rescans afterwards, so the model shows up in
// [Models].
func (this *LocalRepository) DownloadModel(scannedModel *definitions.RemoteModel) error {
	switch scannedModel.ModelType {
	case "gguf":
		modelsDir := qenv.QModelsDirectory("gguf")
		namedModelDirectory := path.Join(modelsDir, scannedModel.ModelName)
		if err := os.MkdirAll(namedModelDirectory, 0755); err != nil {
			return err
		}

		filename := path.Base(scannedModel.ModelFile)
		if filename == "." || filename == string(os.PathSeparator) {
			filename = scannedModel.ModelName + ".gguf"
		}

		tempFile := path.Join(modelsDir, tempFileName(scannedModel.ModelName))
		defer os.Remove(tempFile)
		if isURL(scannedModel.ModelFile) {
			if err := qhttp.DownloadFile(scannedModel.ModelFile, modelsDir, path.Base(tempFile)); err != nil {
				return err
			}
		} else {
			if err := copyFile(scannedModel.ModelFile, tempFile); err != nil {
				return err
			}
		}
		if err := os.Rename(tempFile, path.Join(namedModelDirectory, filename)); err != nil {
			return err
		}
		this.scan()
		return nil
	}

	return fmt.Errorf("Unknown model type: %s", scannedModel.ModelType)
}

// tempFileName returns a single file name for the temporary download of the
// named model. The model name can contain path separators (huggingface ids
// look like "org/name"), so they are replaced to keep it to one file.
func tempFileName(modelName string) string {
	safe := strings.NewReplacer("/", "_", "\\", "_").Replace(modelName)
	return "." + safe + ".download"
}

// isURL reports whether the given path is an http(s) URL rather than a local
// filesystem path.
func isURL(p string) bool {
	return strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://")
}

// copyFile copies the file at src to dst.
func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
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
		if !dirEntry.IsDir() {
			continue // stray files (eg temporary downloads) are not models
		}
		models, err = listModelsIn(modelsDir, dirEntry.Name(), modelType, models)
		if err != nil {
			return nil, err
		}
	}
	return models, nil
}

// listModelsIn lists the models stored under name (relative to modelsDir),
// following nested directories for model names that contain a path separator
// (huggingface ids look like "org/name" and are stored one directory per
// name component). A model is a directory whose single entry is its file:
// model.json for linked models, the model file otherwise.
func listModelsIn(modelsDir, name, modelType string, models []definitions.Model) ([]definitions.Model, error) {
	dirPath := path.Join(modelsDir, name)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	if len(entries) == 1 && !entries[0].IsDir() {
		// this directory holds the model's file
		fileName := entries[0].Name()
		filePath := path.Join(dirPath, fileName)
		if fileName == "model.json" {
			remoteModel := definitions.RemoteModel{}
			data, err := os.ReadFile(filePath)
			if err != nil {
				return nil, err
			}
			err = json.Unmarshal(data, &remoteModel)
			if err != nil {
				return nil, err
			}
			models = append(models, remoteModel.Model)
		} else {
			models = append(models, definitions.Model{
				ModelName: name,
				ModelType: modelType,
				ModelFile: filePath,
			})
		}
		return models, nil
	}

	// not a model directory: its subdirectories may be (one name component
	// can hold several models)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		models, err = listModelsIn(modelsDir, path.Join(name, entry.Name()), modelType, models)
		if err != nil {
			return nil, err
		}
	}
	return models, nil
}
