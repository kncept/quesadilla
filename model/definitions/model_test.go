package definitions

import (
	"path"
	"testing"
)

// testModel builds a model with a text variant, a second text variant and a
// vision variant, so the default getters can be checked against each.
func testModel() Model {
	return Model{
		ModelFamily:    "org",
		ModelName:      "a",
		ModelDirectory: "/models/org/a",
		Variants: []ModelVariant{
			{FileName: "a-q4.gguf", SupportsChat: true},
			{FileName: "a-q8.gguf", SupportsChat: true},
			{FileName: "a-mmproj.gguf", SupportsVision: true},
		},
	}
}

func TestGetDefaultVariantFallsBackToFirst(t *testing.T) {
	m := testModel()

	v, err := m.GetDefaultVariant()
	if err != nil {
		t.Fatalf("GetDefaultVariant: %v", err)
	}
	if v.FileName != "a-q4.gguf" {
		t.Errorf("variant = %q, want a-q4.gguf", v.FileName)
	}
	if want := path.Join(m.ModelDirectory, "a-q4.gguf"); v.GetVariantPath() != want {
		t.Errorf("path = %q, want %q", v.GetVariantPath(), want)
	}
}

func TestGetDefaultVariantUsesSelection(t *testing.T) {
	m := testModel()
	m.SelectedTextModel = "a-q8.gguf"

	v, err := m.GetDefaultVariant()
	if err != nil {
		t.Fatalf("GetDefaultVariant: %v", err)
	}
	if v.FileName != "a-q8.gguf" {
		t.Errorf("variant = %q, want a-q8.gguf", v.FileName)
	}
}

func TestGetDefaultVariantErrorsWhenSelectionMissing(t *testing.T) {
	m := testModel()
	m.SelectedTextModel = "gone.gguf"

	if _, err := m.GetDefaultVariant(); err == nil {
		t.Fatal("expected an error when the selected text variant is not present")
	}
}

func TestGetDefaultVariantErrorsWithoutVariants(t *testing.T) {
	m := testModel()
	m.Variants = nil

	if _, err := m.GetDefaultVariant(); err == nil {
		t.Fatal("expected an error when the model has no variants")
	}
}

func TestGetDefaultVisionVariantFallsBackToFirstVisionVariant(t *testing.T) {
	m := testModel()

	v, err := m.GetDefaultVisionVariant()
	if err != nil {
		t.Fatalf("GetDefaultVisionVariant: %v", err)
	}
	if v.FileName != "a-mmproj.gguf" {
		t.Errorf("variant = %q, want a-mmproj.gguf", v.FileName)
	}
	if want := path.Join(m.ModelDirectory, "a-mmproj.gguf"); v.GetVariantPath() != want {
		t.Errorf("path = %q, want %q", v.GetVariantPath(), want)
	}
}

func TestGetDefaultVisionVariantUsesSelection(t *testing.T) {
	m := testModel()
	m.SelectedVisionModel = "a-q8.gguf"

	v, err := m.GetDefaultVisionVariant()
	if err != nil {
		t.Fatalf("GetDefaultVisionVariant: %v", err)
	}
	if v.FileName != "a-q8.gguf" {
		t.Errorf("variant = %q, want a-q8.gguf", v.FileName)
	}
}

func TestGetDefaultVisionVariantErrorsWhenSelectionMissing(t *testing.T) {
	m := testModel()
	m.SelectedVisionModel = "gone.gguf"

	if _, err := m.GetDefaultVisionVariant(); err == nil {
		t.Fatal("expected an error when the selected vision variant is not present")
	}
}

func TestGetDefaultVisionVariantWithoutVisionVariantIsNotAnError(t *testing.T) {
	m := testModel()
	m.Variants = m.Variants[:2]

	v, err := m.GetDefaultVisionVariant()
	if err != nil {
		t.Fatalf("GetDefaultVisionVariant: %v", err)
	}
	if v.FileName != "" {
		t.Errorf("variant = %q, want the zero variant", v.FileName)
	}
}
