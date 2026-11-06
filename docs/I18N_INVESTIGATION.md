# Investigación: Implementación de i18n (Internacionalización) en mcTUI

Implementar i18n para alternar y persistir diferentes idiomas en mcTUI es un objetivo excelente para mejorar la accesibilidad, pero requiere cambios sistemáticos en toda la interfaz. A continuación, se detalla qué conllevaría esta implementación, sus riesgos y la estrategia recomendada.

## 1. Modificaciones Estructurales Requeridas

### A. Persistencia de la Configuración
- **Archivo `internal/config/config.go`**: Habrá que extender el struct `Data` para incluir un nuevo campo `Language string` (por ejemplo, "en", "es").
- **Valores por defecto**: Actualizar `defaults()` para que devuelva un idioma base si es la primera vez que se ejecuta el programa.
- Como la configuración ya se guarda en disco (en formato JSON dentro de `AppData` o `~/.config`), el cambio en la persistencia será transparente y automático.

### B. Inyección en el Modelo (Bubble Tea)
- **Archivo `internal/ui/model.go`**: El struct `Model`, que es el corazón de la TUI, necesitará acceso al idioma actual y a un diccionario/traductor.
- Habrá que pasar el idioma seleccionado a través de la función constructora `ui.New()` o mantener una referencia global.
- Crear una nueva opción en el menú principal para rotar entre los idiomas disponibles ("Language: English", "Idioma: Español") que actualice el `Model` y guarde la configuración.

### C. Refactorización de Vistas
- **Archivo `internal/ui/view.go`**: Aquí reside el mayor volumen de trabajo. Actualmente, la TUI tiene docenas de cadenas de texto en duro (ej: `"Play"`, `"Modloader: "`, `"Memory: "`, `"PERMANENT DELETION:..."`).
- Todas estas cadenas deberán ser reemplazadas por una función traductora, por ejemplo: `m.T("menu.play")` o `T("Play", m.Language)`.
- Se deberán adaptar las cadenas que usan variables. Por ejemplo: `fmt.Sprintf("Play (%s)", m.versionSelect)` pasará a requerir interpolación según el idioma.

## 2. Estrategias Técnicas Disponibles

Para un proyecto Go como mcTUI (que se compila en un único ejecutable), hay varias formas de manejar los diccionarios de traducción:

1. **`go:embed` + JSON (Recomendado)**:
   - Se crean archivos como `locales/en.json` y `locales/es.json`.
   - Se incrustan en el binario final utilizando la directiva `go:embed`. Esto es fundamental para que el usuario siga descargando un solo archivo `.exe` o binario de Linux sin depender de carpetas de recursos externos.
   - Es ligero, fácil de leer para futuros contribuidores que quieran traducir la app a otros idiomas, y requiere poco código adicional.

2. **Librerías estándar de Go (`golang.org/x/text`)**:
   - Es el estándar "oficial" pero puede resultar muy pesado (over-engineering) para una TUI sencilla, además de requerir herramientas de generación de código (`gotext`).

3. **Librería de terceros (`nicksnyder/go-i18n`)**:
   - Robusta, soporta pluralización fácilmente, pero añade una dependencia más al proyecto.

## 3. Riesgos, Dificultades y Edge Cases

### A. Rotura de Layout (Diseño y Anchos)
- **Problema de Longitud**: Las palabras en español y otros idiomas suelen ser más largas que en inglés. Por ejemplo, "Settings" vs "Configuración".
- En `view.go`, mcTUI hace uso de anchos explícitos para renderizar paneles (ej. `Width(28)` o `Width(64)`) y de separadores fijos (`strings.Repeat("─", 24)`).
- **Riesgo**: Si un texto traducido excede ese ancho, `lipgloss` hará un salto de línea (wrap), lo que romperá el diseño de las cajas de la terminal. Habrá que repasar y dinamizar algunos márgenes.

### B. Interacción del Usuario y Atajos (Teclas)
- En `internal/ui/update.go`, existen flujos que esperan teclas precisas. Por ejemplo, al abortar un proceso o confirmar acciones:
  ```go
  case "y", "Y":
      m.gameKillConfirm = false
  case "n", "N": ...
  ```
- **Decisión de UX**: ¿Se internacionalizan también los atajos de teclado? (ej. cambiar a `s/S` para "Sí" en español) o ¿se mantiene `y/n` de forma universal? Cambiarlo requiere que la lógica del `Update()` en Bubble Tea dependa del idioma actual, lo cual añade complejidad. Rta: No, esos atajos de teclado son globales, no son dependientes de la region, es un idioma global.

### C. Contenido Externo: El Roadmap
- En `internal/roadmap/roadmap.go`, las novedades ("Future Changes") se obtienen dinámicamente de un archivo en bruto en GitHub: `https://raw.githubusercontent.com/agmonetti/mcTUI/main/roadmap.json`.
- Al estar en un servidor externo y no en el binario, si queremos traducir esto habría que:
  1. Hospedar múltiples archivos (`roadmap_en.json`, `roadmap_es.json`) en el repo.
  2. Modificar la URL de descarga según el `Language` del usuario en runtime.
Rta: Eliminar ese contenido externo de roadmap, redundante, va a quedar inutilizado.

### D. Errores Técnicos (Logs vs Pantalla)
- Hay errores manejados por el sistema (ej. fallo al copiar instancias, permisos denegados de Windows/Linux) y errores definidos en el código (ej. `"Cannot inspect installation..."`).
- Traducir todos los errores técnicos puede dificultar el soporte/debugging cuando un usuario envíe un issue en GitHub en otro idioma. Se suele recomendar traducir los mensajes amigables (ej. "La descarga falló") pero mantener los logs detallados (`err.Error()`) originales. Rta: Me parece correcta esta consideracion, de acuerdo con vos.

## 4. Diseño Técnico a Nivel de Código (Profundización)

Para responder concretamente a tus dudas: **Sí, es estrictamente necesario desacoplar todos los strings estáticos de la interfaz**. Los textos ya no vivirán en `view.go`, sino en archivos JSON externos que la aplicación leerá en memoria al arrancar.

A continuación, se detalla la arquitectura exacta a nivel de código para lograr esto.

### A. Nueva Estructura de Directorios
Se deberá crear un nuevo paquete interno (`internal/i18n`) para aislar la lógica de traducción y alojar los diccionarios.

```text
mcTUI/
├── internal/
│   ├── i18n/
│   │   ├── i18n.go            <-- Lógica de parseo e interpolación
│   │   └── locales/           <-- Directorio embebido
│   │       ├── en.json
│   │       └── es.json
```

### B. Desacoplamiento de Textos (Ejemplo de JSON)
Extraeremos los textos de la TUI hacia llaves (`keys`).

**`internal/i18n/locales/en.json`**:
```json
{
  "menu.play": "Play (%s)",
  "menu.modloader": "Modloader: %s",
  "menu.memory": "Memory: %d MB",
  "menu.language": "Language: English",
  "error.download_failed": "Download failed: %s"
}
```
**`internal/i18n/locales/es.json`**:
```json
{
  "menu.play": "Jugar (%s)",
  "menu.modloader": "Gestor de mods: %s",
  "menu.memory": "Memoria: %d MB",
  "menu.language": "Idioma: Español",
  "error.download_failed": "Descarga fallida: %s"
}
```

### C. Código del Paquete `i18n` (Uso de `go:embed`)
Se escribirá un gestor súper ligero que cargue los JSON en un mapa de Go en tiempo de compilación. No requiere archivos externos al distribuir.

**`internal/i18n/i18n.go`**:
```go
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed locales/*.json
var localeFiles embed.FS

var translations = make(map[string]map[string]string)

// Init carga los JSON a memoria al arrancar la app.
func Init() error {
	langs := []string{"en", "es"}
	for _, lang := range langs {
		data, err := localeFiles.ReadFile(fmt.Sprintf("locales/%s.json", lang))
		if err != nil {
			return err
		}
		var dict map[string]string
		if err := json.Unmarshal(data, &dict); err != nil {
			return err
		}
		translations[lang] = dict
	}
	return nil
}

// T devuelve la cadena traducida e interpola variables si es necesario.
func T(lang, key string, args ...interface{}) string {
	dict, ok := translations[lang]
	if !ok {
		dict = translations["en"] // fallback
	}
	str, ok := dict[key]
	if !ok {
		return key // si no existe la llave, la muestra tal cual
	}
	if len(args) > 0 {
		return fmt.Sprintf(str, args...)
	}
	return str
}
```

### D. Cambios Exactos en la Configuración
**`internal/config/config.go`**:
```go
type Data struct {
	Username  string `json:"username"`
	Version   string `json:"version"`
	Modloader string `json:"modloader"`
	MemoryMB  int    `json:"memory_mb"`
	Language  string `json:"language"` // <-- NUEVO
}

func defaults() Data {
	return Data{
		Username:  "Player",
		Version:   "1.20.4",
		Modloader: "Vanilla",
		MemoryMB:  2048,
		Language:  "en", // <-- VALOR BASE
	}
}
```

### E. Impacto en el Modelo y la Vista
El modelo guardará el idioma y utilizará la función `T` en el método `View()`.

**Antes (`internal/ui/view.go`)**:
```go
menuOptions := []string{
    fmt.Sprintf("Play (%s)", m.versionSelect),
    fmt.Sprintf("Modloader: %s", lipgloss.NewStyle().Foreground(colorAccent2).Render(loaderLabel)),
    fmt.Sprintf("Memory: %d MB", m.memoryMB),
}
```

**Después (`internal/ui/view.go`)**:
```go
lang := m.config.Language // o m.Language

menuOptions := []string{
    // Interpola la versión directamente sobre el string traducido
    i18n.T(lang, "menu.play", m.versionSelect), 
    
    // Aplica color al label *antes* de interpolar en el string traducido
    i18n.T(lang, "menu.modloader", lipgloss.NewStyle().Foreground(colorAccent2).Render(loaderLabel)), 
    
    i18n.T(lang, "menu.memory", m.memoryMB),
    
    i18n.T(lang, "menu.language"), // Nueva opción
}
```

### F. Saneamiento General
1. **`main.go`**: Llamar a `i18n.Init()` al inicio y remover `roadmap.Load()`.
2. **`internal/ui/model.go` y `update.go`**: Remover los estados, variables y vistas asociadas al roadmap dinámico.
3. Actualizar la variable const `menuOptionsCount` en `model.go` de `9` a `10` (por la nueva opción de Language).

---
## 5. Resumen de Acción Sugerida
Si decides avanzar con esto, los pasos ideales (fases) serían:
1. Crear el sistema de `locales/` y agregar el cargador de traducciones básico con soporte de variables (JSON simple).
2. Agregar `Language` a la configuración (`internal/config/config.go`).
3. Refactorizar `view.go` iterativamente para que consuma los textos del diccionario.
4. Ajustar los problemas visuales de padding/width que surjan con los nuevos textos.
