package localrepository

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/kncept/quesadilla/model/definitions"
	"github.com/kncept/quesadilla/model/metadata"
	"github.com/kncept/quesadilla/utils/qenv"
	"github.com/kncept/quesadilla/utils/qhttp"
)

// supportedTypes are the model types the local repository stores and scans.
var supportedTypes = []string{string(definitions.ModelTypeGGUF), string(definitions.ModelTypeONNX)}

// typeFileExtensions maps a model type to the file extension of the files
// that make up its variants.
var typeFileExtensions = map[string]string{
	string(definitions.ModelTypeGGUF): ".gguf",
	string(definitions.ModelTypeONNX): ".onnx",
}

// fileExtensionFor returns the variant file extension of the given model
// type (the legacy .gguf when the type is not one the repository knows).
func fileExtensionFor(modelType string) string {
	if ext, ok := typeFileExtensions[modelType]; ok {
		return ext
	}
	return ".gguf"
}

// LocalRepository maintains the application's locally-installed models.
// Models are scanned once when the repository is created, and the loaded set
// is retained from then on. Local models are those stored under
// ~/.quesadilla/models/<modelType>/<ModelFamily>/<ModelName>, where the
// family is "none" when the model's family is unknown. Each model's directory
// holds a qmodel.json (its metadata) alongside the variant files that can be
// run.
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

// GetModel returns the named local model, or nil when absent. The name may be
// the model's full name ("family/name") or, for a model with no known
// family, just its name (the "none/name" form is accepted too).
func (this *LocalRepository) GetModel(modelName string) *definitions.Model {
	for i := range this.models {
		m := &this.models[i]
		if m.FullModelName() == modelName || m.ModelName == modelName {
			return m
		}
		if m.ModelFamily == definitions.NoneFamily && modelName == definitions.NoneFamily+"/"+m.ModelName {
			return m
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
	if err := os.RemoveAll(m.ModelDirectory); err != nil {
		return err
	}
	this.scan()
	return nil
}

// LinkScannedModel links a model found by a scanner into local storage
// without moving its files: the model's directory holds only its
// qmodel.json, with the variants marked linked and pointing at where their
// files live. The repository rescans afterwards.
func (this *LocalRepository) LinkScannedModel(scannedModel *definitions.RemoteModel) error {
	if _, ok := typeFileExtensions[scannedModel.ModelType]; !ok {
		return fmt.Errorf("Unknown model type: %s", scannedModel.ModelType)
	}
	modelsDir := qenv.QModelsDirectory(scannedModel.ModelType)
	modelDir := path.Join(modelsDir, modelFamilyDirectory(scannedModel.ModelFamily), scannedModel.ModelName)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return err
	}

	qm := definitions.QModel{
		MetadataVersion: "v1.0.0",
		ModelFamily:     modelFamilyName(scannedModel.ModelFamily),
		ModelName:       scannedModel.ModelName,
		ModelSource:     scannedModel.ModelSource,
		ModelType:       scannedModel.ModelType,
		ModelURL:        scannedModel.ModelURL,
		LicenceName:     scannedModel.LicenceName,
	}
	now := time.Now()
	for _, v := range scannedModel.Variants {
		v.ImportedAt = now
		v.LinkedModel = true
		if v.Location != "" && !isURL(v.Location) {
			if info, err := os.Stat(v.Location); err == nil {
				v.FileSize = info.Size()
			}
		}
		qm.Variants = append(qm.Variants, v)
	}
	if err := fetchLicence(&qm, scannedModel.LicenceFile, modelDir, nil); err != nil {
		// a failed licence fetch does not fail the link
		log.Printf("local repository: fetching the licence for %s: %v", scannedModel.FullModelName(), err)
	}
	if err := writeQModel(modelDir, qm); err != nil {
		return err
	}
	this.scan()
	return nil
}

// DownloadModel installs a scanned model into local storage: every variant of
// the model is downloaded when the scanner's references are URLs, or copied
// when they are local paths (eg a model found by the LocalAI scanner). Each
// file is fetched to a temporary location first and moved into place only
// once it is complete, so a failed download never leaves a partial file where
// a model is expected (nor clobbers the model that is already installed, on a
// re-download). The model's licence file is fetched too, when the scanner
// knows of one. The repository rescans afterwards, so the model shows up in
// [Models].
func (this *LocalRepository) DownloadModel(scannedModel *definitions.RemoteModel, progress func(current, total int64)) error {
	return this.installVariants(scannedModel, nil, progress)
}

// DownloadVariant installs one of a scanned model's variants (matched by file
// name), merging it with the model's existing local variants when the model
// is already installed. This is how a partially downloaded model grows: each
// additional variant lands in the same model directory.
func (this *LocalRepository) DownloadVariant(scannedModel *definitions.RemoteModel, fileName string, progress func(current, total int64)) error {
	for i := range scannedModel.Variants {
		if scannedModel.Variants[i].FileName == fileName {
			return this.installVariants(scannedModel, &scannedModel.Variants[i], progress)
		}
	}
	return fmt.Errorf("model %s has no variant named %s", scannedModel.FullModelName(), fileName)
}

// installVariants installs the given variants of the scanned model (or, when
// only is nil, all of them) into the model's directory, merging with the
// variants already stored there, then (re)writes the model's qmodel.json and
// rescans.
func (this *LocalRepository) installVariants(scannedModel *definitions.RemoteModel, only *definitions.ModelVariant, progress func(current, total int64)) error {
	if _, ok := typeFileExtensions[scannedModel.ModelType]; !ok {
		return fmt.Errorf("Unknown model type: %s", scannedModel.ModelType)
	}
	modelsDir := qenv.QModelsDirectory(scannedModel.ModelType)
	modelDir := path.Join(modelsDir, modelFamilyDirectory(scannedModel.ModelFamily), scannedModel.ModelName)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return err
	}

	// start from the model's stored metadata, so a re-download (or an
	// additional variant) merges with what is already installed
	qm := loadQModel(modelDir)
	qm.ModelFamily = modelFamilyName(scannedModel.ModelFamily)
	qm.ModelName = scannedModel.ModelName
	qm.ModelSource = scannedModel.ModelSource
	qm.ModelType = scannedModel.ModelType
	if scannedModel.ModelURL != "" {
		qm.ModelURL = scannedModel.ModelURL
	}
	if scannedModel.LicenceName != "" {
		qm.LicenceName = scannedModel.LicenceName
	}

	toInstall := scannedModel.Variants
	if only != nil {
		toInstall = []definitions.ModelVariant{*only}
	}
	now := time.Now()
	for i := range toInstall {
		variant := toInstall[i]
		if variant.FileName == "" || variant.Location == "" {
			continue
		}
		// fetch the variant's file to a temporary location, then move it
		// into place only once it is complete
		tempFile := path.Join(modelsDir, tempFileName(scannedModel.FullModelName(), variant.FileName))
		defer os.Remove(tempFile)
		if isURL(variant.Location) {
			if err := qhttp.DownloadFileWithProgress(variant.Location, modelsDir, path.Base(tempFile), progress); err != nil {
				return err
			}
		} else {
			if err := copyFile(variant.Location, tempFile); err != nil {
				return err
			}
		}
		if err := os.Rename(tempFile, path.Join(modelDir, variant.FileName)); err != nil {
			return err
		}

		variant.ImportedAt = now
		variant.LinkedModel = false
		variant.Location = ""
		if info, err := os.Stat(path.Join(modelDir, variant.FileName)); err == nil {
			variant.FileSize = info.Size()
		}
		// Extract embedded metadata from GGUF files
		if scannedModel.ModelType == string(definitions.ModelTypeGGUF) {
			variantPath := path.Join(modelDir, variant.FileName)
			variant.EmbeddedMetadata = extractEmbeddedMetadata(variantPath)
		}
		upsertVariant(&qm, variant)
	}

	if err := fetchLicence(&qm, scannedModel.LicenceFile, modelDir, progress); err != nil {
		// a failed licence fetch does not fail the install
		log.Printf("local repository: fetching the licence for %s: %v", scannedModel.FullModelName(), err)
	}

	if err := writeQModel(modelDir, qm); err != nil {
		return err
	}
	this.scan()
	return nil
}

// UpdateModelMetadata updates the local model's metadata from the remote model,
// without downloading any variant files. It refreshes fields like description,
// license, variants list, etc., then rescans the repository.
// Only variants that have been downloaded (Location is empty or not https) are kept.
// Variants with https locations are removed from the config.
func (this *LocalRepository) UpdateModelMetadata(scannedModel *definitions.RemoteModel) error {
	modelsDir := qenv.QModelsDirectory(scannedModel.ModelType)
	modelDir := path.Join(modelsDir, modelFamilyDirectory(scannedModel.ModelFamily), scannedModel.ModelName)

	// Load existing local model metadata
	qm := loadQModel(modelDir)
	if qm.ModelName == "" {
		return fmt.Errorf("model not installed locally: %s", scannedModel.FullModelName())
	}

	// Update metadata from remote
	qm.ModelFamily = modelFamilyName(scannedModel.ModelFamily)
	qm.ModelName = scannedModel.ModelName
	qm.ModelSource = scannedModel.ModelSource
	qm.ModelType = scannedModel.ModelType
	if scannedModel.ModelURL != "" {
		qm.ModelURL = scannedModel.ModelURL
	}
	if scannedModel.LicenceName != "" {
		qm.LicenceName = scannedModel.LicenceName
	}

	// Update variants list from remote (but keep local file info like FileSize, ImportedAt)
	remoteVariants := make(map[string]definitions.ModelVariant)
	for _, v := range scannedModel.Variants {
		remoteVariants[v.FileName] = v
	}

	// Merge remote variants into local variants, but only keep locally downloaded variants
	var mergedVariants []definitions.ModelVariant
	for _, localVariant := range qm.Variants {
		// Skip variants that have https location (not downloaded)
		if strings.HasPrefix(localVariant.Location, "https://") {
			continue
		}
		if remoteVariant, ok := remoteVariants[localVariant.FileName]; ok {
			// Update with remote metadata but preserve local file info
			merged := remoteVariant
			merged.FileSize = localVariant.FileSize
			merged.ImportedAt = localVariant.ImportedAt
			merged.LinkedModel = localVariant.LinkedModel
			merged.Location = localVariant.Location
			mergedVariants = append(mergedVariants, merged)
			delete(remoteVariants, localVariant.FileName)
		} else {
			// Keep local variant that's not in remote (but only if not https)
			mergedVariants = append(mergedVariants, localVariant)
		}
	}

	// Do NOT add new remote variants that aren't installed locally
	// Only keep variants that have been downloaded

	qm.Variants = mergedVariants

	// Extract embedded metadata for GGUF variants
	if scannedModel.ModelType == string(definitions.ModelTypeGGUF) {
		for i := range qm.Variants {
			variantPath := path.Join(modelDir, qm.Variants[i].FileName)
			qm.Variants[i].EmbeddedMetadata = extractEmbeddedMetadata(variantPath)
		}
	}

	// Fetch licence if not already present
	if err := fetchLicence(&qm, scannedModel.LicenceFile, modelDir, nil); err != nil {
		log.Printf("local repository: fetching the licence for %s: %v", scannedModel.FullModelName(), err)
	}

	if err := writeQModel(modelDir, qm); err != nil {
		return err
	}
	this.scan()
	return nil
}

// fetchLicence fetches the model's licence file (from a URL or a local path)
// into the model directory, when it is not stored there already. The file is
// stored under its base name, recorded in the metadata.
func fetchLicence(qm *definitions.QModel, licenceLocation string, modelDir string, progress func(current, total int64)) error {
	if licenceLocation == "" || qm.LicenceFile != "" {
		return nil
	}
	licenceName := path.Base(licenceLocation)
	if isURL(licenceLocation) {
		if err := qhttp.DownloadFileWithProgress(licenceLocation, modelDir, licenceName, progress); err != nil {
			return err
		}
	} else {
		if err := copyFile(licenceLocation, path.Join(modelDir, licenceName)); err != nil {
			return err
		}
	}
	qm.LicenceFile = licenceName
	return nil
}

// upsertVariant replaces the variant with the same file name in the model's
// variant list (when present), or appends the variant.
func upsertVariant(qm *definitions.QModel, v definitions.ModelVariant) {
	for i := range qm.Variants {
		if qm.Variants[i].FileName == v.FileName {
			qm.Variants[i] = v
			return
		}
	}
	qm.Variants = append(qm.Variants, v)
}

// extractEmbeddedMetadata reads the GGUF metadata from a variant file and
// converts the raw metadata (map[string]any) to map[string]string for storage.
// Returns nil if the file is not a GGUF file or metadata cannot be read.
func extractEmbeddedMetadata(variantPath string) map[string]string {
	ggufMeta, err := metadata.ReadGGUFMetadata(variantPath)
	if err != nil || ggufMeta == nil || ggufMeta.Raw == nil {
		return nil
	}

	result := make(map[string]string, len(ggufMeta.Raw))
	for k, v := range ggufMeta.Raw {
		if v != nil {
			result[k] = fmt.Sprintf("%v", v)
		}
	}
	return result
}

// tempFileName returns a single file name for the temporary download of a
// model variant. Model names can contain path separators (huggingface ids
// look like "org/name"), so they are replaced to keep it to one file.
func tempFileName(modelName, fileName string) string {
	safe := strings.NewReplacer("/", "_", "\\", "_").Replace(modelName + "-" + fileName)
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

// listAvailableModels lists the models installed for every supported type.
func listAvailableModels() ([]definitions.Model, error) {
	models := make([]definitions.Model, 0)
	for _, modelType := range supportedTypes {
		typedModels, err := listModelsOfType(modelType)
		if err != nil {
			return nil, err
		}
		models = append(models, typedModels...)
	}
	return models, nil
}

// listModelsOfType lists the models installed for one type. Each top-level
// directory of the type directory is a model family (or the "none" family);
// each model's directory beneath it holds the model's qmodel.json and (for
// models that store their files) the variant files.
func listModelsOfType(modelType string) ([]definitions.Model, error) {
	modelsDir := qenv.QModelsDirectory(modelType)
	entries, err := os.ReadDir(modelsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	models := make([]definitions.Model, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue // stray files (eg temporary downloads) are not models
		}
		found, err := listModelsUnder(modelsDir, []string{entry.Name()}, path.Join(modelsDir, entry.Name()), fileExtensionFor(modelType))
		if err != nil {
			return nil, err
		}
		models = append(models, found...)
	}
	return models, nil
}

// listModelsUnder lists the models stored under dir, which sits
// path.Join(modelsDir, components...) below the type directory. A directory
// that holds a model (its qmodel.json, a legacy model.json, or bare model
// files) is loaded; the model's family and name are the walked components
// (the first component is the family, the rest the name). Otherwise the
// model, if any, may be in a subdirectory (legacy nested names), so the walk
// continues into the subdirectories.
func listModelsUnder(modelsDir string, components []string, dir string, ext string) ([]definitions.Model, error) {
	if isModelDirectory(dir, ext) {
		if len(components) == 1 && components[0] != definitions.NoneFamily {
			// a model directly under the type directory has no family:
			// relocate it under the "none" family, as the current layout
			// requires
			if err := relocateUnderNone(modelsDir, dir); err != nil {
				return nil, err
			}
			components = []string{definitions.NoneFamily, components[0]}
			dir = path.Join(modelsDir, definitions.NoneFamily, components[1])
		}
		return loadModelDirectory(components, dir, ext)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	models := make([]definitions.Model, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		found, err := listModelsUnder(modelsDir, append(components, entry.Name()), path.Join(dir, entry.Name()), ext)
		if err != nil {
			return nil, err
		}
		models = append(models, found...)
	}
	return models, nil
}

// isModelDirectory reports whether dir holds a model: its qmodel.json, a
// legacy model.json, or one or more model files with the type's extension.
func isModelDirectory(dir string, ext string) bool {
	if fileExists(path.Join(dir, definitions.QModelFileName)) {
		return true
	}
	if fileExists(path.Join(dir, legacyMetadataFileName)) {
		return true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ext) {
			return true
		}
	}
	return false
}

// relocateUnderNone moves a legacy flat model directory (one directly under
// the type directory, with no family of its own) under the "none" family.
// When a model is already stored at the destination, the directories are
// merged rather than one clobbering the other.
func relocateUnderNone(modelsDir, dir string) error {
	noneDir := path.Join(modelsDir, definitions.NoneFamily)
	if err := os.MkdirAll(noneDir, 0755); err != nil {
		return err
	}
	dest := path.Join(noneDir, path.Base(dir))
	if dest == dir {
		return nil
	}
	if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
		return os.Rename(dir, dest)
	}
	// a model is already at the destination: merge the entries across
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.Rename(path.Join(dir, entry.Name()), path.Join(dest, entry.Name())); err != nil {
			// an entry that is already present at the destination wins
			log.Printf("local repository: merging %s into %s: %v", dir, dest, err)
		}
	}
	return os.RemoveAll(dir)
}

// legacyMetadataFileName is the metadata file of the previous layout, held
// by linked models.
const legacyMetadataFileName = "model.json"

// legacyQModel is the on-disk shape of the previous layout's model.json.
type legacyQModel struct {
	ModelName    string
	ModelType    string
	ModelFile    string
	ScannerName  string
	MetadataFile string
}

// loadModelDirectory loads the model stored in dir (whose path components
// below the type directory are components: the first is the model's family,
// the rest its name). The model's metadata is its qmodel.json; a directory
// without one is a legacy layout (bare model files, or a single model.json
// for a linked model) and is migrated to the current layout on the spot.
// Models without a single runnable variant are not listed.
func loadModelDirectory(components []string, dir string, ext string) ([]definitions.Model, error) {
	family := components[0]
	name := strings.Join(components[1:], "/")
	if name == "" {
		family, name = definitions.NoneFamily, family
	}

	qm, rewrite := loadOrMigrateQModel(dir, family, name, ext)
	if qm.ModelFamily == "" {
		qm.ModelFamily = family
	}
	if qm.ModelName == "" {
		qm.ModelName = name
	}

	// refresh the variant list against the files in the directory: local
	// model files the metadata does not list become variants, and the
	// metadata's local variants get their sizes refreshed from disk
	reconcileVariants(&qm, dir, ext)

	if len(qm.Variants) == 0 {
		// no runnable variants: not a usable model
		return nil, nil
	}

	// keep the stored metadata in step with the directory layout
	if rewrite || qm.ModelFamily != family || qm.ModelName != name {
		qm.ModelFamily = family
		qm.ModelName = name
		if err := writeQModel(dir, qm); err != nil {
			return nil, err
		}
	}

	model := definitions.Model{
		MetadataVersion: qm.MetadataVersion,
		ModelFamily:     qm.ModelFamily,
		ModelName:       qm.ModelName,
		ModelType:       qm.ModelType,
		ModelDirectory:  dir,
		Variants:        qm.Variants,
		ModelSource:     qm.ModelSource,
		ModelURL:        qm.ModelURL,
		LicenceName:     qm.LicenceName,
	}
	if qm.LicenceFile != "" {
		model.LicenceFile = path.Join(dir, qm.LicenceFile)
	}
	model.ModelFile = model.VariantPath(qm.Variants[0])
	return []definitions.Model{model}, nil
}

// loadOrMigrateQModel reads the model's qmodel.json from dir, or builds one
// from the legacy layout stored there (a single model.json for a linked
// model, or bare model files). The second result reports whether the metadata
// must be (re)written: it is a migration, or the stored metadata was missing
// or unreadable.
func loadOrMigrateQModel(dir, family, name string, ext string) (definitions.QModel, bool) {
	qmPath := path.Join(dir, definitions.QModelFileName)
	if data, err := os.ReadFile(qmPath); err == nil {
		qm := definitions.QModel{}
		if err := json.Unmarshal(data, &qm); err != nil {
			log.Printf("local repository: reading %s: %v (the metadata will be rewritten)", qmPath, err)
			return definitions.QModel{MetadataVersion: "v1.0.0", ModelFamily: family, ModelName: name}, true
		}
		return qm, false
	}

	qm := definitions.QModel{MetadataVersion: "v1.0.0", ModelFamily: family, ModelName: name}
	legacyPath := path.Join(dir, legacyMetadataFileName)
	if data, err := os.ReadFile(legacyPath); err == nil {
		// a legacy linked model: its single file is the model.json
		legacy := legacyQModel{}
		if err := json.Unmarshal(data, &legacy); err != nil {
			log.Printf("local repository: reading %s: %v", legacyPath, err)
			return qm, true
		}
		qm.ModelSource = legacy.ScannerName
		if legacy.ModelType != "" {
			qm.ModelType = legacy.ModelType
		}
		importedAt := time.Now()
		if info, err := os.Stat(legacyPath); err == nil {
			// the metadata was written when the model was linked: that is
			// as close to an import time as the legacy layout kept
			importedAt = info.ModTime()
		}
		if legacy.ModelFile != "" {
			variant := definitions.ModelVariant{
				FileName:    path.Base(legacy.ModelFile),
				Location:    legacy.ModelFile,
				LinkedModel: true,
				ImportedAt:  importedAt,
			}
			if !isURL(legacy.ModelFile) {
				if info, err := os.Stat(legacy.ModelFile); err == nil {
					variant.FileSize = info.Size()
				}
			}
			qm.Variants = append(qm.Variants, variant)
		}
		_ = os.Remove(legacyPath)
		return qm, true
	}

	// a legacy bare-file model: the directory holds the model's files
	if entries, err := os.ReadDir(dir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ext) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			variant := definitions.ModelVariant{
				FileName:   entry.Name(),
				FileSize:   info.Size(),
				ImportedAt: info.ModTime(),
			}
			// Extract embedded metadata for GGUF files
			if ext == ".gguf" {
				variantPath := path.Join(dir, entry.Name())
				variant.EmbeddedMetadata = extractEmbeddedMetadata(variantPath)
			}
			qm.Variants = append(qm.Variants, variant)
		}
	}
	return qm, true
}

// reconcileVariants refreshes a model's variant list against the files in
// its directory: local model files the metadata does not list are added as
// variants, and the metadata's local variants have their sizes refreshed
// from disk. The variant list is left sorted by file name.
func reconcileVariants(qm *definitions.QModel, dir string, ext string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ext) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		found := false
		for i := range qm.Variants {
			if qm.Variants[i].FileName == entry.Name() && !qm.Variants[i].LinkedModel {
				qm.Variants[i].FileSize = info.Size()
				found = true
				break
			}
		}
		if !found {
			variant := definitions.ModelVariant{
				FileName:   entry.Name(),
				FileSize:   info.Size(),
				ImportedAt: info.ModTime(),
			}
			// Extract embedded metadata for GGUF files
			if ext == ".gguf" {
				variantPath := path.Join(dir, entry.Name())
				variant.EmbeddedMetadata = extractEmbeddedMetadata(variantPath)
			}
			qm.Variants = append(qm.Variants, variant)
		}
	}
	sort.Slice(qm.Variants, func(i, j int) bool {
		return qm.Variants[i].FileName < qm.Variants[j].FileName
	})
}

// loadQModel reads the model's qmodel.json from the model directory,
// returning a zero QModel when there is none (or it cannot be read).
func loadQModel(modelDir string) definitions.QModel {
	data, err := os.ReadFile(path.Join(modelDir, definitions.QModelFileName))
	if err != nil {
		return definitions.QModel{}
	}
	qm := definitions.QModel{}
	if err := json.Unmarshal(data, &qm); err != nil {
		log.Printf("local repository: reading qmodel.json in %s: %v", modelDir, err)
		return definitions.QModel{}
	}
	return qm
}

// writeQModel writes the model's metadata to its directory's qmodel.json.
func writeQModel(modelDir string, qm definitions.QModel) error {
	jsonData, err := json.MarshalIndent(qm, "", " ")
	if err != nil {
		return err
	}
	jsonData = append(jsonData, '\n')
	return os.WriteFile(path.Join(modelDir, definitions.QModelFileName), jsonData, 0644)
}

// modelFamilyDirectory returns the directory name a model's family is stored
// under: the family itself, or "none" when the family is unknown.
func modelFamilyDirectory(family string) string {
	if family == "" {
		return definitions.NoneFamily
	}
	return family
}

// modelFamilyName returns the family name stored in a model's metadata: the
// family itself, or "none" when the family is unknown.
func modelFamilyName(family string) string {
	if family == "" {
		return definitions.NoneFamily
	}
	return family
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
