package ui

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"mcTUI/internal/i18n"
)

func TestSpanishSecondaryViews(t *testing.T) {
	if err := i18n.Init("es"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { i18n.SetLanguage("en") })
	m := Model{state: instancesScreen, width: 100, height: 30}
	if got := m.renderInstancesListContent(); !strings.Contains(got, "ELEGIR INSTANCIA") || !strings.Contains(got, "No hay instancias disponibles.") {
		t.Fatalf("instance secondary view not translated: %s", got)
	}
	m.state, m.resetDir, m.resetInstance = resetModeScreen, "/minecraft", "Prueba"
	if got := strings.Join(m.resetLines(), "\n"); !strings.Contains(got, "INSTALACIÓN") || !strings.Contains(got, "Directorio de Minecraft: /minecraft") {
		t.Fatalf("reset secondary view not translated: %s", got)
	}
}

func TestEveryTranslationKeyExistsInAllCatalogs(t *testing.T) {
	root := filepath.Join("..", "..")
	used := map[string]bool{}
	fset := token.NewFileSet()
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "T" {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			if !ok || pkg.Name != "i18n" {
				return true
			}
			key, ok := call.Args[0].(*ast.BasicLit)
			if !ok || key.Kind != token.STRING {
				return true
			}
			var value string
			if err := json.Unmarshal([]byte(key.Value), &value); err == nil {
				used[value] = true
			}
			return true
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// These keys are formed from folder status values at runtime.
	for _, status := range []string{"untouched", "moved", "restored", "deleted", "partially_deleted", "failed"} {
		used["reset.folder_"+status] = true
	}
	placeholder := regexp.MustCompile(`%[+#0 -]*[0-9]*(?:\.[0-9]+)?[a-zA-Z%]`)
	var catalogs []map[string]string
	for _, lang := range []string{"en", "es"} {
		data, err := os.ReadFile(filepath.Join(root, "internal", "i18n", "locales", lang+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var catalog map[string]string
		if err := json.Unmarshal(data, &catalog); err != nil {
			t.Fatal(err)
		}
		catalogs = append(catalogs, catalog)
		var missing []string
		for key := range used {
			if _, ok := catalog[key]; !ok {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Errorf("%s catalog missing keys: %v", lang, missing)
		}
	}
	for key := range used {
		if en, es := placeholder.FindAllString(catalogs[0][key], -1), placeholder.FindAllString(catalogs[1][key], -1); len(en) != len(es) {
			t.Errorf("format placeholder count differs for %q: en %v, es %v", key, en, es)
		} else {
			for i := range en {
				if en[i] != es[i] {
					t.Errorf("format placeholders differ for %q: en %v, es %v", key, en, es)
					break
				}
			}
		}
	}
}
