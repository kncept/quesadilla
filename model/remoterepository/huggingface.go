package remoterepository

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/kncept/quesadilla/model/definitions"
)

var (
	_ definitions.ModelScanner      = (*HuggingFaceScanner)(nil)
	_ definitions.QueryModelScanner = (*HuggingFaceScanner)(nil)
)

// HuggingFaceScannerName is the name the HuggingFace scanner registers under
// and stamps on the models it finds.
const HuggingFaceScannerName = "HuggingFace"

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
	{
		// the most downloaded ONNX models
		"filter":    {"onnx"},
		"sort":      {"downloads"},
		"direction": {"-1"},
		"limit":     {"10"},
	},
	{
		// the most liked ONNX models
		"filter":    {"onnx"},
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
	License     string   `json:"license"`
}

// hfTreeFile is one entry of the HuggingFace tree API response.
type hfTreeFile struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// HuggingFaceScanner scans the HuggingFace model hub for models.
type HuggingFaceScanner struct{}

// NewHuggingFaceScanner creates a new HuggingFaceScanner.
func NewHuggingFaceScanner() *HuggingFaceScanner {
	return &HuggingFaceScanner{}
}

// ScannerName implements [definitions.ModelScanner].
func (this *HuggingFaceScanner) ScannerName() string {
	return HuggingFaceScannerName
}

// GetModel implements [definitions.ModelScanner]. It verifies that a model
// with the given id exists on the hub and lists its GGUF files (its
// variants) and its licence, so an install can fetch them. It returns nil
// when the model does not exist, or when it holds no runnable variants.
func (this *HuggingFaceScanner) GetModel(modelName string) *definitions.RemoteModel {
	m, err := this.getModel(modelName)
	if err != nil {
		// the model does not exist (or the hub is unreachable); either way
		// there is nothing to install
		return nil
	}
	return this.toRemoteModel(*m)
}

// ScanForModels implements [definitions.ModelScanner]. It runs each hard
// coded filter option against the hub API and merges the results, dropping
// models that more than one option returns. Models without a single GGUF
// file (no runnable variants) are trimmed from the results.
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
			if remote := this.toRemoteModel(m); remote != nil {
				models = append(models, *remote)
			}
		}
	}
	return models, nil
}

// ScanForQuery implements [definitions.QueryModelScanner]. It runs a single
// hub search for the query (limited to GGUF and ONNX models) and lists what it finds,
// trimming models without a runnable GGUF or ONNX file. An empty query falls back to
// the scanner's default listing.
func (this *HuggingFaceScanner) ScanForQuery(query string) ([]definitions.RemoteModel, error) {
	return this.ScanForQueryWithTypes(query, []string{string(definitions.ModelTypeGGUF), string(definitions.ModelTypeONNX)})
}

// ScanForQueryWithTypes runs a hub search for the query, filtered by the given
// model types (e.g., "gguf", "onnx"). An empty query runs a search with just the
// model type filters (no text search). An empty modelTypes slice means no type
// filtering.
func (this *HuggingFaceScanner) ScanForQueryWithTypes(query string, modelTypes []string) ([]definitions.RemoteModel, error) {
	// Build filter from model types
	filterValue := ""
	if len(modelTypes) > 0 {
		filterValue = strings.Join(modelTypes, ",")
	}

	params := url.Values{
		"sort":      {"downloads"},
		"direction": {"-1"},
		"limit":     {"30"},
	}
	if strings.TrimSpace(query) != "" {
		params.Set("search", query)
	}
	if filterValue != "" {
		params.Set("filter", filterValue)
	}

	list, err := this.listModels(params)
	if err != nil {
		return nil, err
	}

	models := make([]definitions.RemoteModel, 0, len(list))
	for _, m := range list {
		if remote := this.toRemoteModel(m); remote != nil {
			models = append(models, *remote)
		}
	}
	return models, nil
}

// toRemoteModel maps a hub model to the scanner's RemoteModel: its family
// and name come from the model's id, and its variants are the model's GGUF
// or ONNX files (listed from the repository's tree, with their sizes). A model
// without a single GGUF or ONNX file has no runnable variants and is reported as
// nil, so it can be trimmed from listings.
func (this *HuggingFaceScanner) toRemoteModel(m hfModel) *definitions.RemoteModel {
	variants, licencePath, modelType := this.variantsFromTree(m)
	if len(variants) == 0 {
		return nil
	}

	remote := &definitions.RemoteModel{
		ModelFamily: "",
		ModelName:   "",
		ModelSource: this.ScannerName(),
		ModelType:   modelType,
		ModelURL:    m.repoURL(),
		Variants:    variants,
		LicenceName: m.License,
	}
	remote.ModelFamily, remote.ModelName = definitions.SplitModelName(m.ID)
	if licencePath != "" {
		remote.LicenceFile = m.resolveURL(licencePath)
	}
	return remote
}

// variantsFromTree lists the model's variants (its GGUF and ONNX files) and its
// licence file, from the model's repository tree. It returns the variants, the
// licence file path (if found), and the model type (GGUF or ONNX) based on the
// first variant found.
func (this *HuggingFaceScanner) variantsFromTree(m hfModel) ([]definitions.ModelVariant, string, string) {
	files, err := this.treeFiles(m.ID)
	if err != nil {
		// without the tree there is no way to list the model's variants
		return nil, "", ""
	}

	variants := make([]definitions.ModelVariant, 0)
	licencePath := ""
	var modelType string
	for _, f := range files {
		if f.Type != "file" {
			continue
		}
		if strings.HasSuffix(f.Path, ".gguf") {
			variants = append(variants, definitions.ModelVariant{
				FileName: path.Base(f.Path),
				Location: m.resolveURL(f.Path),
				FileSize: f.Size,
			})
			if modelType == "" {
				modelType = string(definitions.ModelTypeGGUF)
			}
			continue
		}
		if strings.HasSuffix(f.Path, ".onnx") {
			variants = append(variants, definitions.ModelVariant{
				FileName: path.Base(f.Path),
				Location: m.resolveURL(f.Path),
				FileSize: f.Size,
			})
			if modelType == "" {
				modelType = string(definitions.ModelTypeONNX)
			}
			continue
		}
		if licencePath == "" && isLicenceFile(f.Path) {
			licencePath = f.Path
		}
	}
	return variants, licencePath, modelType
}

// isLicenceFile reports whether a repository path is the repository's
// licence file: a LICENSE or COPYING file in the repository root.
func isLicenceFile(p string) bool {
	if path.Dir(p) != "." {
		return false // only the repository root: a nested licence is not the model's
	}
	switch strings.ToUpper(path.Base(p)) {
	case "LICENSE", "LICENSE.MD", "LICENSE.TXT", "COPYING", "COPYING.MD":
		return true
	}
	return false
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

// treeFiles fetches the files in the model's default branch, walked
// recursively, so GGUF files nested in subdirectories are found too.
func (this *HuggingFaceScanner) treeFiles(modelID string) ([]hfTreeFile, error) {
	u := fmt.Sprintf("%s/api/models/%s/tree/main?recursive=true", hfBaseURL, modelID)
	resp, err := hfClient.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model %s: tree: unexpected status %d", modelID, resp.StatusCode)
	}

	var files []hfTreeFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}
	return files, nil
}
