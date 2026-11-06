# Fase 5C — Especificación técnica detallada: Modrinth, dependencias y actualizaciones

> **Estado:** Especificación aprobada; implementación 5C en curso, **no cerrada** (estado del árbol local sin commit de cierre).  
> **Fecha de especificación:** 2026-09-26.  
> **Base:** Fases 5A y 5B cerradas (`0e4685f`, `d6d906e`); las comprobaciones de 5C descritas abajo son del árbol local.  
> **Decisiones resueltas asociadas:** `DEC-05` (versión/loader fijos), `DEC-06` (retención/rollback de updates) y `DEC-10` (límites de red, seguridad, hashes y staging).  
> **Objetivo:** Catálogo integrado de Modrinth, resolución determinista de dependencias, descargas verificadas por SHA-512, actualización masiva eficiente (batch) y recuperación/rollback de versiones reemplazadas, todo integrado en la TUI sin manipulaciones externas.

---

## Índice

1. [Arquitectura y diseño de componentes](#1-arquitectura-y-diseño-de-componentes)
2. [Contrato de red y cliente API de Modrinth (C01, DEC-10)](#2-contrato-de-red-y-cliente-api-de-modrinth-c01-dec-10)
3. [Identificación por hash y content-addressing](#3-identificación-por-hash-y-content-addressing)
4. [Búsqueda y selección de versiones compatibles (C02–C03)](#4-búsqueda-y-selección-de-versiones-compatibles-c02c03)
5. [Grafo de dependencias y plan de instalación (C04–C06, C12)](#5-grafo-de-dependencias-y-plan-de-instalación-c04c06-c12)
6. [Descarga segura, staging y verificación atómica (C07, DEC-10)](#6-descarga-segura-staging-y-verificación-atómica-c07-dec-10)
7. [Política de actualizaciones, canales y rollback (C08–C11, DEC-06)](#7-política-de-actualizaciones-canales-y-rollback-c08c11-dec-06)
8. [Evolución del esquema `mods.json`](#8-evolución-del-esquema-modsjson)
9. [Diseño de pantallas y navegación en la TUI](#9-diseño-de-pantallas-y-navegación-en-la-tui)
10. [Matriz de pruebas y validación](#10-matriz-de-pruebas-y-validación)
11. [Estado actual, brechas y criterios de cierre](#11-estado-actual-brechas-y-criterios-de-cierre)

---

## 1. Arquitectura y diseño de componentes

La Fase 5C se distribuye entre cliente/resolutor, almacenamiento local y TUI dentro de `internal/`. Los archivos existentes en el árbol local son:

```text
internal/
├── modrinth/                  # Cliente HTTP, tipos API v2 y resolutor
│   ├── client.go              # Cliente HTTP con User-Agent, timeouts, rate limits y 429
│   ├── types.go               # Estructuras JSON para Project, Version, File, Dependency, Search
│   ├── search.go              # Búsqueda paginada con facets y ordenamiento
│   ├── versions.go            # Consulta de versiones, filtrado y resolución de compatibles
│   ├── hashes.go              # Endpoints batch version_files y version_files/update
│   ├── resolver.go            # Grafo de dependencias: required, optional, incompatible, embedded
│   └── download.go            # Descarga en staging con SHA-512 y callback de bytes
├── mods/                      # Gestor local extendido desde 5B
│   ├── mods.go                # Store, escaneo y registro schema_version: 2
│   ├── install.go             # Instalación del conjunto bajo lock
│   ├── recovery.go            # Journal y recuperación de instalaciones interrumpidas
│   └── rollback.go            # Historial, retención y reversión manual
└── ui/
    ├── mods.go                # Atajos de catálogo, canal, updates y rollback
    └── modrinth.go            # Búsqueda, detalle, revisión, updates y estados Bubble Tea
```

### Principios rectores (filosofía mcTUI):
- **Aislamiento absoluto:** Todo mod instalado o actualizado se aplica exclusivamente sobre la instancia activa (`gameDir`).
- **Verificación criptográfica estricta:** Ningún JAR entra a `mods/` sin comprobar su hash SHA-512 contra el valor oficial de Modrinth.
- **Sin sorpresas en disco:** La TUI muestra siempre el plan completo de instalación (mod principal + dependencias obligatorias + opcionales) antes de descargar un solo byte.
- **Recuperación verificable:** Las actualizaciones preservan el JAR previo y registran un journal antes de publicar cambios. Una secuencia de renombres **no** es una transacción multiarchivo; si la recuperación no puede verificar un archivo modificado externamente, se detiene y conserva journal y archivos para intervención explícita.

---

## 2. Contrato de red y cliente API de Modrinth (C01, DEC-10)

### 2.1. Dominios y protocolos autorizados (`DEC-10`)
- **API REST:** `https://api.modrinth.com/v2`
- **Descargas de binarios:** `https://cdn.modrinth.net`
- **Regla de hosts:** Cualquier redirección HTTP o URL fuera de estos dos dominios exactos HTTPS debe ser **rechazada inmediatamente con error**. No se permiten protocolos inseguros (`http://`), URLs de red local o dominios externos en esta fase.

### 2.2. Cabecera `User-Agent` canónica obligatoria
Modrinth exige en su documentación oficial que todo cliente se identifique claramente. El formato adoptado para mcTUI es:
```http
User-Agent: agmonetti/mcTUI/1.0.0 (https://github.com/agmonetti/mcTUI)
```
Si se detecta un `User-Agent` vacío o genérico, el cliente de Go debe abortar antes de emitir la llamada para evitar bloqueos por IP.

### 2.3. Rate Limiting y tratamiento de errores 429
- Modrinth impone un límite de **300 solicitudes por minuto por dirección IP**.
- En cada respuesta, el cliente lee e interpreta los encabezados:
  - `X-Ratelimit-Limit`: Cantidad total permitida en la ventana.
  - `X-Ratelimit-Remaining`: Solicitudes restantes.
  - `X-Ratelimit-Reset`: Segundos restantes hasta que se restablezca el contador.
- **Comportamiento ante HTTP 429 (Too Many Requests):**
  - Si se recibe un 429, el cliente no realiza peticiones inmediatas.
  - Se lee la cabecera `Retry-After` (o `X-Ratelimit-Reset`).
  - Para peticiones interactivas (búsqueda): se informa a la TUI de inmediato: *"Límite de peticiones de Modrinth alcanzado. Espere X segundos antes de reintentar"*.
  - Para peticiones batch en segundo plano: backoff exponencial acotado (máximo 2 reintentos antes de devolver error).

### 2.4. Timeouts y límites de tamaño
- **Timeout para llamadas REST:** 15 segundos mediante `context.WithTimeout`.
- **Timeout para descargas de JARs:** 5 minutos por archivo o cancelación inmediata si el usuario presiona `Esc`.
- **Tope de tamaño de archivo (`DEC-10`):** Máximo **250 MiB** por archivo `.jar`. Si la cabecera `Content-Length` o el flujo de bytes excede este límite, la descarga se cancela con error de seguridad.
- **Tope de respuestas JSON:** Máximo **10 MiB** para evitar ataques de consumo de memoria al deserializar payloads malformados.

---

## 3. Identificación por hash y content-addressing

En lugar de depender exclusivamente de nombres de archivo o slugs (que son mutables), mcTUI adopta el modelo de **content-addressing** probado por Prism Launcher.

### 3.1. Hashing local al escanear o importar
`mods.Store.Scan` calcula SHA-512 en streaming junto al SHA-256 para archivos locales/5B y externos; los JARs descargados de Modrinth se verifican con SHA-512. No atribuir procedencia Modrinth por nombre, slug ni por una coincidencia sin verificar el hash actual.

### 3.2. Descubrimiento de mods huérfanos / externos
Si un mod fue copiado manualmente a `mods/` o proviene de 5B sin metadata de Modrinth:
- Endpoint: `POST https://api.modrinth.com/v2/version_files`
- Payload:
  ```json
  {
    "hashes": ["<sha512_en_minusculas>"],
    "algorithm": "sha512"
  }
  ```
- Modrinth responde con un mapa `hash -> Version`. Si existe coincidencia:
  - Se vincula el mod con su `project_id`, `version_id`, `version_number` y autor.
  - Vincular `project_id`, `version_id` y hash solo si el archivo no cambió entre hashing y persistencia, bajo el lock de la instancia; conservar la procedencia local/externa si la API no encuentra coincidencia o no hay conexión. La identificación nunca debe impedir gestión local o Play offline.
  - `Store.Identify` se ejecuta al chequear actualizaciones: consulta los hashes de los mods sin identidad, revalida archivo y registro bajo el lock antes de persistir los metadatos, y mantiene el origen `local`/`external`. Tests cubren mod reconocido, desconocido, cambios concurrentes, respuesta inválida y operación local offline. La consulta online falla explícitamente si no hay conexión; Play y gestión local siguen funcionando.

---

## 4. Búsqueda y selección de versiones compatibles (C02–C03)

### 4.1. Búsqueda en el catálogo (`GET /v2/search`)
- La búsqueda en la TUI utiliza facetas estrictas filtradas por la configuración inmutable de la instancia activa:
  - `project_type:mod`
  - `categories:<loader>` (ej: `categories:fabric`)
  - `versions:<mc_version>` (ej: `versions:1.21.1`)
- Parámetros de consulta:
```http
GET /v2/search?query=<texto>&facets=[["project_type:mod"],["categories:fabric"],["versions:1.21.1"]]&index=relevance&limit=20&offset=0
```
- **Debounce interactivo:** La TUI espera **350 ms** tras la última pulsación de tecla antes de despachar el comando HTTP de búsqueda.

### 4.2. Selección de versión compatible (Regla de C03)
**Nunca confiar en el campo `latest_version` del resultado de búsqueda.** Ese campo representa el último release global del proyecto (que puede ser para otra versión de Minecraft o para otro loader).
- Se consultan las versiones del proyecto mediante:
```http
GET /v2/project/{id|slug}/version?loaders=["fabric"]&game_versions=["1.21.1"]
```
- De las versiones devueltas:
  1. Se filtra por el canal activo (por defecto `release`; si no hay releases, se permite inspeccionar `beta` o `alpha`).
  2. Se toma la versión más reciente que declare soporte explícito para la versión de Minecraft y loader de la instancia.
  3. Se extrae el archivo primario (`primary: true`) del array `files`.

---

## 5. Grafo de dependencias y plan de instalación (C04–C06, C12)

Cada versión de Modrinth incluye una lista de `dependencies`:
```json
{
  "version_id": "8ss8jhox",
  "project_id": "P7dR8mSH",
  "file_name": null,
  "dependency_type": "required"
}
```

### 5.1. Taxonomía de dependencias y reglas de resolución

| Tipo | Comportamiento en mcTUI |
| --- | --- |
| **`required`** | **Obligatoria.** El resolutor busca recursivamente una versión compatible con la instancia. Si no existe versión compatible, **se detiene la instalación completa** y se explica el motivo. Nunca instalar un conjunto incompleto. |
| **`optional`** | **Opcional recomendada.** El usuario decide si instalarla. Al seleccionarla se incluyen también sus dependencias `required` transitivas, aunque estén representadas en `OptionalDeps`; el plan, la selección efectiva y el total de bytes deben coincidir. Sin seleccionarla no se instala su rama salvo que otro mod la requiera. |
| **`incompatible`** | **Conflicto.** Si el proyecto incompatible ya está instalado en la instancia o está presente en el lote de instalación, **se bloquea la operación** y se alerta del choque al usuario. |
| **`embedded`** | **Embebida.** La dependencia ya viene compilada dentro del archivo `.jar` principal (común en Fabric API). **Se omite de la descarga** y no se genera archivo suelto. |

### 5.2. Algoritmo del resolutor (`resolver.go`)
1. **Inicialización:** Cola de dependencias a resolver y conjunto de proyectos visitados `visited = map[string]bool{}` para evitar ciclos infinitos.
2. **Chequeo de estado local:** Para cada dependencia `required`:
   - Si el mod ya está instalado y activo en `mods/`: se verifica si la versión instalada cumple los requisitos. Si ya está, se marca como `AlreadyInstalled` y no se descarga de nuevo.
   - Si no está instalado: se consulta a la API de Modrinth la versión compatible para el par (Minecraft, Loader).
3. **Detección de incompatibilidades:** Se cruza la lista de dependencias `incompatible` contra todos los mods presentes en `mods.json` y los planificados para instalar.
4. **Construcción del `InstallPlan`:**
   ```go
   type InstallPlan struct {
       TargetMod        *ModrinthVersion
       RequiredDeps     []*ModrinthVersion
       OptionalDeps     []*ModrinthVersion
       IncompatibleMods []string
       EmbeddedMods     []string
       AlreadyInstalled []string
       TotalBytes       int64
   }
   ```
5. **Revisión en la TUI (C06):** Antes de descargar, se muestra la pantalla de confirmación. Si existen incompatibilidades o dependencias obligatorias irresolubles, el botón de confirmar queda inhabilitado con el mensaje de error explícito. `InstallPlan.SelectedOptional` ya calcula el cierre transitivo de los opcionales elegidos y la TUI muestra la selección y bytes efectivos; el test `TestSelectedOptionalIncludesRequiredDescendants` cubre una cadena de tres niveles. Falta una prueba integrada que verifique la publicación del conjunto completo desde la selección en TUI.

### 5.3. Protección de dependencias compartidas (`required_by`) (C12)
Cuando un mod se instala como dependencia de otro, se registra en su metadata:
```json
"required_by": ["P7dR8mSH"]
```
Si el usuario luego borra el mod dependiente o el principal:
- El sistema evalúa si otros mods activos todavía listan este ID en `required_by`.
- Si otros mods aún lo requieren, **no se elimina automáticamente** ni se ofrece eliminarlo en bloque.

---

## 6. Descarga segura, staging y verificación atómica (C07, DEC-10)

La descarga de mods sigue el mismo rigor de seguridad implementado en 5A para las bibliotecas del juego:

### 6.1. Flujo de descarga paso a paso
1. **Adquisición del Lock:** Se adquiere el bloqueo de operaciones de la instancia (`operationlock.Acquire`).
2. **Directorio de staging:** La descarga se realiza en un archivo temporal dentro de la misma partición/directorio:
   `instances/<id>/minecraft/mods/.tmp-*.jar` (nombre generado por `os.CreateTemp`, no derivado de datos externos).
3. **Streaming y Hashing simultáneo:**
   - Durante la lectura del body HTTP (`io.TeeReader`), se alimentan los digestores `crypto/sha512` y `crypto/sha1`.
   - Se controla el límite estricto de 250 MiB con `io.LimitReader`.
   - Se notifica el progreso en bytes a la TUI para actualizar la barra de descarga.
4. **Validación de integridad:**
   - Se compara el hash SHA-512 calculado contra el hash publicado en `files[].hashes.sha512`.
   - Si no coincide (o si la respuesta HTTP devolvió un código distinto de 200 o tipo HTML), el archivo temporal se **elimina de inmediato** y se reporta el error.
5. **Publicación recuperable (no atómica como conjunto):**
   - Una vez verificados todos los archivos, se guarda `instances/<id>/backups/mods_replaced/install.pending.json` con el registro e historial anteriores y los pasos de publicación. Se mueven los JARs antiguos al respaldo y se renombran los temporales uno por uno; `mods.json` e `history.json` se escriben mediante temporal y `os.Rename`.
   - Solo después de persistir ambos registros se retira el journal; la depuración de respaldos antiguos se realiza después. En error o al siguiente `Scan`/`History`/operación bajo lock, `recoverInstall` restaura los JARs y registros previos en orden inverso. Si el archivo de destino o el respaldo ya no coincide con el hash esperado, **no** se sobrescribe: se informa el bloqueo y se conserva el journal.
   - Las pruebas simulan interrupción tras guardar metadata, reemplazo con mismo/distinto nombre y cambios externos; no prueban cortes de energía ni garantizan durabilidad de renombres de directorio frente a pérdida de energía. Falta validar una interrupción real del proceso a mitad de un lote y la recuperación en los OS objetivo.

---

## 7. Política de actualizaciones, canales y rollback (C08–C11, DEC-06)

### 7.1. Chequeo masivo en una sola petición (`POST /v2/version_files/update`)
En lugar de iterar haciendo una petición HTTP por cada mod, mcTUI adopta el patrón batch de Prism:
```json
POST https://api.modrinth.com/v2/version_files/update
{
  "hashes": ["<hash_mod1>", "<hash_mod2>", "..."],
  "algorithm": "sha512",
  "loaders": ["fabric"],
  "game_versions": ["1.21.1"]
}
```
Modrinth devuelve en una sola respuesta el mapeo de cada hash con su última versión compatible.

### 7.2. Política de canales (`DEC-05` / C08)
- **Canal por defecto:** `release`. Solo se consideran actualizaciones etiquetadas como `release`.
- **Canal mutable por instancia:** El usuario puede alternar la política en la TUI para incluir `beta` o `alpha` si desea probar snapshots.

### 7.3. Política de Backup y Rollback (`DEC-06`)
Cuando una actualización reemplaza un archivo `.jar` existente:
1. **Directorio de reemplazos (instancia managed):** El `.jar` anterior se mueve a:
   `instances/<id>/backups/mods_replaced/<timestamp>_<nombre_archivo_anterior>.jar`.
   Para legacy, `Store.backupDir()` deriva el respaldo de `filepath.Dir(Registry)`, es decir `instances/legacy/backups/mods_replaced` bajo la raíz de datos de mcTUI, no junto a `.minecraft`. Una prueba comprueba esa ruta y que `History` lee allí; la interrupción real aún no se ha validado.
2. **Registro de rollback:** Se persiste en `instances/<id>/backups/mods_replaced/history.json`:
   - Fecha y hora del reemplazo.
   - Nombre del mod y `project_id`.
   - Hash del archivo anterior y del nuevo.
   - Ruta relativa del archivo respaldado.
3. **Cuota de retención:** Se retiene como máximo **1 versión anterior por mod** (o un tope de 500 MiB por instancia). Los respaldos más antiguos se depuran automáticamente para no agotar el espacio en disco.
4. **Rollback manual tras crash del juego:** La TUI permite `b` sobre un mod registrado para restaurar su JAR anterior y actualizar `mods.json`, después de verificar hashes. **Distinto** de recuperar automáticamente una actualización interrumpida mediante `install.pending.json`; el rollback manual no se dispara al detectar un crash.

### 7.4. Detección de modificaciones locales (C10)
Si un mod fue instalado por Modrinth pero su hash actual no coincide con el registrado en `mods.json` (indicando edición manual o corrupción), la TUI solicita **confirmación explícita diferenciada** antes de sobreescribir:  
*"El archivo local ha sido modificado manualmente. ¿Sobrescribir con la versión oficial de Modrinth? [y/N]"*.

---

## 8. Evolución del esquema `mods.json`

El registro local `instances/<id>/mods.json` usa `schema_version: 2` y acepta registros de 5B (`schema_version: 1`) hasta el siguiente guardado. Forma persistida actual (los hashes aquí son ilustrativos, no valores válidos):

```json
{
  "schema_version": 2,
  "mods": [{
    "file": "fabric-api.jar",
    "sha256": "<64 caracteres hexadecimales>",
    "enabled": true,
    "origin": "modrinth",
    "mtime_ns": 1727364600000000000,
    "project_id": "P7dR8mSH",
    "version_id": "kq5rM3y7",
    "channel": "release",
    "hashes": {"sha256": "<sha256>", "sha512": "<sha512>", "sha1": "<sha1>"},
    "required_by": ["<project_id_del_mod_que_lo_requiere>"],
    "installed_at": "2026-09-26T15:30:00Z"
  }]
}
```

La metadata `mod_id`, nombre y versión visible se extrae del JAR al escanear (`fabric.mod.json`), **no** se persiste como en el ejemplo antiguo. `origin` y `file` son las claves actuales, no `source`/`file_name`; no existe `updated_at` a nivel de registro. No migrar ni sobrescribir archivos externos por una simple coincidencia de nombre.

---

## 9. Diseño de pantallas y navegación en la TUI

### 9.1. Puntos de entrada desde el gestor de mods (5B)
En la lista de mods actual, se añaden los atajos:
- `s` o `/`: Abrir catálogo y buscador de Modrinth.
- `u`: Chequear actualizaciones para todos los mods de la instancia.

### 9.2. Flujo de navegación

```text
[Lista de Mods (5B)]
      │
      ├── (s / /) ──► [Buscador Modrinth (Debounce 350ms)]
      │                     │
      │                     ├── (Enter) ──► [Detalle del Mod]
      │                     │                     │
      │                     │                     └── (Enter) ──► [Plan de Instalación]
      │                     │                                           │
      │                     │                                           └── (y) ──► [Progreso Descarga]
      │                     │                                                            │
      │                     │                                                            └── (Listo) ──► Vuelve a [Lista de Mods]
      │
      └── (u) ────────► [Chequeo Batch de Updates]
                            │
                            └── (Updates hallados) ──► [Revisión de Actualizaciones]
                                                             │
                                                             └── (y) ──► [Progreso Descarga + Backup]
                                                                              │
                                                                              └── (Listo) ──► Vuelve a [Lista de Mods]
```

### 9.3. Wireframes en texto de la TUI

#### Pantalla 1: Buscador de Modrinth
```text
================================================================================
 Catálogo Modrinth — Instancia: "Super Fabric" (Fabric 0.16.5 | MC 1.21.1)
================================================================================
 Buscar: sodi█
 Filtros: [Loader: Fabric] [MC: 1.21.1] [Canal: Release]
--------------------------------------------------------------------------------
   Nombre          Autor       Descargas   Descripción corta
 > Sodium          jellysquid3   42.1M     Modern rendering engine for Minecraft
   Sodium Extra    FlashyReese    8.4M     Features that shouldn't be in Sodium
   Iris Shaders    coderbot      29.8M     A modern shaders mod for Minecraft
--------------------------------------------------------------------------------
 [Enter] Ver detalles  [Tab] Cambiar orden  [Esc] Volver al gestor de mods
================================================================================
```

#### Pantalla 2: Revisión del Plan de Instalación (`ReviewPlan`)
```text
================================================================================
 Plan de instalación — Modrinth
================================================================================
 Se va a instalar: Sodium (0.5.11+mc1.21.1)
 Archivo: sodium-fabric-0.5.11+mc1.21.1.jar (1.8 MiB)

 Dependencias obligatorias (se instalarán automáticamente):
   [✓] Fabric API (0.102.0+1.21.1) — 2.4 MiB

 Dependencias opcionales disponibles:
   [ ] Sodium Extra (0.5.4+mc1.21.1) — 320 KiB  (Espacio para marcar/desmarcar)

 Incompatibilidades detectadas:
   Ninguna.

 Total a descargar: 4.2 MiB (2 archivos)
 Integridad: Verificación obligatoria SHA-512 previa a la activación.
--------------------------------------------------------------------------------
 ¿Confirmar instalación y descarga? [y / n]
================================================================================
```

#### Pantalla 3: Revisión de Actualizaciones (`ReviewUpdates`)
```text
================================================================================
 Actualizaciones disponibles (2 mods)
================================================================================
 [x] Fabric API     0.100.8 ➔ 0.102.0+1.21.1  (2.4 MiB) [Release]
 [x] Lithium        0.12.0  ➔ 0.12.7+1.21.1   (890 KiB) [Release]

 Política de respaldo: Los archivos anteriores se moverán a:
   instances/test/backups/mods_replaced/
 para permitir rollback en caso de problemas.
--------------------------------------------------------------------------------
 [Espacio] Alternar selección  [y] Actualizar seleccionados  [Esc/n] Cancelar
================================================================================
```

---

## 10. Matriz de pruebas y validación

| Área | Escenario de prueba | Validación obligatoria |
| --- | --- | --- |
| **API Client** | Rate limiting (HTTP 429) | Lee cabeceras `X-Ratelimit-Reset` y aplica backoff/aviso; no satura la API. |
| **Seguridad de Red** | Redirección a dominio no autorizado | Aborta inmediatamente si una URL sale de `api.modrinth.com` o `cdn.modrinth.net`. |
| **Seguridad de Red** | Payload de descarga > 250 MiB | Aborta descarga y elimina el temporal si excede el tamaño máximo. |
| **Integridad** | Hash SHA-512 disuelto o corrupto | El archivo `.tmp` se descarta y nunca entra a `mods/`. La instalación se detiene. |
| **Resolutor** | Dependencia `required` no disponible | Se aborta la operación completa; no se instala el mod padre sin su dependencia. |
| **Resolutor** | Dependencia `embedded` | Se reconoce pero no se agrega a la lista de descargas. |
| **Resolutor** | Conflicto `incompatible` | Se detecta el conflicto contra mod instalado y se bloquea la confirmación. |
| **Resolutor** | Dependencia circular (A ➔ B ➔ A) | El detector de visitados corta el ciclo sin caer en bucle infinito. |
| **Opcionales** | A opcional B, B requiere C y C requiere D | Elegir B incluye B/C/D; omitir B no instala esa rama; bytes y confirmación coinciden con lo publicado. |
| **Identificación 5B** | JAR externo reconocido/no reconocido, cambio durante hash y API offline | Identifica por SHA-512 sin atribución falsa; conserva archivo y gestión offline si falla la API. |
| **Actualizaciones** | Chequeo batch `version_files/update` | Envía hashes en lote y mapea correctamente versiones nuevas; confirma canal y compatibilidad del conjunto. |
| **Interrupción** | Cierre entre backup, renombre y guardado de cada registro | Al reabrir se recuperan JARs y metadata anteriores o se informa bloqueo con rutas; archivo externo cambiado no se borra. |
| **TUI** | Búsqueda lenta/obsoleta, descarga cancelada y revisión de actualizaciones | `Esc` cancela petición activa; no aplica respuestas obsoletas; muestra bytes reales, versiones anterior→nueva y respaldo antes de confirmar. |
| **Rollback** | Actualización de mod existente | El JAR viejo se preserva en `backups/mods_replaced/` y se puede recuperar. |
| **Concurrencia** | Intento de instalación con juego activo | Bloqueo por `operationlock.Acquire`; rechaza mutación con mensaje de instancia ocupada. |
| **Multiplataforma** | Pruebas Linux y ejecución real en Windows/macOS | Pasan `go test ./...`, `go test -race ./...`, `go vet ./...` en entorno de pruebas y compilación cruzada; esta última **no** sustituye ejecutar en Windows/macOS. |

---

## 11. Estado actual, brechas y criterios de cierre

**Corte observado:** árbol local sin commit de 5C ni validación manual de Modrinth. En este corte `go test ./...`, `go test -race ./internal/mods ./internal/modrinth ./internal/ui`, `go vet ./...` y `git diff --check` pasan; los siete fallos previos de UI ya no se reproducen en este árbol. Por pedido del usuario, no se ejecutaron pruebas ni compilaciones Windows/macOS en este corte; las compilaciones cruzadas registradas corresponden al corte anterior. Los tests de red usan `httptest`, no demuestran funcionamiento de la API pública ni ejecución en esos OS. En el plan general, C01–C12 siguen sin marcar: **no confundir código existente con tarea cerrada**. La tabla concreta la evidencia y el trabajo aún requerido; las rutas describen el código presente, no una promesa de cierre.

| Tarea | Estado del árbol local | Falta para cierre |
| --- | --- | --- |
| **C01** Cliente API | `internal/modrinth/client.go`: User-Agent, hosts HTTPS, timeouts, límites JSON/JAR, cabeceras 429 y reintentos batch; tests simulados. La TUI comparte un cliente para consultas/descargas y cancela consultas obsoletas. Sin caché persistente: los resultados de búsqueda, compatibilidad y updates pueden quedar obsoletos entre revisión y publicación; se priorizan cancelación y rate limit compartido. | Validar límites ante tráfico real; reevaluar caché solo si la latencia o el volumen medidos lo justifican. |
| **C02** Búsqueda y detalles | `Search`, paginación y debounce 350 ms; detalle básico en `internal/ui/modrinth.go`. Las consultas de búsqueda, detalle, plan y updates se cancelan al abandonar o reemplazar la operación y sus respuestas tardías se descartan por identificador; prueba focalizada de cancelación/obsolescencia. | Ficha/ordenación y renderizado útil en terminal pequeña según §9; validar navegación manual y latencia real. |
| **C03** Versión compatible | Consulta filtrada de versiones, `Compatible` y archivo primario; no se usa `latest_version` del buscador. | Probar selección y filtrado de canal con respuestas reales o fixtures representativos de Modrinth; no anunciar compatibilidad de ejecución garantizada. |
| **C04–C05** Grafo y bloqueos | `ResolveBatch` maneja required/optional/incompatible/embedded, ciclos y conflictos; `SelectedOptional` incluye descendientes requeridos de opcionales elegidos. Una prueba integrada verifica resolución→descarga/publicación de una rama opcional B→C→D y sus referencias `required_by`. | Una prueba integrada publica dos updates y su dependencia compartida, comprueba `required_by` tras quitar una arista, y otro test impide atribuir a un opcional omitido una dependencia ya instalada. Faltan cambios externos no registrados y conflictos que solo conciernen a opcionales no elegidos. La versión observada por cada elemento seleccionado y los IDs incompatibles se revalidan bajo lock antes de staging (tests de versión cambiada en rama requerida/opcional y conflicto nuevo). Los conflictos declarados por ramas opcionales ya no contaminan el plan obligatorio: se guardan por elemento y se comprueban solo si se elige esa rama, incluso frente a mods instalados o a otros elementos elegidos (tests de resolutor, instalación y TUI). |
| **C06** Revisión previa | Pantalla de plan con opcionales, selección efectiva y tamaño calculado. | Mostrar mod principal, dependencias ya instaladas, reemplazos, destino/instancia, tamaños y conflictos; diferenciar claramente un opcional seleccionado de un descendiente requerido automáticamente y revisar cambios de estado al confirmar. La TUI ahora marca descendientes requeridos por un opcional seleccionado, muestra/bloquea conflictos de la selección y reinicia el cursor al entrar en revisión. |
| **C07** Descarga/aplicación | `Stage` verifica SHA-512, tamaño, host, staging y limpieza; `Store.Install` aplica bajo lock y conserva metadata. El callback de bytes acumulados llega a Bubble Tea y se muestra en la pantalla de trabajo; `Esc` cancela la descarga y espera la limpieza antes de salir. Tests cubren progreso/espera de la TUI, cancelación durante descarga, cancelación tras el primer archivo y fallo HTTP 503 en el segundo JAR sin publicación ni metadata parcial. | Falta validación manual del progreso real. |
| **C08** Updates y canales | Endpoint batch, filtro de versión/canal y canal por instancia visible y mutable; chequeo `u` identifica previamente mods 5B/externos por SHA-512. Si batch devuelve una versión de otro canal, se consultan las versiones filtradas de ese proyecto antes de descartar un posible release; test simulado comprueba fallback release, canal beta y versión ya instalada. | Revisar política y resultados de todos los mods elegibles con respuestas reales; no confundir los tests simulados con validación de la API pública. |
| **C09** Conjunto actualizado | `ResolveBatch` resuelve juntos los targets seleccionados. | Una prueba integrada cubre dos actualizaciones y una dependencia compartida, publicación y backups; se rechaza bajo lock un cambio de versión registrado entre revisión y aplicación y una edición externa durante la descarga. Faltan conflicto en lote completo y validación de ejecución real. |
| **C10** Cambios locales | `Install` revalida SHA-256/mtime y exige consentimiento; la TUI ofrece confirmación diferenciada. | La revisión muestra cada reemplazo y marca JARs modificados, incluidos dependientes opcionales elegidos; el test TUI exige consentimiento antes de iniciarlos. Se rechaza una edición externa hecha durante la descarga, aun con consentimiento anterior; el archivo original queda intacto y no se publica el update. Una edición previamente consentida se respalda con su hash real y se restaura mediante rollback (test). Falta validar esta ruta manualmente. |
| **C11** Recuperación | Backups, `history.json`, `Rollback` manual y `install.pending.json`; tests de interrupción simulada con mismo/distinto nombre y archivo externo modificado. Antes de restaurar se verifican **todos** los JAR, temporales, respaldos y destinos del lote: un backup corrupto o un archivo modificado en un paso posterior ya no deja pasos anteriores parcialmente recuperados. Un proceso hijo termina con `os.Exit` tras publicar el primer JAR de dos y `Scan` recupera el lote al reiniciar (Linux). Legacy guarda historial bajo `instances/legacy/backups/mods_replaced`. La poda persiste primero el historial retenido y después borra backups descartados; el fallo de eliminación puede dejar un backup huérfano pero no una referencia rota. | Probar otros puntos de interrupción del lote y la ventana historial→poda; no hay garantía probada ante cortes eléctricos. Por pedido del usuario se omite ejecución Windows/macOS. La validación previa no evita carreras de escritores externos *durante* la restauración. |
| **C12** Dependencias compartidas/embedded | `required_by` impide retirar una dependencia aún requerida; `embedded` no se descarga; tests puntuales. | Al actualizar dos mods, la prueba comprueba que una dependencia compartida conserva el dueño activo y pierde el dueño que dejó de requerirla. Falta verificar el borrado/actualización de más combinaciones y que nunca se retire una dependencia activa. |

**Orden de cierre recomendado:** (1) validación manual de revisión, progreso y cancelación de la TUI con API real; (2) ampliar interrupción/reanudación y rollback con pruebas de proceso en Linux; (3) actualizar evidencia y casillas C01–C12 del `PHASE_5_IMPLEMENTATION_PLAN.md`, registrar diferencias aceptadas y solo entonces declarar 5C cerrada. Windows/macOS quedan fuera de este corte por pedido del usuario. 5D y el núcleo 5A–5D siguen siendo cierres separados.

**Avance posterior de TUI y cierre interactivo (Linux):**
1. **Menú principal y barra superior:** Se incorporó `Mods` como opción directa (Play, Worlds, Mods, Change Instances, Configs, Quit) y en la barra superior con el atajo global `m` y navegación con Enter, evitando depender únicamente de submenús.
2. **Revisión y confirmación de Rollback (`modsRollbackScreen`):** El atajo `b` ya no restaura a ciegas; consulta el historial asíncronamente en `backups/mods_replaced/history.json`, despliega la pantalla de revisión con la versión actual, versión a restaurar, fecha del respaldo y ruta del JAR anterior. Exige confirmación explícita `[y/n]` y comprueba hashes antes de restaurar, protegiendo archivos modificados externamente.
3. **Buscador Modrinth (§9.3 Wireframe 1 / C02):**
   - Ordenación dinámica con `[Tab]` ciclando entre `relevance`, `downloads`, `follows`, `newest` y `updated` con parámetro API `index`.
   - Filtros activos visibles en cabecera: `[Loader: ...] [MC: ...] [Canal: ...] [Orden: ...]`.
   - Formateo de descargas legible (`formatDownloads`, ej. `42.1M`, `170.7M`).
   - Tabla responsiva con columnas alineadas (`Nombre`, `Autor`, `Descargas`, `Descripción corta`) para terminales estándar/anchas, con fallback compacto en terminales angostas.
   - Contador de resultados visibles y totales (`Mostrando X de Y mods`).
4. **Ficha y detalles de Mod enriquecida (§9.2 / C02):**
   - Título, slug, autor/equipo y contador de descargas formateado.
   - Descripción envuelta a los límites de la terminal.
   - Sección de versión compatible para la instancia activa: número de versión, badge de canal, archivo primario, tamaño en MiB/KiB, fecha de publicación, loaders soportados y versiones de MC compatibles.
   - Sección de dependencias declaradas desglosada por tipo: obligatorias (`✓`), opcionales (`•`) e incompatibles (`✗`).
   - Acción directa para pasar al plan de instalación con `[Enter]` o volver con `[Esc]`.
5. **Verificación en Linux:** Smoke test interactivo ejecutado contra el API real de Modrinth (`https://api.modrinth.com/v2`), demostrando búsqueda live, ordenación dinámica por descargas, ficha de detalles con Fabric API para Minecraft 1.21, revisión del plan de instalación con destino en `.minecraft/mods`, cancelación segura con `[n]` sin tocar disco, y salida limpia. Pruebas automatizadas `go test ./...` y `go test -race ./internal/mods ./internal/modrinth ./internal/ui` pasando al 100%.
---
*Fin de la especificación técnica 5C.*
