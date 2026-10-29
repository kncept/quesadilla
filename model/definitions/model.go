package definitions

import (
	"path"
	"strings"
	"time"
)

// ModelType identifies what kind of a model it is, and implies the backend
// type required to run it.
type ModelType string

// The model types the application supports.
const (
	ModelTypeGGUF ModelType = "gguf"
	ModelTypeONNX ModelType = "onnx"
)

// NoneFamily is the model family used when a model's family is unknown.
// Models stored under it live in the "none" directory alongside the other
// family directories.
const NoneFamily = "none"

// QModelFileName is the name of the metadata file stored in each installed
// model's directory.
const QModelFileName = "qmodel.json"

// ModelVariant is one runnable file of a model. For gguf and onnx models, a
// variant is a single model file (.gguf or .onnx); for gguf it is typically
// one quantization of the model.
//
// For variants whose files are stored in the model's own directory (beside
// its qmodel.json), FileName is the file's base name and Location is empty.
// For linked variants (LinkedModel is true) the file lives elsewhere: a path
// somewhere else on the filesystem (or a URL), held in Location, with
// FileName still the file's base name.
//
// ImportedAt and FileSize are stamped by the local repository when a variant
// is imported; for remote (not yet installed) variants they may be zero.
type ModelVariant struct {
	FileName          string            `json:"FileName"`
	Description       string            `json:"Description,omitempty"`
	ImportedAt        time.Time         `json:"ImportedAt"`
	FileSize          int64             `json:"FileSize"`
	LinkedModel       bool              `json:"LinkedModel"`
	Location          string            `json:"Location,omitempty"`
	SupportsVision    bool              `json:"SupportsVision,omitempty"`
	EmbeddedMetadata  map[string]string `json:"EmbeddedMetadata,omitempty"`
}

// Model is an installed local model: a family and a name (together it is
// identified by "family/name"), a type that selects its backend, and the
// variants (files) that can be run.
type Model struct { // output to qmodel.json
	MetadataVersion string
	ModelFamily     string
	ModelName       string
	ModelType       string // implies the backend type required
	ModelDirectory  string // the directory holding qmodel.json (and usually the variant files)
	Variants        []ModelVariant
	ModelSource     string // name of the scanner that found the model
	ModelURL        string // URL of the model's page at its source, if known
	LicenceName     string // eg "apache-2.0"
	LicenceFile     string // path to the stored licence file, if one was stored
}

// RemoteModel is a model a scanner found somewhere outside the local
// repository: on a model hub, or in another application's model directory.
// Its variants carry where their files can be fetched from (Location); the
// local repository stamps the rest (import times, sizes) when the model is
// installed. ModelURL is a page the user can open to view the model at its
// source (eg the model's card on a model hub); it is empty when the scanner
// does not know of one.
type RemoteModel struct {
	ModelFamily string
	ModelName   string
	ModelSource string // name of the scanner that found the model
	ModelType   string
	ModelURL    string // URL of the model's page at its source, if known
	Variants    []ModelVariant
	LicenceName string
	LicenceFile string // URL or path to the licence file, if known
}

// QModel is the on-disk metadata (qmodel.json) of an installed model: what
// the model is (family, name), where it came from (source), what type it is,
// the model's page at its source (ModelURL, eg its card on a model hub), the
// variants that can be run, and its licence, if any. The family and name
// match the model's directory (<ModelFamily>/<ModelName>).
type QModel struct {
	MetadataVersion string         `json:"MetadataVersion"`
	ModelFamily     string         `json:"ModelFamily"`
	ModelName       string         `json:"ModelName"`
	ModelSource     string         `json:"ModelSource"`
	ModelType       string         `json:"ModelType"`
	ModelURL        string         `json:"ModelURL,omitempty"`
	Variants        []ModelVariant `json:"Variants"`
	LicenceName     string         `json:"LicenceName,omitempty"`
	LicenceFile     string         `json:"LicenceFile,omitempty"`
}

// ModelScanner finds models installed or hosted elsewhere.
type ModelScanner interface {
	ScannerName() string
	ScanForModels() ([]RemoteModel, error)

	// verifies the model exists, then returns the metadata
	// returns nil if absent or
	GetModel(modelName string) *RemoteModel
}

// QueryModelScanner is a ModelScanner whose scan can also be driven by a
// user-supplied query (eg the search box on the Model Search screen), rather
// than only its built-in default listing. An empty query behaves like
// [ModelScanner.ScanForModels].
type QueryModelScanner interface {
	ModelScanner

	ScanForQuery(query string) ([]RemoteModel, error)
}

// FullModelName joins a model's family and name for display and lookup:
// "family/name", or just the name when the family is unknown.
func FullModelName(family, name string) string {
	if family == "" || family == NoneFamily {
		return name
	}
	return family + "/" + name
}

// SplitModelName splits a full model name ("family/name") into its family and
// name: the component before the first slash is the family, everything after
// it is the name. A name without a slash has an unknown family.
func SplitModelName(fullName string) (family, name string) {
	if i := strings.Index(fullName, "/"); i >= 0 {
		return fullName[:i], fullName[i+1:]
	}
	return NoneFamily, fullName
}

// FullModelName returns the model's full name, "family/name" (see
// [FullModelName]).
func (m Model) FullModelName() string {
	return FullModelName(m.ModelFamily, m.ModelName)
}

// IsLinked reports whether the model's variants all point at files elsewhere
// (a linked model), rather than being stored in the model's own directory. A
// mixed model (some variants linked, some stored) counts as not linked.
func (m Model) IsLinked() bool {
	if len(m.Variants) == 0 {
		return false
	}
	for _, v := range m.Variants {
		if !v.LinkedModel {
			return false
		}
	}
	return true
}

// FullModelName returns the model's full name, "family/name" (see
// [FullModelName]).
func (m RemoteModel) FullModelName() string {
	return FullModelName(m.ModelFamily, m.ModelName)
}

// VariantPath resolves where one of the model's variants lives: for linked
// variants, where they point (a path elsewhere on the filesystem, or a URL);
// otherwise the file in the model's own directory.
func (m Model) VariantPath(v ModelVariant) string {
	if v.LinkedModel {
		return v.Location
	}
	return path.Join(m.ModelDirectory, v.FileName)
}

// GetDefaultVariant returns the first variant of the model, or a zero
// ModelVariant if the model has no variants.
func (m Model) GetDefaultVariant() ModelVariant {
	if len(m.Variants) == 0 {
		return ModelVariant{}
	}
	return m.Variants[0]
}

// GetDefaultVariantPath returns the resolved path of the model's default
// variant (the first variant), or an empty string if the model has no variants.
func (m Model) GetDefaultVariantPath() string {
	v := m.GetDefaultVariant()
	if v.FileName == "" {
		return ""
	}
	return m.VariantPath(v)
}
