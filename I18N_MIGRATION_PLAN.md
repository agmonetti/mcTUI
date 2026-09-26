# Plan de migración y arquitectura técnica: i18n (Internacionalización) en mcTUI

> **Estado:** Propuesta de arquitectura y evaluación de impacto.  
> **Fecha:** 2026-09-26  
> **Idiomas iniciales:** Inglés (`en`, idioma base/fallback) y Español (`es`).  
> **Enfoque técnico:** Estándar Go (`go:embed` + JSON), cero dependencias externas, fallback determinista y persistencia en configuración global.

---

## 1. Opinión técnica y evaluación de impacto (Senior Dev / Ponytail Assessment)

### 1.1. ¿Vale la pena la migración a i18n?
**Sí, absolutamente.**  
Minecraft es un fenómeno global donde las comunidades de habla hispana, portuguesa, francesa, etc., representan una porción masiva de los jugadores de escritorio. mcTUI nació con textos mixtos (menús principales en inglés tipo CS:GO, pero mensajes de error, advertencias de Reset Configs y gestor de instancias en español). Unificar y profesionalizar todos los textos bajo un sistema i18n:
- Da coherencia visual a la aplicación.
- Permite a la comunidad contribuir con nuevas traducciones (JSONs aislados) sin tocar una sola línea de código Go.
- Convierte a mcTUI en una alternativa de primer nivel frente a Prism Launcher o CurseForge App para usuarios de cualquier región.

### 1.2. El factor crítico: ¿Cuándo implementarlo respecto a la Fase 5?
Actualmente nos encontramos a mitad de la **Fase 5 (Instancias y Mods)**:
- 5A (Instancias) y 5B (Mods locales) están cerradas.
- 5C (Modrinth) y 5D (Modpacks) están pendientes de codificación.

Hay dos estrategias de rollout posibles:

| Estrategia | Ventajas | Desventajas | Veredicto |
| --- | --- | --- | --- |
| **A. Big Bang al terminar la Fase 5** | No interrumpe el flujo actual de desarrollo de 5C/5D. | El volumen de strings a extraer al final será enorme y habrá que re-testear todas las pantallas de golpe. | Riesgosa por acumulación de deuda técnica. |
| **B. Habilitar la base (`internal/i18n`) ahora e integrar gradualmente (Recomendada)** | 1. 5C y 5D ya nacen 100% internacionalizadas desde el primer día. 2. Se migran el menú principal y la configuración de inmediato. 3. Las pantallas existentes (5A/5B) se van migrando módulo a módulo sin fricción. | Requiere un pequeño sprint inicial de infraestructura antes de 5C. | **La mejor opción arquitectónica.** |

---

## 2. Decisiones de diseño y políticas cerradas

A partir de la investigación previa (`I18N_INVESTIGATION.md`), se formalizan las siguientes 4 políticas:

### Política 1: Atajos de teclado globales e inmutables
- Los atajos de confirmación (`[y/n]`), navegación (`[Enter]`, `[Esc]`, `[Tab]`, `[Space]`) y acciones (`[a]` agregar, `[d]` eliminar, `[r]` refrescar, `[s]` buscar, `[u]` actualizar) **no cambian según el idioma**.
- *Motivo:* Cambiar `[y/n]` por `[s/n]` en español rompe la memoria muscular de los usuarios de terminal y requeriría condicionar la máquina de estados en `Update()` a la lengua activa. El estándar de la industria (git, docker, prism) mantiene teclas de control universales.

### Política 2: Desacoplamiento de Errores Técnicos vs. Mensajes de Usuario
- **Textos en pantalla (UI):** Se traducen completamente (ej: *"Instancia ocupada"*, *"Descarga fallida"*, *"No se pudo conectar a Modrinth"*).
- **Detalle del error (`err.Error()`):** Se mantiene el mensaje original del sistema operativo o de Go (ej: `permission denied`, `connection refused`, `file exists`).
- *Motivo:* Si un usuario reporta un issue en GitHub, el desarrollador necesita ver el error técnico en su formato original para diagnosticar la causa raíz sin perderse en traducciones arbitrarias.

### Política 3: Retiro definitivo del Roadmap externo
- El panel derecho de "Future Changes" que descargaba dinámicamente `roadmap.json` desde GitHub queda **oficialmente deprecado y eliminado**.
- *Motivo:* mcTUI ya no es un prototipo inicial; el plan de implementación reside en el repositorio (`PHASE_5_IMPLEMENTATION_PLAN.md`). El panel además ya se oculta en terminales estrechas y en pantallas de subflujo. Retirarlo simplifica el modelo, elimina una petición HTTP innecesaria en el arranque y libera espacio horizontal para textos en otros idiomas.

### Política 4: Fallback determinista a inglés (`en`)
- Si una clave no existe en el idioma activo (por ejemplo, una traducción incompleta en español), el motor recurre automáticamente a la versión en inglés.
- Si tampoco existe en inglés, se renderiza la clave en texto plano (`"[missing: menu.play]"`) para que sea evidente durante el desarrollo sin provocar panics ni caídas de la TUI.

---

## 3. Arquitectura técnica del paquete `internal/i18n`

Se descartan librerías de terceros pesadas (`nicksnyder/go-i18n`) o herramientas complejas de generación de código (`golang.org/x/text`). mcTUI utiliza la biblioteca estándar de Go con `embed.FS`:

```text
internal/
└── i18n/
    ├── i18n.go            # Motor de traducción, carga de JSONs y formateo
    ├── i18n_test.go       # Tests unitarios de interpolación y fallbacks
    └── locales/           # Archivos JSON embebidos en el binario
        ├── en.json        # Diccionario base en inglés
        └── es.json        # Diccionario en español
```

### 3.1. Implementación del motor (`internal/i18n/i18n.go`)

```go
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
	translations = make(map[string]map[string]string)
	supported    = []string{"en", "es"}
	currentLang  = "en"
)

// Init inicializa y parsea todos los diccionarios embebidos al arrancar.
func Init(defaultLang string) error {
	mu.Lock()
	defer mu.Unlock()

	for _, lang := range supported {
		data, err := localesFS.ReadFile(fmt.Sprintf("locales/%s.json", lang))
		if err != nil {
			return fmt.Errorf("i18n: fallo al leer locale %s: %w", lang, err)
		}
		var dict map[string]string
		if err := json.Unmarshal(data, &dict); err != nil {
			return fmt.Errorf("i18n: JSON malformado en locale %s: %w", lang, err)
		}
		translations[lang] = dict
	}

	SetLanguage(defaultLang)
	return nil
}

// SetLanguage define el idioma activo. Si no está soportado, usa "en".
func SetLanguage(lang string) {
	for _, s := range supported {
		if s == lang {
			currentLang = s
			return
		}
	}
	currentLang = "en"
}

// CurrentLanguage devuelve el código del idioma activo.
func CurrentLanguage() string {
	return currentLang
}

// SupportedLanguages devuelve la lista de idiomas disponibles.
func SupportedLanguages() []string {
	return supported
}

// NextLanguage rota al siguiente idioma soportado (en -> es -> en).
func NextLanguage() string {
	for i, lang := range supported {
		if lang == currentLang {
			next := supported[(i+1)%len(supported)]
			SetLanguage(next)
			return next
		}
	}
	SetLanguage("en")
	return "en"
}

// T traduce una clave con el idioma actualmente activo.
func T(key string, args ...any) string {
	return TWithLang(currentLang, key, args...)
}

// TWithLang traduce una clave forzando un idioma específico, con fallback a "en".
func TWithLang(lang, key string, args ...any) string {
	mu.RLock()
	defer mu.RUnlock()

	dict, ok := translations[lang]
	if !ok {
		dict = translations["en"]
	}

	text, ok := dict[key]
	if !ok || text == "" {
		// Fallback a inglés si faltaba en el idioma solicitado
		if fallbackDict, hasEn := translations["en"]; hasEn {
			if fallbackText, hasText := fallbackDict[key]; hasText && fallbackText != "" {
				text = fallbackText
			} else {
				return fmt.Sprintf("[missing: %s]", key)
			}
		} else {
			return fmt.Sprintf("[missing: %s]", key)
		}
	}

	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}
```

---

## 4. Persistencia en configuración (`internal/config/config.go`)

Se amplía el struct `Data` para recordar la elección del usuario:

```go
type Data struct {
	Username  string `json:"username"`
	Version   string `json:"version"`
	Modloader string `json:"modloader"`
	MemoryMB  int    `json:"memory_mb"`
	Language  string `json:"language"` // "en" o "es"
}

func defaults() Data {
	return Data{
		Username:  "Player",
		Version:   "1.20.4",
		Modloader: "Vanilla",
		MemoryMB:  2048,
		Language:  "en", // Idioma por defecto en primera ejecución
	}
}
```

Al iniciar `main.go`:
```go
cfg, _ := config.Load()
if err := i18n.Init(cfg.Language); err != nil {
    // Manejo de error o log no fatal
}
```

---

## 5. El desafío del Layout en la terminal (Lipgloss y anchos de columna)

Las terminales no son navegadores web con auto-reflow. El español suele requerir entre un 20% y un 35% más de caracteres que el inglés.

### 5.1. Comparativa de términos y riesgos de layout

| Concepto | Inglés (`en`) | Español (`es`) | Riesgo en mcTUI |
| --- | --- | --- | --- |
| Menú Jugar | `Play (%s)` | `Jugar (%s)` | Bajo (cabe perfectamente). |
| Selector Loader | `Modloader: %s` | `Cargador: %s` | Usar "Cargador" o "Modloader" para no sobrepasar el ancho de 28 caracteres del panel izquierdo. |
| Memoria | `Memory: %d MB` | `Memoria: %d MB` | Bajo. |
| Ajustes | `Settings` | `Ajustes` | Preferir "Ajustes" sobre "Configuraciones" (más corto). |
| Selector de Instancias | `INSTANCES` | `INSTANCIAS` | Tab superior: requiere calcular espaciado de tabs. |
| Estado de archivos | `● FILES PRESENT` | `● ARCHIVOS LISTOS` | Preferir "ARCHIVOS LISTOS" a "ARCHIVOS PRESENTES EN DISCO". |
| Advertencia de mods | `Notice: removing mods may harm worlds` | `Aviso: retirar mods puede dañar mundos` | Mismo largo, renderizar con `wrapText` dinámico. |

### 5.2. Reglas de diseño para diccionarios de traducción
1. **Concisión terminológica:** En español, elegir sinónimos cortos reconocidos por la comunidad técnica ("Ajustes" en vez de "Configuración del sistema", "Borrar" en vez de "Eliminar permanentemente").
2. **Uso de `lipgloss.MaxWidth()` y truncado:** Ningún panel debe asumir un largo de texto estático sin proteger el ancho máximo mediante `MaxWidth()` o wrapping automático para evitar que un salto de línea involuntario rompa la caja del borde.
3. **Separadores dinámicos:** En lugar de `strings.Repeat("─", 46)`, calcular el largo del separador en base al ancho real del panel o componente.

---

## 6. Catálogo maestro de llaves (Estructura de `locales/`)

Para mantener el orden, las claves se organizan jerárquicamente por módulo con prefijos:

```text
menu.*          -> Opciones del menú principal y topbar
status.*        -> Barra de estado inferior y banner destacado
instances.*     -> Creación, selección, renombrado, duplicación y retiro
mods.*          -> Gestor local de mods (5B)
modrinth.*      -> Búsqueda, catálogo, plan de instalación y updates (5C)
reset.*         -> Respaldo y restauración de Reset Configs
common.*        -> Acciones comunes ([y/n], Cancelar, Aceptar, Volver)
errors.*        -> Mensajes de error en pantalla
```

### 6.1. Ejemplo: `locales/en.json`
```json
{
  "menu.options": "OPTIONS",
  "menu.play": "Play (%s)",
  "menu.change_name": "Change Name",
  "menu.worlds": "Worlds",
  "menu.modloader": "Modloader: %s",
  "menu.memory": "Memory: %d MB",
  "menu.reset": "Reset Configs",
  "menu.java": "Java: %s",
  "menu.language": "Language: %s",
  "menu.quit": "Quit",

  "topbar.play": "PLAY",
  "topbar.version": "VERSION",
  "topbar.instances": "INSTANCES",
  "topbar.mods": "MODS",

  "status.files_present": "● FILES PRESENT · verify before Play",
  "status.files_missing": "● FILES MISSING",

  "mods.title": "LOCAL MODS",
  "mods.confirm_delete_title": "CONFIRM DELETE",
  "mods.confirm_delete_desc": "Config and dependencies are preserved.",
  "mods.add_title": "ADD MOD (.jar)",
  "mods.enabled": "Enabled",
  "mods.disabled": "Disabled",
  "mods.unknown": "Unknown",
  "mods.corrupt": "Corrupt",
  "mods.duplicate": "Duplicate",
  "mods.modified": "Modified",

  "modrinth.catalog_title": "Modrinth Catalog — Instance: \"%s\"",
  "modrinth.search_label": "Search: %s",
  "modrinth.filter_status": "[Loader: %s] [MC: %s] [Channel: %s]",
  "modrinth.plan_title": "Installation Plan — Modrinth",
  "modrinth.required_deps": "Required dependencies (will be downloaded automatically):",
  "modrinth.optional_deps": "Optional dependencies available:",
  "modrinth.no_conflicts": "None.",
  "modrinth.confirm_prompt": "Confirm installation and download? [y / n]",
  "modrinth.updates_title": "Available Updates (%d mods)",

  "common.confirm": "[y] Confirm",
  "common.cancel": "[n/Esc] Cancel",
  "common.back": "[Esc] Back"
}
```

### 6.2. Ejemplo: `locales/es.json`
```json
{
  "menu.options": "OPCIONES",
  "menu.play": "Jugar (%s)",
  "menu.change_name": "Cambiar Nombre",
  "menu.worlds": "Mundos",
  "menu.modloader": "Cargador: %s",
  "menu.memory": "Memoria: %d MB",
  "menu.reset": "Reset Configs",
  "menu.java": "Java: %s",
  "menu.language": "Idioma: %s",
  "menu.quit": "Salir",

  "topbar.play": "JUGAR",
  "topbar.version": "VERSIÓN",
  "topbar.instances": "INSTANCIAS",
  "topbar.mods": "MODS",

  "status.files_present": "● ARCHIVOS LISTOS · verificar antes de Jugar",
  "status.files_missing": "● FALTAN ARCHIVOS",

  "mods.title": "MODS LOCALES",
  "mods.confirm_delete_title": "CONFIRMAR ELIMINACIÓN",
  "mods.confirm_delete_desc": "Las configuraciones y dependencias se conservan.",
  "mods.add_title": "AGREGAR MOD (.jar)",
  "mods.enabled": "Activo",
  "mods.disabled": "Desactivado",
  "mods.unknown": "Desconocido",
  "mods.corrupt": "Corrupto",
  "mods.duplicate": "Duplicado",
  "mods.modified": "Modificado",

  "modrinth.catalog_title": "Catálogo Modrinth — Instancia: \"%s\"",
  "modrinth.search_label": "Buscar: %s",
  "modrinth.filter_status": "[Cargador: %s] [MC: %s] [Canal: %s]",
  "modrinth.plan_title": "Plan de instalación — Modrinth",
  "modrinth.required_deps": "Dependencias obligatorias (se descargarán automáticamente):",
  "modrinth.optional_deps": "Dependencias opcionales disponibles:",
  "modrinth.no_conflicts": "Ninguna.",
  "modrinth.confirm_prompt": "¿Confirmar instalación y descarga? [y / n]",
  "modrinth.updates_title": "Actualizaciones disponibles (%d mods)",

  "common.confirm": "[y] Confirmar",
  "common.cancel": "[n/Esc] Cancelar",
  "common.back": "[Esc] Volver"
}
```

---

## 7. Plan de trabajo estructurado por etapas

Para evitar dispersión y mantener la estabilidad de la rama `main`:

### Etapa 1: Infraestructura y saneamiento (Cero impacto en vistas)
1. Crear el paquete `internal/i18n` con `i18n.go`, `i18n_test.go`, y `locales/{en,es}.json`.
2. Agregar el campo `Language` en `internal/config/config.go` con persistencia automática.
3. Eliminar el paquete `internal/roadmap` y las llamadas a `roadmap.json` para limpiar el modelo.
4. Agregar la opción "Language: English / Idioma: Español" en el menú principal (`view.go` y `update.go`).

### Etapa 2: Migración del Menú Principal, Topbar y Footer
1. Reemplazar los textos fijos del panel izquierdo, la topbar y el footer con llamadas a `i18n.T()`.
2. Probar visualmente la alternancia entre `en` y `es` con la tecla asignada (Enter sobre Language o tecla rápida).
3. Asegurar que ningún panel se desplace vertical ni horizontalmente al cambiar de idioma.

### Etapa 3: Nuevas pantallas de la Fase 5C (Desarrollo nativo en i18n)
- Todas las pantallas de Modrinth (`modrinth_search.go`, `modrinth_detail.go`, `modrinth_review.go`, `modrinth_updates.go`) se escriben **directamente** utilizando `i18n.T()` para todos sus textos, banners y encabezados.

### Etapa 4: Migración de pantallas existentes de 5A y 5B
- Extraer cadenas de `instances.go`, `copy.go`, `remove.go` y `mods.go`.
- Validar el layout de confirmación de borrado y selección de archivos.

---

## 8. Conclusión y recomendación de ejecución

La migración a i18n es **totalmente viable, limpia y de gran valor para mcTUI**.

Implementar la **Etapa 1** ahora mismo dejará listos los cimientos para que la **Fase 5C (Modrinth)** se construya de forma limpia e internacionalizada desde su primera línea de código, evitando tener que refactorizarla más adelante.
