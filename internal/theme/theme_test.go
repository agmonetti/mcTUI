package theme

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinThemes(t *testing.T) {
	themes := BuiltinThemes()
	if len(themes) < 4 {
		t.Fatalf("expected at least 4 built-in themes, got %d", len(themes))
	}

	def := Default()
	if def.ID != DefaultThemeID {
		t.Fatalf("expected default theme ID %q, got %q", DefaultThemeID, def.ID)
	}

	for _, th := range themes {
		if th.ID == "" || th.Name == "" || th.Accent == "" || th.Border == "" {
			t.Errorf("theme %+v is missing required fields", th)
		}
	}
}

func TestNormalize(t *testing.T) {
	th := Theme{
		ID:   "minimal",
		Name: "Minimal Theme",
	}
	th.Normalize()

	def := Default()
	if th.Border != def.Border || th.Accent != def.Accent || th.White != def.White {
		t.Fatalf("expected missing colors to fallback to default theme, got %+v", th)
	}
	if th.TabActive != th.Accent {
		t.Fatalf("expected TabActive to equal Accent, got %q", th.TabActive)
	}
	if th.TabInact != th.Gray {
		t.Fatalf("expected TabInact to equal Gray, got %q", th.TabInact)
	}
}

func TestLoadUserThemesAndGet(t *testing.T) {
	tmpDir := t.TempDir()

	customTheme := Theme{
		ID:          "synthwave",
		Name:        "Synthwave 84",
		Description: "Neon 80s aesthetic",
		Border:      "#ff7edb",
		BorderHi:    "#36f9f6",
		White:       "#f92aad",
		Gray:        "#b880ff",
		DimGray:     "#463465",
		Accent:      "#fe4450",
		Accent2:     "#ff8b39",
		Green:       "#72f1b8",
		Red:         "#fe4450",
		Blue:        "#36f9f6",
	}

	data, err := json.Marshal(customTheme)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	jsonPath := filepath.Join(tmpDir, "synthwave.json")
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		t.Fatalf("write file error: %v", err)
	}

	// Write invalid json to verify error tolerance
	badPath := filepath.Join(tmpDir, "invalid.json")
	_ = os.WriteFile(badPath, []byte("{invalid json"), 0644)

	userThemes := LoadUserThemes(tmpDir)
	if len(userThemes) != 1 {
		t.Fatalf("expected 1 valid user theme, got %d", len(userThemes))
	}

	if userThemes[0].ID != "synthwave" {
		t.Fatalf("expected ID synthwave, got %s", userThemes[0].ID)
	}

	// Test List
	all := List(tmpDir)
	found := false
	for _, th := range all {
		if th.ID == "synthwave" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("synthwave not found in List(tmpDir)")
	}

	// Test Get
	got := Get("synthwave", tmpDir)
	if got.Name != "Synthwave 84" {
		t.Fatalf("expected name Synthwave 84, got %s", got.Name)
	}

	// Test Get fallback
	fallback := Get("non-existent-theme-id", tmpDir)
	if fallback.ID != DefaultThemeID {
		t.Fatalf("expected fallback to default %q, got %q", DefaultThemeID, fallback.ID)
	}
}
