package ui

import (
	"errors"
	"testing"

	"mcTUI/internal/i18n"
)

func TestDisplayErrorTranslatesOwnedCopyAndPreservesDetails(t *testing.T) {
	if err := i18n.Init("es"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { i18n.SetLanguage("en") })

	for _, tc := range []struct {
		err  string
		want string
	}{
		{"JAR too large", "El archivo JAR es demasiado grande"},
		{"destination collision: mods/a.jar", "Ya existe un archivo de destino: mods/a.jar"},
		{"instance busy (/game): lock held", "Instancia ocupada (/game): lock held"},
		{"file added at /mods/a.jar but registry failed; refresh: permission denied", "Se agregó el archivo en /mods/a.jar, pero no se pudo actualizar el registro; actualízalo. Detalles: permission denied"},
		{"network timeout", "network timeout"},
	} {
		if got := displayError(errors.New(tc.err)); got != tc.want {
			t.Errorf("displayError(%q) = %q, want %q", tc.err, got, tc.want)
		}
	}
}
