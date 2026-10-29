package metadata

import (
	"os"
	"testing"
)

// TestReadGGUFMetadata tests reading metadata from a GGUF file
// This test requires a GGUF file to be present
func TestReadGGUFMetadata(t *testing.T) {
	// Skip if no test model is available
	testModelPath := os.Getenv("TEST_GGUF_MODEL")
	if testModelPath == "" {
		t.Skip("TEST_GGUF_MODEL environment variable not set, skipping test")
	}

	if _, err := os.Stat(testModelPath); os.IsNotExist(err) {
		t.Skipf("Test model not found at %s", testModelPath)
	}

	metadata, err := ReadGGUFMetadata(testModelPath)
	if err != nil {
		t.Fatalf("Failed to read GGUF metadata: %v", err)
	}

	t.Logf("Architecture: %s", metadata.Architecture)
	t.Logf("Model Name: %s", metadata.ModelName)
	t.Logf("File Type: %s", metadata.FileType)
	t.Logf("Has Vision: %v", metadata.HasVision)
	t.Logf("MMProj Path: %s", metadata.MMProjPath)
	t.Logf("Can Use MMProj: %v", metadata.CanUseMMProj())

	// Basic validation
	if metadata.Architecture == "" {
		t.Error("Architecture should not be empty")
	}
}

// TestDetectVisionCapabilities tests the vision detection logic
func TestDetectVisionCapabilities(t *testing.T) {
	testCases := []struct {
		name           string
		architecture   string
		extraMetadata  map[string]any
		expectedVision bool
	}{
		{
			name:           "llava architecture",
			architecture:   "llava",
			expectedVision: true,
		},
		{
			name:           "qwen2_vl architecture",
			architecture:   "qwen2_vl",
			expectedVision: true,
		},
		{
			name:           "phi3_v architecture",
			architecture:   "phi3_v",
			expectedVision: true,
		},
		{
			name:           "llama architecture (no vision)",
			architecture:   "llama",
			expectedVision: false,
		},
		{
			name:           "unknown architecture with vision key",
			architecture:   "unknown",
			extraMetadata:  map[string]any{"general.vision": true},
			expectedVision: true,
		},
		{
			name:           "unknown architecture with mmproj key",
			architecture:   "unknown",
			extraMetadata:  map[string]any{"llama.mmproj": "/path/to/mmproj"},
			expectedVision: true,
		},
		// CLIP/SigLIP architecture detection
		{
			name:           "clip architecture",
			architecture:   "clip",
			expectedVision: true,
		},
		{
			name:           "siglip architecture",
			architecture:   "siglip",
			expectedVision: true,
		},
		{
			name:           "clip_vision architecture",
			architecture:   "clip_vision",
			expectedVision: true,
		},
		{
			name:           "siglip_vision architecture",
			architecture:   "siglip_vision",
			expectedVision: true,
		},
		{
			name:           "CLIP architecture (uppercase)",
			architecture:   "CLIP",
			expectedVision: true,
		},
		{
			name:           "SigLIP architecture (mixed case)",
			architecture:   "SigLIP",
			expectedVision: true,
		},
		{
			name:           "my_custom_clip_model architecture",
			architecture:   "my_custom_clip_model",
			expectedVision: true,
		},
		{
			name:           "llama architecture with clip in name (should not match)",
			architecture:   "llama",
			expectedVision: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			metadata := &GGUFMetadata{
				Raw:         make(map[string]any),
				Architecture: tc.architecture,
			}

			// Add extra metadata
			for k, v := range tc.extraMetadata {
				metadata.Raw[k] = v
			}

			metadata.detectVisionCapabilities()

			if metadata.HasVision != tc.expectedVision {
				t.Errorf("Expected HasVision=%v, got %v", tc.expectedVision, metadata.HasVision)
			}
		})
	}
}

// TestGetString tests the getString helper
func TestGetString(t *testing.T) {
	metadata := &GGUFMetadata{
		Raw: map[string]any{
			"test.key": "test_value",
			"int.key":  123,
		},
	}

	// Test successful string retrieval
	val, err := metadata.getString("test.key")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", val)
	}

	// Test missing key
	_, err = metadata.getString("missing.key")
	if err == nil {
		t.Error("Expected error for missing key")
	}

	// Test non-string value
	_, err = metadata.getString("int.key")
	if err == nil {
		t.Error("Expected error for non-string value")
	}
}