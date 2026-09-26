// Package i18n translates UI keys using embedded English and Spanish dictionaries.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed locales/*.json
var localesFS embed.FS

var (
	mu           sync.RWMutex
	translations = map[string]map[string]string{}
	supported    = []string{"en", "es"}
	currentLang  = "en"
)

// Models can also be constructed directly (e.g. in tests), without main.
func init() {
	if err := Init("en"); err != nil {
		panic(err)
	}
}

// Init loads the embedded dictionaries and selects the persisted language.
func Init(lang string) error {
	loaded := make(map[string]map[string]string, len(supported))
	for _, code := range supported {
		data, err := localesFS.ReadFile("locales/" + code + ".json")
		if err != nil {
			return fmt.Errorf("i18n: read %s: %w", code, err)
		}
		var dict map[string]string
		if err := json.Unmarshal(data, &dict); err != nil {
			return fmt.Errorf("i18n: parse %s: %w", code, err)
		}
		loaded[code] = dict
	}
	mu.Lock()
	defer mu.Unlock()
	translations = loaded
	currentLang = normalize(lang)
	return nil
}

func normalize(lang string) string {
	for _, code := range supported {
		if code == lang {
			return code
		}
	}
	return "en"
}

// SetLanguage selects a supported language, falling back to English.
func SetLanguage(lang string) {
	mu.Lock()
	defer mu.Unlock()
	currentLang = normalize(lang)
}

// CurrentLanguage returns the active language code.
func CurrentLanguage() string {
	mu.RLock()
	defer mu.RUnlock()
	return currentLang
}

// SupportedLanguages returns the available language codes.
func SupportedLanguages() []string { return append([]string(nil), supported...) }

// NextLanguage cycles to the next available language and returns it.
func NextLanguage() string {
	mu.Lock()
	defer mu.Unlock()
	for i, code := range supported {
		if code == currentLang {
			currentLang = supported[(i+1)%len(supported)]
			return currentLang
		}
	}
	currentLang = "en"
	return currentLang
}

// T translates a key in the current language.
func T(key string, args ...any) string {
	mu.RLock()
	defer mu.RUnlock()
	return translate(currentLang, key, args...)
}

// TWithLang translates a key in a specific language, falling back to English.
func TWithLang(lang, key string, args ...any) string {
	mu.RLock()
	defer mu.RUnlock()
	return translate(normalize(lang), key, args...)
}

// translate is called with mu held.
func translate(lang, key string, args ...any) string {
	text := translations[lang][key]
	if text == "" {
		text = translations["en"][key]
	}
	if text == "" {
		return fmt.Sprintf("[missing: %s]", key)
	}
	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}
