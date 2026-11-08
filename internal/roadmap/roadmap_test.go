package roadmap

import (
	"testing"

	"mcTUI/internal/i18n"
)

func TestEmbeddedRoadmap(t *testing.T) {
	rm := Embedded()
	if rm.Version != "2.0" {
		t.Fatalf("expected version 2.0, got %s", rm.Version)
	}
	if len(rm.Items) < 3 {
		t.Fatalf("expected at least 3 items, got %d", len(rm.Items))
	}

	for _, it := range rm.Items {
		if it.ID == "" || it.Status == "" || it.TitleEN == "" || it.TitleES == "" {
			t.Errorf("item %+v missing fields", it)
		}
	}
}

func TestItemLocalization(t *testing.T) {
	_ = i18n.Init("en")

	item := Item{
		ID:      "test_feature",
		TitleEN: "English Title",
		TitleES: "Título en Español",
		DescEN:  "English Description",
		DescES:  "Descripción en Español",
	}

	if item.Title() != "English Title" {
		t.Fatalf("expected English Title, got %s", item.Title())
	}
	if item.Description() != "English Description" {
		t.Fatalf("expected English Description, got %s", item.Description())
	}

	_ = i18n.Init("es")
	if item.Title() != "Título en Español" {
		t.Fatalf("expected Spanish Title, got %s", item.Title())
	}
	if item.Description() != "Descripción en Español" {
		t.Fatalf("expected Spanish Description, got %s", item.Description())
	}

	// Restore
	_ = i18n.Init("en")
}
