package i18n

import (
	"strings"
	"sync"
	"testing"
)

func TestTranslationAndFallback(t *testing.T) {
	if err := Init("es"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { SetLanguage("en") })
	if got := T("menu.play", "1.20.4"); got != "Jugar (1.20.4)" {
		t.Fatalf("Spanish interpolation: %q", got)
	}
	if got := TWithLang("en", "menu.memory", 2048); got != "Memory: 2048 MB" {
		t.Fatalf("English interpolation: %q", got)
	}
	if got := TWithLang("fr", "menu.quit"); got != "Quit" {
		t.Fatalf("unsupported language fallback: %q", got)
	}
	if got := T("nonexistent.key"); got != "[missing: nonexistent.key]" {
		t.Fatalf("missing key: %q", got)
	}
	if got := NextLanguage(); got != "en" || CurrentLanguage() != "en" {
		t.Fatalf("language cycle: %q", got)
	}
	if got := NextLanguage(); got != "es" {
		t.Fatalf("language cycle: %q", got)
	}
	SetLanguage("invalid")
	if CurrentLanguage() != "en" {
		t.Fatal("unsupported language must select English")
	}
	codes := SupportedLanguages()
	codes[0] = "changed"
	if SupportedLanguages()[0] != "en" {
		t.Fatal("supported languages must not be mutable by callers")
	}
}

func TestMissingSpanishKeyFallsBack(t *testing.T) {
	if err := Init("es"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Init("en") })
	mu.Lock()
	delete(translations["es"], "menu.quit")
	mu.Unlock()
	if got := T("menu.quit"); got != "Quit" {
		t.Fatalf("missing Spanish translation: %q", got)
	}
}

func TestConcurrentLanguageChangeAndTranslation(t *testing.T) {
	if err := Init("en"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				NextLanguage()
				if got := T("menu.quit"); got != "Quit" && got != "Salir" {
					t.Errorf("unexpected translation: %q", got)
				}
			}
		}()
	}
	wg.Wait()
	if got := TWithLang("es", "modrinth.catalog_title", "A"); !strings.Contains(got, "A") {
		t.Fatalf("catalog interpolation: %q", got)
	}
}
