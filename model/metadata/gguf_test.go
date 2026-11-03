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

// TestDetectChatCapabilities tests the chat detection logic
func TestDetectChatCapabilities(t *testing.T) {
	testCases := []struct {
		name           string
		architecture   string
		extraMetadata  map[string]any
		expectedChat   bool
	}{
		// Standard language model architectures - should support chat
		{
			name:           "llama architecture",
			architecture:   "llama",
			expectedChat:   true,
		},
		{
			name:           "qwen2 architecture",
			architecture:   "qwen2",
			expectedChat:   true,
		},
		{
			name:           "qwen3 architecture",
			architecture:   "qwen3",
			expectedChat:   true,
		},
		{
			name:           "phi3 architecture",
			architecture:   "phi3",
			expectedChat:   true,
		},
		{
			name:           "gemma2 architecture",
			architecture:   "gemma2",
			expectedChat:   true,
		},
		{
			name:           "mistral architecture",
			architecture:   "mistral",
			expectedChat:   true,
		},
		{
			name:           "deepseek architecture",
			architecture:   "deepseek",
			expectedChat:   true,
		},
		{
			name:           "chatglm architecture",
			architecture:   "chatglm",
			expectedChat:   true,
		},
		{
			name:           "glm4 architecture",
			architecture:   "glm4",
			expectedChat:   true,
		},
		{
			name:           "nemotron architecture",
			architecture:   "nemotron",
			expectedChat:   true,
		},
		{
			name:           "mamba architecture",
			architecture:   "mamba",
			expectedChat:   true,
		},
		{
			name:           "rwkv6 architecture",
			architecture:   "rwkv6",
			expectedChat:   true,
		},
		{
			name:           "starcoder2 architecture",
			architecture:   "starcoder2",
			expectedChat:   true,
		},
		{
			name:           "falcon architecture",
			architecture:   "falcon",
			expectedChat:   true,
		},
		{
			name:           "bloom architecture",
			architecture:   "bloom",
			expectedChat:   true,
		},
		{
			name:           "baichuan architecture",
			architecture:   "baichuan",
			expectedChat:   true,
		},
		{
			name:           "gpt2 architecture",
			architecture:   "gpt2",
			expectedChat:   true,
		},
		{
			name:           "gptj architecture",
			architecture:   "gptj",
			expectedChat:   true,
		},
		{
			name:           "gptneox architecture",
			architecture:   "gptneox",
			expectedChat:   true,
		},
		{
			name:           "mpt architecture",
			architecture:   "mpt",
			expectedChat:   true,
		},
		{
			name:           "stablelm architecture",
			architecture:   "stablelm",
			expectedChat:   true,
		},
		{
			name:           "internlm2 architecture",
			architecture:   "internlm2",
			expectedChat:   true,
		},
		{
			name:           "minicpm3 architecture",
			architecture:   "minicpm3",
			expectedChat:   true,
		},
		{
			name:           "qwen2_moe architecture",
			architecture:   "qwen2moe",
			expectedChat:   true,
		},
		{
			name:           "qwen3_moe architecture",
			architecture:   "qwen3moe",
			expectedChat:   true,
		},
		{
			name:           "phi_moe architecture",
			architecture:   "phimoe",
			expectedChat:   true,
		},
		{
			name:           "olmo architecture",
			architecture:   "olmo",
			expectedChat:   true,
		},
		{
			name:           "olmo2 architecture",
			architecture:   "olmo2",
			expectedChat:   true,
		},
		{
			name:           "command_r architecture",
			architecture:   "command_r",
			expectedChat:   true,
		},
		{
			name:           "cohere2 architecture",
			architecture:   "cohere2",
			expectedChat:   true,
		},
		{
			name:           "dbrx architecture",
			architecture:   "dbrx",
			expectedChat:   true,
		},
		{
			name:           "jamba architecture",
			architecture:   "jamba",
			expectedChat:   true,
		},
		{
			name:           "granite architecture",
			architecture:   "granite",
			expectedChat:   true,
		},
		{
			name:           "exaone architecture",
			architecture:   "exaone",
			expectedChat:   true,
		},
		{
			name:           "xverse architecture",
			architecture:   "xverse",
			expectedChat:   true,
		},
		{
			name:           "jaais architecture",
			architecture:   "jais",
			expectedChat:   true,
		},
		{
			name:           "plamo architecture",
			architecture:   "plamo",
			expectedChat:   true,
		},
		{
			name:           "codeshell architecture",
			architecture:   "codeshell",
			expectedChat:   true,
		},
		{
			name:           "orion architecture",
			architecture:   "orion",
			expectedChat:   true,
		},
		{
			name:           "nemotron_h architecture",
			architecture:   "nemotron_h",
			expectedChat:   true,
		},
		{
			name:           "nemotron_h_moe architecture",
			architecture:   "nemotron_h_moe",
			expectedChat:   true,
		},
		{
			name:           "exaone4 architecture",
			architecture:   "exaone4",
			expectedChat:   true,
		},
		{
			name:           "exaone_moe architecture",
			architecture:   "exaone_moe",
			expectedChat:   true,
		},
		{
			name:           "rwkv6qwen2 architecture",
			architecture:   "rwkv6qwen2",
			expectedChat:   true,
		},
		{
			name:           "rwkv7 architecture",
			architecture:   "rwkv7",
			expectedChat:   true,
		},
		{
			name:           "arwkv7 architecture",
			architecture:   "arwkv7",
			expectedChat:   true,
		},
		{
			name:           "maple architecture",
			architecture:   "maple",
			expectedChat:   true,
		},
		{
			name:           "muse_glimmer architecture",
			architecture:   "muse_glimmer",
			expectedChat:   true,
		},
		{
			name:           "openelm architecture",
			architecture:   "openelm",
			expectedChat:   true,
		},
		{
			name:           "arctic architecture",
			architecture:   "arctic",
			expectedChat:   true,
		},
		{
			name:           "deepseek2 architecture",
			architecture:   "deepseek2",
			expectedChat:   true,
		},
		{
			name:           "deepseek4 architecture",
			architecture:   "deepseek4",
			expectedChat:   true,
		},
		{
			name:           "glm4_moe architecture",
			architecture:   "glm4_moe",
			expectedChat:   true,
		},
		{
			name:           "glm_dsa architecture",
			architecture:   "glm_dsa",
			expectedChat:   true,
		},
		{
			name:           "bitnet architecture",
			architecture:   "bitnet",
			expectedChat:   true,
		},
		{
			name:           "t5 architecture",
			architecture:   "t5",
			expectedChat:   true,
		},
		{
			name:           "jaais2 architecture",
			architecture:   "jais2",
			expectedChat:   true,
		},
		{
			name:           "plamo2 architecture",
			architecture:   "plamo2",
			expectedChat:   true,
		},
		{
			name:           "plamo3 architecture",
			architecture:   "plamo3",
			expectedChat:   true,
		},
		{
			name:           "refact architecture",
			architecture:   "refact",
			expectedChat:   true,
		},
		{
			name:           "deci architecture",
			architecture:   "deci",
			expectedChat:   true,
		},
		{
			name:           "grok architecture",
			architecture:   "grok",
			expectedChat:   true,
		},
		{
			name:           "clef architecture",
			architecture:   "clef",
			expectedChat:   true,
		},
		{
			name:           "qwen4exp architecture",
			architecture:   "qwen4exp",
			expectedChat:   true,
		},
		{
			name:           "qwen3next architecture",
			architecture:   "qwen3next",
			expectedChat:   true,
		},
		{
			name:           "qwen35 architecture",
			architecture:   "qwen35",
			expectedChat:   true,
		},
		{
			name:           "qwen35moe architecture",
			architecture:   "qwen35moe",
			expectedChat:   true,
		},
		{
			name:           "gemma3 architecture",
			architecture:   "gemma3",
			expectedChat:   true,
		},
		{
			name:           "gemma4 architecture",
			architecture:   "gemma4",
			expectedChat:   true,
		},
		{
			name:           "gemma4_assistant architecture",
			architecture:   "gemma4_assistant",
			expectedChat:   true,
		},

		// Embedding-only architectures - should NOT support chat
		{
			name:           "bert architecture",
			architecture:   "bert",
			expectedChat:   false,
		},
		{
			name:           "modern_bert architecture",
			architecture:   "modern_bert",
			expectedChat:   false,
		},
		{
			name:           "nomic_bert architecture",
			architecture:   "nomic_bert",
			expectedChat:   false,
		},
		{
			name:           "nomic_bert_moe architecture",
			architecture:   "nomic_bert_moe",
			expectedChat:   false,
		},
		{
			name:           "neo_bert architecture",
			architecture:   "neo_bert",
			expectedChat:   false,
		},
		{
			name:           "jina_bert_v2 architecture",
			architecture:   "jina_bert_v2",
			expectedChat:   false,
		},
		{
			name:           "jina_bert_v3 architecture",
			architecture:   "jina_bert_v3",
			expectedChat:   false,
		},
		{
			name:           "eurobert architecture",
			architecture:   "eurobert",
			expectedChat:   false,
		},
		{
			name:           "gemma_embedding architecture",
			architecture:   "gemma_embedding",
			expectedChat:   false,
		},
		{
			name:           "gemma_embedding2 architecture",
			architecture:   "gemma_embedding2",
			expectedChat:   false,
		},
		{
			name:           "t5encoder architecture",
			architecture:   "t5encoder",
			expectedChat:   false,
		},
		{
			name:           "e5 architecture",
			architecture:   "e5",
			expectedChat:   false,
		},
		{
			name:           "bge architecture",
			architecture:   "bge",
			expectedChat:   false,
		},
		{
			name:           "gte architecture",
			architecture:   "gte",
			expectedChat:   false,
		},
		{
			name:           "jina_embedding architecture",
			architecture:   "jina_embedding",
			expectedChat:   false,
		},
		{
			name:           "jina_embedding_v2 architecture",
			architecture:   "jina_embedding_v2",
			expectedChat:   false,
		},
		{
			name:           "jina_embedding_v3 architecture",
			architecture:   "jina_embedding_v3",
			expectedChat:   false,
		},
		{
			name:           "bge_m3 architecture",
			architecture:   "bge_m3",
			expectedChat:   false,
		},
		{
			name:           "uv architecture",
			architecture:   "uv",
			expectedChat:   false,
		},
		{
			name:           "nomic_embed architecture",
			architecture:   "nomic_embed",
			expectedChat:   false,
		},
		{
			name:           "nomic_embed_v1 architecture",
			architecture:   "nomic_embed_v1",
			expectedChat:   false,
		},
		{
			name:           "nomic_embed_v1_5 architecture",
			architecture:   "nomic_embed_v1_5",
			expectedChat:   false,
		},
		{
			name:           "all_minilm architecture",
			architecture:   "all_minilm",
			expectedChat:   false,
		},
		{
			name:           "all_mpnet architecture",
			architecture:   "all_mpnet",
			expectedChat:   false,
		},
		{
			name:           "multilingual_e5 architecture",
			architecture:   "multilingual_e5",
			expectedChat:   false,
		},
		{
			name:           "bge_multilingual architecture",
			architecture:   "bge_multilingual",
			expectedChat:   false,
		},
		{
			name:           "gte_multilingual architecture",
			architecture:   "gte_multilingual",
			expectedChat:   false,
		},
		{
			name:           "jina_multilingual architecture",
			architecture:   "jina_multilingual",
			expectedChat:   false,
		},

		// Embedding indicator in architecture name
		{
			name:           "custom_embedding_model architecture",
			architecture:   "custom_embedding_model",
			expectedChat:   false,
		},
		{
			name:           "my_embed_model architecture",
			architecture:   "my_embed_model",
			expectedChat:   false,
		},
		{
			name:           "encoder_only_model architecture",
			architecture:   "encoder_only_model",
			expectedChat:   false,
		},
		{
			name:           "reranker_model architecture",
			architecture:   "reranker_model",
			expectedChat:   false,
		},
		{
			name:           "rerank_model architecture",
			architecture:   "rerank_model",
			expectedChat:   false,
		},

		// mmproj (multimodal projector) - should NOT support chat
		{
			name:           "mmproj architecture",
			architecture:   "mmproj",
			expectedChat:   false,
		},
		{
			name:           "custom_mmproj_model architecture",
			architecture:   "custom_mmproj_model",
			expectedChat:   false,
		},

		// Vision encoder architectures - should NOT support chat
		{
			name:           "clip architecture",
			architecture:   "clip",
			expectedChat:   false,
		},
		{
			name:           "siglip architecture",
			architecture:   "siglip",
			expectedChat:   false,
		},
		{
			name:           "clip_vision architecture",
			architecture:   "clip_vision",
			expectedChat:   false,
		},
		{
			name:           "siglip_vision architecture",
			architecture:   "siglip_vision",
			expectedChat:   false,
		},

		// Embedding-related metadata keys
		{
			name:           "unknown architecture with embedding key",
			architecture:   "unknown",
			extraMetadata:  map[string]any{"general.embedding": true},
			expectedChat:   false,
		},
		{
			name:           "unknown architecture with embeddings key",
			architecture:   "unknown",
			extraMetadata:  map[string]any{"general.embeddings": true},
			expectedChat:   false,
		},
		{
			name:           "unknown architecture with bert.embeddings key",
			architecture:   "unknown",
			extraMetadata:  map[string]any{"bert.embeddings": true},
			expectedChat:   false,
		},
		{
			name:           "unknown architecture with embedding.model key",
			architecture:   "unknown",
			extraMetadata:  map[string]any{"embedding.model": true},
			expectedChat:   false,
		},
		{
			name:           "unknown architecture with embeddings key (plural)",
			architecture:   "unknown",
			extraMetadata:  map[string]any{"embeddings": true},
			expectedChat:   false,
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

			metadata.detectChatCapabilities()

			if metadata.HasChat != tc.expectedChat {
				t.Errorf("Expected HasChat=%v, got %v", tc.expectedChat, metadata.HasChat)
			}
		})
	}
}