// Package theme provides color theme definitions, built-in presets,
// and user theme loading for mcTUI.
package theme

import (
	"strings"
)

// DefaultThemeID is the ID of the default theme ("mctui").
const DefaultThemeID = "mctui"

// Theme defines the complete color palette and metadata for the mcTUI interface.
type Theme struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsDark      bool   `json:"is_dark"`

	// Structural borders and highlights
	Border   string `json:"border"`
	BorderHi string `json:"border_hi"`

	// Typography & content
	White   string `json:"white"`
	Gray    string `json:"gray"`
	DimGray string `json:"dim_gray"`

	// Accents & focus
	Accent  string `json:"accent"`
	Accent2 string `json:"accent2"`

	// Status & functional colors
	Green string `json:"green"`
	Red   string `json:"red"`
	Blue  string `json:"blue"`

	// Navigation tabs
	TabActive string `json:"tab_active,omitempty"`
	TabInact  string `json:"tab_inactive,omitempty"`
}

// Normalize ensures all required tokens are non-empty, falling back
// to the default theme values when fields are missing or empty.
func (t *Theme) Normalize() {
	def := Default()

	if strings.TrimSpace(t.ID) == "" {
		t.ID = "custom"
	}
	if strings.TrimSpace(t.Name) == "" {
		t.Name = t.ID
	}
	if strings.TrimSpace(t.Border) == "" {
		t.Border = def.Border
	}
	if strings.TrimSpace(t.BorderHi) == "" {
		t.BorderHi = def.BorderHi
	}
	if strings.TrimSpace(t.White) == "" {
		t.White = def.White
	}
	if strings.TrimSpace(t.Gray) == "" {
		t.Gray = def.Gray
	}
	if strings.TrimSpace(t.DimGray) == "" {
		t.DimGray = def.DimGray
	}
	if strings.TrimSpace(t.Accent) == "" {
		t.Accent = def.Accent
	}
	if strings.TrimSpace(t.Accent2) == "" {
		t.Accent2 = def.Accent2
	}
	if strings.TrimSpace(t.Green) == "" {
		t.Green = def.Green
	}
	if strings.TrimSpace(t.Red) == "" {
		t.Red = def.Red
	}
	if strings.TrimSpace(t.Blue) == "" {
		t.Blue = def.Blue
	}
	if strings.TrimSpace(t.TabActive) == "" {
		t.TabActive = t.Accent
	}
	if strings.TrimSpace(t.TabInact) == "" {
		t.TabInact = t.Gray
	}
}
