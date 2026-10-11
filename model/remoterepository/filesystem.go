package remoterepository

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/kncept/quesadilla/model/definitions"
)

// FilesystemScannerName is the name the filesystem scanner registers and
// reports its models under.
const FilesystemScannerName = "Filesystem"

// FilesystemDepthUnlimited is the depth that makes the scanner descend every
// level below its root directory.
const FilesystemDepthUnlimited = -1

// FilesystemScanner scans a directory chosen by the user for GGUF models.
//
// Every directory that holds one or more .gguf files is one model: the model
// is named by its path relative to the scan root (the root itself is named by
// its base name), and the .gguf files it holds are the model's variants. The
// scan descends at most Depth levels below the root, or every level when the
// depth is [FilesystemDepthUnlimited].
//
// The scanner is configured with the directory to search (and how deep)
// through [FilesystemScanner.Configure] before scanning, because the
// directory is chosen by the user rather than known up front.
var _ definitions.ModelScanner = (*FilesystemScanner)(nil)

type FilesystemScanner struct {
	mu    sync.RWMutex
	root  string
	depth int
}

// NewFilesystemScanner creates a filesystem scanner with no directory
// configured. [FilesystemScanner.Configure] must be called before it can
// scan.
func NewFilesystemScanner() *FilesystemScanner {
	return &FilesystemScanner{}
}

// Configure sets the directory to scan and the depth to descend below it. A
// depth of [FilesystemDepthUnlimited] descends every level; a depth of 0
// searches only the directory itself. The directory is stored absolute (and a
// leading ~ is expanded to the user's home), so the locations of the models
// it finds stay valid wherever the process runs.
func (this *FilesystemScanner) Configure(root string, depth int) {
	root = expandHome(strings.TrimSpace(root))
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	this.mu.Lock()
	defer this.mu.Unlock()
	this.root = root
	this.depth = depth
}

// expandHome expands a leading ~ (or a bare ~) in p to the user's home
// directory.
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, `~\`) {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

// ScannerName implements [definitions.ModelScanner].
func (this *FilesystemScanner) ScannerName() string {
	return FilesystemScannerName
}

// ScanForModels implements [definitions.ModelScanner]. It walks the
// configured directory (to the configured depth) and returns one model per
// directory holding .gguf files, each .gguf file being one of the model's
// variants.
func (this *FilesystemScanner) ScanForModels() ([]definitions.RemoteModel, error) {
	this.mu.RLock()
	root, depth := this.root, this.depth
	this.mu.RUnlock()

	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("no directory configured")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}

	models := make([]definitions.RemoteModel, 0)
	if err := scanFilesystemDir(root, root, depth, 0, &models); err != nil {
		return nil, err
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].FullModelName() < models[j].FullModelName()
	})
	return models, nil
}

// GetModel implements [definitions.ModelScanner]. It re-walks the configured
// directory and returns the model whose name matches, or nil when it is not
// found (or the scan fails).
func (this *FilesystemScanner) GetModel(modelName string) *definitions.RemoteModel {
	models, err := this.ScanForModels()
	if err != nil {
		return nil
	}
	for i := range models {
		if models[i].FullModelName() == modelName || models[i].ModelName == modelName {
			return &models[i]
		}
	}
	return nil
}

// scanFilesystemDir walks dir (level levels below root) collecting models: a
// directory holding .gguf files is one model, and the walk continues into its
// subdirectories while depth allows it.
func scanFilesystemDir(root, dir string, depth, level int, models *[]definitions.RemoteModel) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	variants := make([]definitions.ModelVariant, 0)
	subdirs := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			subdirs = append(subdirs, entry.Name())
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".gguf") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		variants = append(variants, definitions.ModelVariant{
			FileName: entry.Name(),
			Location: filepath.Join(dir, entry.Name()),
			FileSize: info.Size(),
		})
	}

	if len(variants) > 0 {
		sort.Slice(variants, func(i, j int) bool {
			return variants[i].FileName < variants[j].FileName
		})
		*models = append(*models, definitions.RemoteModel{
			ModelFamily: definitions.NoneFamily,
			ModelName:   filesystemModelName(root, dir),
			ModelSource: FilesystemScannerName,
			ModelType:   string(definitions.ModelTypeGGUF),
			Variants:    variants,
		})
	}

	if depth != FilesystemDepthUnlimited && level >= depth {
		return nil
	}
	for _, name := range subdirs {
		if err := scanFilesystemDir(root, filepath.Join(dir, name), depth, level+1, models); err != nil {
			return err
		}
	}
	return nil
}

// filesystemModelName returns the name of the model stored in dir: its path
// relative to the scan root (with "/" separators), or the base name when dir
// is the root itself. Using the relative path keeps same-named directories
// under different parents distinct.
func filesystemModelName(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." {
		return filepath.Base(dir)
	}
	return filepath.ToSlash(rel)
}
