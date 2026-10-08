package remoterepository

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/kncept/quesadilla/model/definitions"
)

var _ definitions.ModelScanner = (*HuggingFaceScanner)(nil)

// hfBaseURL is the root of the HuggingFace API. It is a variable rather than
// a constant so tests can point the scanner at a local test server.
var hfBaseURL = "https://huggingface.co"

// hfClient is the shared HTTP client for the HuggingFace API.
var hfClient = &http.Client{Timeout: 30 * time.Second}

// hfFilterOptions are the query presets the scanner uses to list models.
// They are hard coded for now; making them selectable is a later step.
var hfFilterOptions = []url.Values{
	{
		// the most downloaded text-generation models
		"filter":    {"text-generation"},
		"sort":      {"downloads"},
		"direction": {"-1"},
		"limit":     {"10"},
	},
	{
		// the most downloaded GGUF models
		"filter":    {"gguf"},
		"sort":      {"downloads"},
		"direction": {"-1"},
		"limit":     {"10"},
	},
	{
		// the most liked GGUF models
		"filter":    {"gguf"},
		"sort":      {"likes"},
		"direction": {"-1"},
		"limit":     {"10"},
	},
}

// hfModel is one entry of the HuggingFace models API response.
type hfModel struct {
	ID          string   `json:"id"`
	Downloads   int64    `json:"downloads"`
	Likes       int      `json:"likes"`
	PipelineTag string   `json:"pipeline_tag"`
	Tags        []string `json:"tags"`
}

// hfTreeFile is one entry of the HuggingFace tree API response.
type hfTreeFile struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// HuggingFaceScanner scans the HuggingFace model hub for models.
type HuggingFaceScanner struct{}

// NewHuggingFaceScanner creates a new HuggingFaceScanner.
func NewHuggingFaceScanner() *HuggingFaceScanner {
	return &HuggingFaceScanner{}
}

// ScannerName implements [definitions.ModelScanner].
func (this *HuggingFaceScanner) ScannerName() string {
	return "HuggingFace"
}

// GetModel implements [definitions.ModelScanner]. It verifies that a model
// with the given id exists on the hub and, for GGUF releases, locates the
// model's first GGUF file so a link can record where it lives.
func (this *HuggingFaceScanner) GetModel(modelName string) *definitions.RemoteModel {
	m, err := this.getModel(modelName)
	if err != nil {
		// the model does not exist (or the hub is unreachable); either way
		// there is nothing to link
		return nil
	}

	modelType := "hf"
	modelFile := m.repoURL()
	if slices.Contains(m.Tags, "gguf") {
		modelType = "gguf"
		if file, err := this.firstGgufFile(m.ID); err == nil {
			modelFile = m.resolveURL(file)
		}
	}

	return &definitions.RemoteModel{
		Model: definitions.Model{
			ModelName: m.ID,
			ModelType: modelType,
			ModelFile: modelFile,
		},
		ScannerName:  this.ScannerName(),
		MetadataFile: "",
	}
}

// ScanForModels implements [definitions.ModelScanner]. It runs each hard
// coded filter option against the hub API and merges the results, dropping
// models that more than one option returns.
func (this *HuggingFaceScanner) ScanForModels() ([]definitions.RemoteModel, error) {
	models := make([]definitions.RemoteModel, 0)
	seen := make(map[string]bool)
	for _, params := range hfFilterOptions {
		list, err := this.listModels(params)
		if err != nil {
			return nil, err
		}
		for _, m := range list {
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			models = append(models, this.toRemoteModel(m))
		}
	}
	return models, nil
}

// toRemoteModel maps a hub model to the scanner's RemoteModel. The model
// file is the model's hub page; linking via [GetModel] refines it to an
// actual file when one can be found.
func (this *HuggingFaceScanner) toRemoteModel(m hfModel) definitions.RemoteModel {
	modelType := "hf"
	if slices.Contains(m.Tags, "gguf") {
		modelType = "gguf"
	}
	return definitions.RemoteModel{
		Model: definitions.Model{
			ModelName: m.ID,
			ModelType: modelType,
			ModelFile: m.repoURL(),
		},
		ScannerName:  this.ScannerName(),
		MetadataFile: "",
	}
}

// repoURL returns the hub URL of the model's page.
func (m *hfModel) repoURL() string {
	return fmt.Sprintf("%s/%s", hfBaseURL, m.ID)
}

// resolveURL returns the download URL of a file within the model's default
// branch.
func (m *hfModel) resolveURL(filename string) string {
	return fmt.Sprintf("%s/%s/resolve/main/%s", hfBaseURL, m.ID, url.PathEscape(filename))
}

// listModels fetches one page of the hub's models listing API.
func (this *HuggingFaceScanner) listModels(params url.Values) ([]hfModel, error) {
	if params == nil {
		params = url.Values{}
	}

	u := hfBaseURL + "/api/models?" + params.Encode()
	resp, err := hfClient.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var models []hfModel
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, err
	}
	return models, nil
}

// getModel fetches a single model from the hub API.
func (this *HuggingFaceScanner) getModel(modelName string) (*hfModel, error) {
	// model ids look like "org/name"; the hub API wants the slash raw
	u := fmt.Sprintf("%s/api/models/%s", hfBaseURL, modelName)
	resp, err := hfClient.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model %s: unexpected status %d", modelName, resp.StatusCode)
	}

	var m hfModel
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// firstGgufFile returns the path of the first GGUF file in the model's
// default branch.
func (this *HuggingFaceScanner) firstGgufFile(modelID string) (string, error) {
	u := fmt.Sprintf("%s/api/models/%s/tree/main", hfBaseURL, modelID)
	resp, err := hfClient.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("model %s: tree: unexpected status %d", modelID, resp.StatusCode)
	}

	var files []hfTreeFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return "", err
	}
	for _, f := range files {
		if f.Type == "file" && strings.HasSuffix(f.Path, ".gguf") {
			return f.Path, nil
		}
	}
	return "", fmt.Errorf("model %s: no GGUF files found", modelID)
}
