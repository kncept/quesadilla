package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	goggufFormat "github.com/magomedcoder/gogguf/pkg/format"
)

// GGUFMetadata holds the parsed metadata from a GGUF file
type GGUFMetadata struct {
	// Raw metadata from the GGUF file
	Raw map[string]any

	// Architecture is the model architecture (e.g., "llama", "qwen2", "llava", "phi3")
	Architecture string

	// ModelName is the name of the model
	ModelName string

	// FileType is the quantization type
	FileType string

	// HasVision indicates if the model has vision/multimodal capabilities
	HasVision bool

	// MMProjPath is the path to the multimodal projector file (mmproj), if specified in metadata or found
	MMProjPath string

	// ModelFilePath is the path to the model file (used for finding mmproj)
	ModelFilePath string
}

// ReadGGUFMetadata reads and parses the metadata from a GGUF file
func ReadGGUFMetadata(filePath string) (*GGUFMetadata, error) {
	reader, err := goggufFormat.OpenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open GGUF file: %w", err)
	}

	metadata := &GGUFMetadata{
		Raw:           reader.Metadata,
		ModelFilePath: filePath,
	}

	// Extract common metadata fields
	if arch, err := metadata.getString("general.architecture"); err == nil {
		metadata.Architecture = arch
	}

	if name, err := metadata.getString("general.name"); err == nil {
		metadata.ModelName = name
	}

	if fileType, err := metadata.getString("general.file_type"); err == nil {
		metadata.FileType = fileType
	}

	// Check for vision/multimodal capabilities
	metadata.detectVisionCapabilities()

	// Check for mmproj path in metadata
	if mmproj, err := metadata.getString("llama.mmproj"); err == nil {
		metadata.MMProjPath = mmproj
	}

	// If no mmproj path in metadata but model has vision, try to find it
	if metadata.HasVision && metadata.MMProjPath == "" {
		metadata.MMProjPath = metadata.findMMProjFile()
	}

	return metadata, nil
}

// getString is a helper to get a string value from metadata
func (this *GGUFMetadata) getString(key string) (string, error) {
	val, ok := this.Raw[key]
	if !ok {
		return "", fmt.Errorf("metadata key %q not found", key)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("metadata key %q is not a string (type: %T)", key, val)
	}
	return str, nil
}

// detectVisionCapabilities checks if the model has vision/multimodal capabilities
// based on its architecture and other metadata
func (this *GGUFMetadata) detectVisionCapabilities() {
	// Known multimodal/vision architectures
	visionArchs := map[string]bool{
		"llava":           true,
		"llava_next":      true,
		"llava-onevision": true,
		"qwen2_vl":        true,
		"qwen2.5_vl":      true,
		"phi3_v":          true,
		"phi3.5_v":        true,
		"minicpmv":        true,
		"minicpm-v":       true,
		"idefics2":        true,
		"idefics3":        true,
		"paligemma":       true,
		"pixtral":         true,
		"molmo":           true,
		"glm4v":           true,
		"internvl2":       true,
		"deepseek_vl":     true,
		"deepseek-vl":     true,
		"cogvlm":          true,
		"cogvlm2":         true,
		"xgen_mm":         true,
		"llama3_2_vision": true,
		"llama3.2_vision": true,
		"smolvlm":         true,
		"gemma3":          true,
	}

	arch := this.Architecture
	if visionArchs[arch] {
		this.HasVision = true
		return
	}

	// Check for clip or siglip in architecture name (common vision encoder indicators)
	// e.g., "clip", "siglip", "clip_vision", "siglip_vision", etc.
	archLower := strings.ToLower(arch)
	if strings.Contains(archLower, "clip") || strings.Contains(archLower, "siglip") {
		this.HasVision = true
		return
	}

	// Also check for vision-related keys in metadata
	visionKeys := []string{
		"general.vision",
		"general.multimodal",
		"llama.vision",
		"llama.mmproj",
		"clip.vision_model",
		"vision.encoder",
	}

	for _, key := range visionKeys {
		if _, ok := this.Raw[key]; ok {
			this.HasVision = true
			return
		}
	}
}

// findMMProjFile searches for an mmproj file in the model directory
// Common naming patterns:
// - mmproj-<model>.gguf
// - <model>-mmproj.gguf
// - mmproj.gguf
// - *.mmproj.gguf
func (this *GGUFMetadata) findMMProjFile() string {
	if this.ModelFilePath == "" {
		return ""
	}

	modelDir := filepath.Dir(this.ModelFilePath)
	modelBase := strings.TrimSuffix(filepath.Base(this.ModelFilePath), filepath.Ext(this.ModelFilePath))

	// Common mmproj naming patterns
	patterns := []string{
		"mmproj-" + modelBase + ".gguf",
		modelBase + "-mmproj.gguf",
		"mmproj.gguf",
		"*mmproj*.gguf",
	}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(modelDir, pattern))
		if err != nil {
			continue
		}
		if len(matches) > 0 {
			// Return the first match
			return matches[0]
		}
	}

	return ""
}

// CanUseMMProj returns true if the model supports multimodal processing
// and has an mmproj file available
func (this *GGUFMetadata) CanUseMMProj() bool {
	return this.HasVision && this.MMProjPath != ""
}

// GetMMProjPath returns the path to the mmproj file if available
// Returns empty string if not available
func (this *GGUFMetadata) GetMMProjPath() string {
	return this.MMProjPath
}

// GetMMProjPathAbs returns the absolute path to the mmproj file
// If the path is relative, it's resolved relative to the model directory
func (this *GGUFMetadata) GetMMProjPathAbs() string {
	if this.MMProjPath == "" {
		return ""
	}
	if filepath.IsAbs(this.MMProjPath) {
		return this.MMProjPath
	}
	if this.ModelFilePath == "" {
		return this.MMProjPath
	}
	modelDir := filepath.Dir(this.ModelFilePath)
	return filepath.Join(modelDir, this.MMProjPath)
}

// ValidateMMProj checks if the mmproj file exists and is a valid GGUF file
func (this *GGUFMetadata) ValidateMMProj() error {
	mmprojPath := this.GetMMProjPathAbs()
	if mmprojPath == "" {
		return fmt.Errorf("no mmproj path available")
	}

	if _, err := os.Stat(mmprojPath); err != nil {
		return fmt.Errorf("mmproj file not found at %s: %w", mmprojPath, err)
	}

	// Verify it's a valid GGUF file
	reader, err := goggufFormat.OpenFile(mmprojPath)
	if err != nil {
		return fmt.Errorf("mmproj file is not a valid GGUF file: %w", err)
	}

	// Check if it has the mmproj architecture
	if arch, err := getStringFromMetadata(reader.Metadata, "general.architecture"); err == nil {
		if arch != "mmproj" && !strings.Contains(arch, "mmproj") {
			// Not necessarily an error, just a warning
		}
	}

	return nil
}

// getStringFromMetadata is a helper to get a string from raw metadata
func getStringFromMetadata(raw map[string]any, key string) (string, error) {
	val, ok := raw[key]
	if !ok {
		return "", fmt.Errorf("metadata key %q not found", key)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("metadata key %q is not a string (type: %T)", key, val)
	}
	return str, nil
}

func (this *GGUFMetadata) StringifiedRawMetadata() map[string]string {
	result := make(map[string]string)

	// Convert raw metadata to string values
	for key, val := range this.Raw {
		if str, ok := val.(string); ok {
			str = fmt.Sprintf("%v", val)
			if len(str) > 250 {
				str = "==overflow=="
			}
			result[key] = str
		} else {
			str = fmt.Sprintf("%v", val)
			if len(str) > 250 {
				str = "==overflow=="
			}
			result[key] = str
		}
	}
	return result
}
