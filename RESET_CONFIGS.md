# Resets Configs

## Propuesta funcional

`Resets Configs` es una funcionalidad de mcTUI destinada a recuperar una instalación de Minecraft que dejó de iniciar correctamente por conflictos entre mods, loaders, configuraciones o archivos generados por distintas versiones.

El objetivo es ofrecer una limpieza segura desde la TUI, sin necesidad de abrir manualmente el explorador de archivos ni conocer la estructura interna de `.minecraft`.

La operación debe ser conservadora: no debe borrar mundos guardados, versiones descargadas, librerías, assets ni shaders.

> **Requisito obligatorio:** mcTUI y la funcionalidad `Resets Configs` operan exclusivamente sobre una instalación legítima de Minecraft: Java Edition comprada y descargada previamente en el equipo a través del launcher oficial. No funcionan ni tienen soporte para copias no autorizadas o carpetas sin una instalación oficial preexistente.

---

## Problema que resuelve

Minecraft Java utiliza una única carpeta de datos para varias versiones y configuraciones. Cuando se alterna entre versiones, Fabric, mods y paquetes de recursos, pueden quedar archivos incompatibles que provocan:

- Crashes durante el inicio.
- Errores de mixins.
- Mods duplicados o incompatibles.
- Configuraciones antiguas que interfieren con una versión nueva.
- Pantallas de carga que nunca terminan.
- Errores difíciles de interpretar para un usuario que no conoce la estructura de `.minecraft`.

Actualmente, la solución habitual consiste en limpiar manualmente partes de `.minecraft`. Esto tiene dos problemas:

1. Es fácil borrar archivos importantes por accidente.
2. Es difícil saber qué archivos pertenecen a los mods y cuáles son necesarios para conservar la instalación.

`Resets Configs` encapsula ese procedimiento dentro de mcTUI.

---

## Objetivo principal

Permitir que el usuario vuelva rápidamente a una configuración limpia de mods, manteniendo intactos sus datos importantes.

La operación debe:

- Ser accesible desde el menú principal de mcTUI.
- Mostrar claramente qué elementos serán afectados.
- Requerir una confirmación explícita.
- Ofrecer la posibilidad de crear un respaldo antes de modificar archivos.
- Permitir ejecutar un reset sin conservar el respaldo cuando el usuario lo prefiera.
- Informar qué se hizo y qué no se pudo modificar.

---

## Alcance de la primera versión

La primera versión de `Resets Configs` debe trabajar sobre la carpeta de Minecraft detectada por mcTUI:

```text
~/.minecraft/
```

En Windows, que es el sistema operativo prioritario para esta funcionalidad, debe utilizar la ruta oficial:

```text
%APPDATA%\\.minecraft\\
```

En Linux y macOS se conserva el comportamiento actual de `config.MinecraftDir()`:

```text
Linux / macOS: ~/.minecraft/
```

La resolución de estas rutas debe centralizarse en `internal/config`. `Resets Configs` no debe construir rutas suponiendo que el sistema utiliza `/`, `~` o una carpeta concreta del usuario.

### Estado actual de mcTUI

`internal/config.MinecraftDir()` utiliza `%APPDATA%\.minecraft` en Windows y `~/.minecraft` en Linux y macOS.

Para la primera versión se mantiene deliberadamente la ruta unificada `~/.minecraft` en macOS para no romper ni desacoplar instalaciones, mundos y perfiles existentes creados por mcTUI. Cualquier cambio a la ruta oficial de macOS (`~/Library/Application Support/minecraft`) debe ser, en todo caso, una migración global independiente con detección de instalaciones previas.

Sin embargo, `MinecraftDir()` actualmente ignora el error de `os.UserHomeDir()`. El motor de reset debe verificar que la ruta devuelta sea absoluta y válida antes de cualquier operación destructiva o de movimiento, abortando inmediatamente si no se puede resolver la carpeta de usuario de manera segura.

---

## Compatibilidad multiplataforma

`Resets Configs` debe considerarse una funcionalidad multiplataforma desde la primera versión, con Windows como plataforma de validación principal.

El motor de reset debe utilizar APIs portables de Go, principalmente:

- `os.UserHomeDir()`.
- `os.Getenv("APPDATA")` cuando corresponda en Windows.
- `filepath.Join()` para construir rutas.
- `os.ReadDir()` para inspeccionar directorios.
- `os.Rename()` para mover carpetas dentro del mismo volumen.
- `os.Stat()` y `os.Lstat()` para verificar archivos, directorios y enlaces.

No se debe depender de comandos específicos como `mv`, `rm`, `rmdir`, `cmd`, PowerShell o utilidades de Unix para realizar la operación principal.

### Windows: plataforma prioritaria

En Windows se deben considerar especialmente los siguientes casos:

- La carpeta de Minecraft normalmente está en `%APPDATA%\\.minecraft`.
- Los separadores de ruta son `\\`, aunque `filepath.Join()` debe abstraerlos.
- Los archivos pueden estar bloqueados por el proceso de Minecraft o por otro programa.
- Windows puede impedir mover archivos que todavía están abiertos.
- Las rutas pueden contener espacios y nombres de usuario con caracteres no ASCII.
- El usuario puede no tener permisos para modificar una instalación ubicada fuera de su perfil.
- Algunas instalaciones pueden incluir junctions o enlaces simbólicos.
- Un antivirus puede inspeccionar o bloquear temporalmente archivos `.jar`.

La TUI debe mostrar errores comprensibles, por ejemplo:

```text
Could not reset mods/: the folder is being used by another process.
Close Minecraft and try again.
```

No se debe solicitar automáticamente elevación de privilegios ni ejecutar mcTUI como administrador. La operación debe funcionar con los permisos normales del usuario siempre que `.minecraft` esté dentro de su perfil.

### Linux

La ruta por defecto es:

```text
~/.minecraft/
```

La operación debe respetar permisos y propietarios existentes tanto como sea posible. Si una carpeta no puede moverse por permisos insuficientes, debe informarse sin modificar las carpetas protegidas.

En distribuciones donde Minecraft se ejecuta mediante un launcher empaquetado, Flatpak, Snap o una instalación personalizada, la ubicación real puede no ser la ruta estándar. La primera versión debe utilizar la ruta ya resuelta por mcTUI y no intentar descubrir instalaciones alternativas automáticamente.

### macOS

mcTUI utiliza la ruta estándar Unix en el perfil del usuario:

```text
~/.minecraft/
```

La implementación debe utilizar `filepath.Join()`. Se mantiene `~/.minecraft` para coincidir con las demás partes del launcher y no perder acceso a perfiles previos.

También debe contemplarse que macOS puede bloquear archivos mientras Minecraft está abierto, de forma análoga a Windows.

### Matriz mínima de comportamiento

| Sistema | Ruta esperada | Prioridad | Riesgo principal |
| --- | --- | ---: | --- |
| Windows | `%APPDATA%\\.minecraft` | Alta | Archivos bloqueados y permisos |
| Linux | `~/.minecraft` | Media | Permisos y launchers alternativos |
| macOS | `~/.minecraft` | Media | Permisos y archivos bloqueados |

El resultado funcional debe ser el mismo en los tres sistemas: mover `mods/` y `config/` a un backup, conservar los datos protegidos y reportar errores de manera clara.

### Elementos que se pueden resetear

#### `mods/`

Contiene los archivos `.jar` y otros archivos utilizados por los mods.

Es el origen más frecuente de conflictos cuando se cambia de versión o de modpack.

La operación debe afectar la carpeta completa, en lugar de eliminar solamente algunos archivos. Según la opción elegida por el usuario, esa carpeta se puede mover a un respaldo o eliminar de forma permanente.

#### `config/`

Contiene configuraciones generadas por Fabric, mods y otros componentes del entorno modificado.

Una configuración perteneciente a una versión anterior puede causar errores aunque el mod correspondiente ya no esté instalado.

#### Archivos de diagnóstico (fuera de la primera versión)

`logs/` y `crash-reports/` no suelen ser la causa del crash. La primera versión no los modifica; más adelante se puede ofrecer su limpieza como acción independiente para facilitar el diagnóstico de errores nuevos.

---

## Elementos que nunca deben tocarse por defecto

La siguiente lista define la protección mínima de `Resets Configs`:

```text
saves/
versions/
assets/
libraries/
shaderpacks/
```

### `saves/`

Contiene los mundos de un jugador. Es el dato más importante para proteger.

`Resets Configs` no debe borrar, mover, renombrar ni modificar ningún archivo dentro de esta carpeta.

> **Advertencia crítica sobre mundos:** Aunque el reset no toca físicamente `saves/`, si el usuario abre y guarda posteriormente un mundo que contenía bloques, entidades o ítems de mods ahora eliminados, el juego puede borrar de forma permanente esos datos del mapa. La TUI debe advertir explícitamente:
> *"Worlds are preserved, but do not load mod-dependent worlds until those mods are restored to prevent block/item loss."*

### `versions/`

Contiene las versiones descargadas y los archivos necesarios para iniciar Minecraft.

También puede contener el perfil cacheado de Fabric que genera mcTUI:

```text
versions/<version>/fabric-profile.json
```

Este perfil no debe eliminarse durante el reset normal.

### `assets/`

Contiene recursos descargados por Minecraft. No tiene sentido descargarlos nuevamente durante una limpieza de mods.

### `libraries/`

Contiene librerías de Minecraft, Fabric y otras dependencias. Borrarlas haría que el próximo inicio requiera descargas innecesarias.

### `shaderpacks/`

Los shaders no son el objetivo de esta funcionalidad y deben conservarse expresamente.

---

## Resource packs

Los paquetes de recursos deben quedar fuera del reset inicial, salvo que más adelante se decida incluir una opción específica.

Aunque algunos resource packs pueden provocar errores visuales o incompatibilidades, no forman parte estrictamente de la configuración de mods. Mezclarlos en el mismo reset aumentaría el alcance de la operación y podría sorprender al usuario.

En una versión futura se podría ofrecer una opción independiente:

```text
Reset mods and configs
Reset resource packs
Reset everything except worlds and installations
```

---

## Estrategia de respaldo

El backup debe ser opcional. Es muy probable que el usuario quiera una limpieza rápida y no necesite conservar mods o configuraciones viejas, por lo que mcTUI no debe imponer un respaldo que ocupe espacio y requiera una limpieza posterior.

La funcionalidad debe ofrecer dos modos:

### Reset rápido, sin backup

Elimina de forma permanente el contenido seleccionado de:

```text
.minecraft/mods/
.minecraft/config/
```

Este modo debe estar acompañado por una advertencia explícita, porque no permite restaurar la configuración anterior desde mcTUI.

Ejemplo de confirmación:

```text
This will permanently delete:
  - mods/
  - config/

Worlds, versions, libraries, assets and shaders will be preserved.

[y] Delete and reset  [n] Cancel
```

### Reset con backup

Mueve las carpetas seleccionadas a una carpeta de respaldo, por ejemplo:

```text
.minecraft/mctui-backups/reset-2026-09-24-153000/
├── mods/
└── config/
```

El nombre debe incluir una marca temporal para evitar sobrescribir respaldos anteriores.

Este modo conserva los archivos para inspeccionarlos y restaurarlos manualmente. Evita la pérdida accidental de configuraciones personalizadas y facilita el diagnóstico cuando el problema continúa. La restauración desde mcTUI queda fuera de la primera versión: la UI no debe presentar el respaldo como una función de deshacer.

El destino debe ser nuevo y exclusivo de cada operación. Si el nombre generado ya existe, se debe elegir otro o cancelar; nunca sobrescribir ni mezclar archivos con un respaldo anterior.

### Selección del modo

La UI debe preguntar por el modo antes de la confirmación final:

```text
How should Resets Configs handle the old files?

  Reset without backup
  Reset and create backup
```

Después debe mostrar una confirmación adaptada al modo elegido. No se debe presentar un reset destructivo como si fuera equivalente a mover archivos a un backup.

La preferencia no necesita persistirse en la primera versión. La decisión debe tomarse en cada ejecución para evitar que una elección antigua provoque una eliminación inesperada.

### Eliminación posterior de backups

La eliminación automática de respaldos no debe formar parte de la primera versión. Si se agrega más adelante, debe ser una acción explícita desde un administrador de respaldos.

---

## Flujo de usuario propuesto

### 1. Entrada desde el menú

Agregar una opción al menú principal:

```text
Resets Configs
```

El nombre se mantiene en inglés para coincidir con la identidad de la aplicación y con el nombre solicitado para la funcionalidad.

### 2. Pantalla informativa

La TUI debe mostrar una explicación breve:

```text
This will reset mod-related files from your Minecraft directory.

Choose how to handle the old files:
  - Delete them permanently
  - Move them to a backup

The following folders will be affected:
  - mods/
  - config/

Your worlds, versions, libraries, assets and shaders will be preserved.
```

También debería mostrar la ruta concreta que se utilizará.

### 3. Detección previa

Antes de pedir confirmación, mcTUI debe verificar:

- Que la carpeta `.minecraft` exista.
- Que `mods/` o `config/` existan.
- Que las rutas que se van a operar no sean enlaces simbólicos ni junctions inesperadas.
- Advertir al usuario que cierre Minecraft. Si se incorpora detección de procesos, bloquear el reset cuando se detecte el juego; un resultado negativo no garantiza que esté cerrado.
- Que las rutas sean accesibles para el modo elegido. Los permisos y bloqueos también pueden cambiar durante la ejecución, así que se deben manejar los errores reales de cada operación.

El backup de la primera versión se crea dentro de `.minecraft` mediante movimiento, sin copia entre volúmenes ni comprobación previa de espacio para una copia.

Si no hay nada para resetear, debe informarse sin crear un respaldo vacío.

### 4. Confirmación

La operación debe requerir una confirmación explícita, por ejemplo:

```text
Reset these folders using the selected mode?

[y] Continue  [n] Cancel
```

La tecla `Esc` también debe cancelar.

### 5. Ejecución

El proceso debe:

1. Validar nuevamente las rutas objetivo.
2. Si se eligió backup, crear el directorio de respaldo y mover `mods/` y `config/`.
3. Si se eligió reset rápido, eliminar únicamente las rutas objetivo permitidas.
4. Verificar qué rutas quedaron en origen y destino.
5. Mostrar un resumen, incluso si una operación falló parcialmente.

### 6. Resultado

Ejemplo de resultado exitoso:

```text
Resets Configs completed successfully.

Mode:
  Reset with backup

Moved to backup:
  - mods/
  - config/

Preserved:
  - worlds
  - versions
  - libraries
  - assets
  - shaders

Backup:
  ~/.minecraft/mctui-backups/reset-2026-09-24-153000/
```

Ejemplo de resultado sin backup:

```text
Resets Configs completed successfully.

Mode:
  Reset without backup

Permanently deleted:
  - mods/
  - config/

Preserved:
  - worlds
  - versions
  - libraries
  - assets
  - shaders
```

Si una parte falla, la TUI debe indicar exactamente cuál fue y no presentar la operación como completamente exitosa.

---

## Reglas de seguridad

### No modificar mundos

La implementación debe trabajar únicamente con rutas construidas desde la carpeta de Minecraft y con nombres de directorio permitidos explícitamente.

No debe utilizar una limpieza recursiva sobre toda la carpeta `.minecraft`.

### No seguir enlaces simbólicos ni junctions de forma peligrosa

Antes de operar, comprobar con `os.Lstat()` las rutas de `.minecraft`, `mods/`, `config/`, el directorio base de respaldos (`mctui-backups/`) y el subdirectorio de destino.

Si alguna de estas rutas intermedias o finales es un enlace simbólico o una junction inesperada, se debe rechazar la operación y explicarlo. Esto evita que un enlace en `mctui-backups` desvíe archivos hacia `saves/` o fuera de la instalación.

En la primera versión se mueven o eliminan las carpetas objetivo completas; no se recorren sus archivos internos para decidir qué resetear. Volver a validar inmediatamente antes de la operación reduce el riesgo de que las rutas hayan cambiado desde la pantalla informativa.

### Minecraft cerrado

La primera versión debe advertir que Minecraft debe estar cerrado antes de confirmar. Si se incorpora detección de procesos y se encuentra el juego activo, debe detenerse la operación; no detectar un proceso no garantiza que esté cerrado. Los bloqueos de archivos detectados durante el movimiento o borrado deben informarse como errores, sin anunciar éxito completo.

Modificar `mods/` o `config/` mientras el juego está escribiendo archivos podría generar un respaldo inconsistente.

### Operación reversible o destructiva

Cuando se elige backup, mover carpetas dentro de `.minecraft` con `os.Rename()` evita copiar y borrar. Si el movimiento falla, mcTUI debe informar el error y conservar el origen; no debe intentar copiar entre volúmenes en la primera versión.

Cuando se elige el reset sin backup, la eliminación permanente debe ejecutarse únicamente sobre las rutas objetivo permitidas y después de una confirmación específica para ese modo. No debe reutilizarse una confirmación genérica de movimiento.

### Fallos parciales

La implementación debe contemplar que `mods/` exista pero `config/` no, o viceversa.

Además, debe contemplarse el **borrado o movimiento parcial dentro de una misma carpeta**: si `os.RemoveAll("mods")` o `os.Rename` elimina o traslada varios archivos internos y luego falla debido a un archivo bloqueado, la carpeta quedará en estado corrupto o parcialmente eliminada.

Si una carpeta se mueve o elimina correctamente y la otra falla, o si una carpeta queda a medio borrar:
- No se debe anunciar éxito completo ni asumir que la carpeta quedó intacta solo porque sigue existiendo.
- No se debe intentar revertir automáticamente lo ya modificado (un borrado parcial no se puede recuperar).
- Se deben detener las operaciones restantes.
- Se debe clasificar el estado de cada carpeta (`deleted`, `partially_deleted`, `moved`, `failed`, `untouched`) e informar qué archivos se alcanzaron a procesar y los errores correspondientes.

---

## Encaje técnico en mcTUI

La funcionalidad puede integrarse sin modificar el sistema de lanzamiento principal.

### `internal/config`

Ya contiene `MinecraftDir()`, que debe ser la única fuente utilizada para resolver la ruta de `.minecraft`.

No se debe duplicar la lógica de rutas dentro de la UI o del nuevo paquete.

### Nuevo paquete sugerido

Crear un paquete independiente:

```text
internal/resetconfigs/
```

Responsabilidades posibles:

- Definir las carpetas resetables.
- Definir las carpetas protegidas conceptualmente.
- Detectar qué elementos existen.
- Crear el nombre del respaldo.
- Mover los elementos al respaldo cuando el usuario elija ese modo.
- Eliminar los elementos de forma permanente cuando el usuario elija el reset rápido.
- Devolver un resultado detallado y errores parciales.

La UI no debería ocuparse directamente de `os.Rename`, `os.ReadDir` ni de la estructura física del respaldo.

### `internal/ui`

La UI necesitaría:

- Nuevos estados de pantalla propios para `Resets Configs` (por ejemplo `resetMenuScreen`, `resetConfirmScreen`, `resetResultScreen`).
- Una pantalla de información y selección de modo (backup vs rápido).
- Una pantalla de confirmación diferenciada para el modo destructivo.
- Una pantalla de progreso y resultado detallado.
- Mensajes de éxito y error con estado granular por carpeta.
- Una nueva opción en el menú principal (`Resets Configs`), actualizando `menuOptionsCount` y el cursor.

**Importante:** No reutilizar los mensajes ni el flujo de descarga de `downloadScreen` (`DownloadDoneMsg` en `update.go` activa `m.play = true` y cierra el TUI con `tea.Quit` para lanzar el juego). El reset debe usar su propio comando asíncrono (`tea.Cmd`) ejecutado en goroutine y su propio tipo de mensaje (`ResetDoneMsg`), evitando congelar la interfaz durante operaciones de E/S lentas o bloqueos.

### `main.go`

No debería ejecutar directamente la operación. La lógica debe vivir en su paquete propio y ser invocada por la UI, igual que la UI actualmente decide si el usuario seleccionó jugar.

---

## Modelo de resultado sugerido

El paquete debe devolver un resultado estructurado que permita reflejar estados parciales tanto en movimiento como en borrado:

```go
type FolderStatus string

const (
    StatusUntouched        FolderStatus = "untouched"
    StatusMoved            FolderStatus = "moved"
    StatusDeleted          FolderStatus = "deleted"
    StatusPartiallyDeleted FolderStatus = "partially_deleted"
    StatusFailed           FolderStatus = "failed"
)

type FolderResult struct {
    Name   string       // "mods", "config"
    Path   string       // ruta absoluta de origen
    Status FolderStatus // estado alcanzado tras la operación
    Err    error        // error específico si falló o quedó parcial
}

type Result struct {
    Mode           string         // "backup" o "purge"
    BackupPath     string         // ruta del directorio de respaldo si se usó
    Folders        []FolderResult // resultado por cada carpeta objetivo
    Preserved      []string       // ["saves", "versions", "assets", "libraries", "shaderpacks"]
    Errors         []error        // resumen de errores encontrados
    NothingToReset bool           // true si no existía ninguna carpeta objetivo
}
```

Esto permite que la UI muestre información exacta sin asumir estados binarios ni interpretar cadenas arbitrarias de error.

---

## Qué no debe hacer la primera versión

La primera versión no debería:

- Borrar `saves/`.
- Borrar `versions/`.
- Borrar `assets/`.
- Borrar `libraries/`.
- Borrar `shaderpacks/`.
- Reinstalar Minecraft.
- Descargar nuevamente todos los archivos.
- Modificar el perfil seleccionado en mcTUI.
- Cambiar automáticamente de Fabric a Vanilla.
- Detectar qué mod específico causó el crash.
- Administrar versiones individuales de cada mod.
- Intentar reparar cualquier crash posible.

El reset debe dejar una configuración modificada limpia, pero no debe prometer resolver problemas de Java, drivers, memoria, versiones incompatibles del juego o archivos del propio cliente.

---

## Relación con el gestor de mods futuro

`Resets Configs` es una buena primera etapa, pero no reemplaza un gestor de mods.

El reset opera sobre una única instalación global. Por eso, si el usuario alterna frecuentemente entre varios conjuntos de mods, todavía puede volver a tener conflictos.

El gestor de mods futuro debería considerar:

- Perfiles por versión de Minecraft.
- Listas de mods por perfil.
- Activar y desactivar mods sin borrarlos.
- Configuraciones separadas por perfil.
- Compatibilidad entre versión del juego, loader y mod.
- Copias de seguridad de cada perfil.
- Posiblemente una carpeta de instancia separada por configuración.

Una arquitectura futura más robusta sería:

```text
mctui/
├── instances/
│   ├── vanilla-1.20.4/
│   ├── fabric-1.20.4/
│   └── fabric-1.21.x-modpack/
└── backups/
```

Sin embargo, implementar instancias aisladas desde el inicio ampliaría bastante el alcance. `Resets Configs` permite resolver el problema inmediato sin esperar a ese rediseño.

---

## Plan de implementación por fases

### Fase 1: motor seguro

- Validar que `internal/config.MinecraftDir()` devuelva una ruta absoluta y no vacía, controlando posibles errores de `os.UserHomeDir()`. Mantener la ruta unificada `~/.minecraft` en macOS y Linux.
- Crear `internal/resetconfigs` para detectar y procesar `mods/` y `config/`.
- Validar las rutas objetivo y ancestros de backup (`mctui-backups`), rechazando enlaces simbólicos o junctions inesperadas.
- Crear un destino de backup exclusivo con timestamp dentro de `.minecraft` cuando corresponda.
- Mover carpetas con `os.Rename()` o eliminarlas según la elección del usuario; no copiar entre volúmenes.
- Verificar el resultado reportando estados parciales (`deleted`, `partially_deleted`, `moved`, `failed`).
- Agregar pruebas unitarias con carpetas temporales para rutas absolutas, inexistentes, symlinks y fallos parciales de borrado/movimiento.

### Fase 2: integración con la TUI

- Agregar la opción `Resets Configs` al menú y actualizar sus límites de navegación.
- Mostrar la ruta concreta, las carpetas afectadas y la advertencia de cerrar Minecraft.
- Permitir elegir el modo en cada ejecución y cancelar antes de operar.
- Exigir una confirmación explícita y distinta para la eliminación permanente.
- Mostrar éxito, ausencia de carpetas o fallos parciales sin prometer restauración desde mcTUI.

### Fase 3: protección adicional

- Evaluar si es posible detectar procesos de Minecraft con un alcance definido y comprobable en cada plataforma. La falta de detección no debe presentarse como garantía de que el juego esté cerrado.

#### Evaluación de la fase 3 (2026-09-24)

La detección **positiva** es viable, pero no hay un mecanismo único en la biblioteca estándar de Go para enumerar procesos y leer sus argumentos en Windows, Linux y macOS. Buscar solo `java`/`javaw` daría falsos positivos; para asociar un proceso con esta instalación habría que comprobar un argumento `--gameDir` cuya ruta coincida con `config.MinecraftDirChecked()` (el launcher de mcTUI ya lo pasa). Otros launchers pueden omitir ese argumento, y los permisos o una carrera entre inspección y reset pueden impedir una respuesta concluyente.

| Plataforma | Fuente candidata | Límite para validar |
| --- | --- | --- |
| Windows | `Win32_Process.CommandLine` vía CIM/WMI | Probar Java abierto, otro Java, argumentos no accesibles y rutas con espacios en Windows real; `tasklist` no expone la línea de comandos. |
| Linux | `/proc/<pid>/cmdline` (argumentos separados por NUL) | Probar proceso propio y lectura restringida (`hidepid`); un proceso puede terminar durante el recorrido. |
| macOS | Argumentos del proceso vía API específica del sistema o `ps` | Probar permisos y argumentos largos en macOS real; la salida textual de `ps` no es una fuente estructurada de argumentos. |

**Decisión:** no introducir todavía un bloqueo basado en procesos. No se puede comprobar su comportamiento multiplataforma desde este entorno ni usar una respuesta negativa como prueba de que Minecraft está cerrado. Se mantiene la advertencia explícita de la fase 2 y el manejo de errores de archivos en uso. Si se incorpora después, solo una coincidencia positiva comprobada debe bloquear el reset; resultado negativo o inaccesible debe mostrarse como *no verificado*, nunca como *Minecraft cerrado*. Antes de hacerlo obligatorio, ejecutar los casos de la tabla en las tres plataformas y volver a comprobar justo antes de operar; aun así existirá una carrera entre comprobación y modificación.

Referencias: [Win32_Process](https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-process), [tasklist](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/tasklist), [proc_pid_cmdline(5)](https://man7.org/linux/man-pages/man5/proc_pid_cmdline.5.html), [proc(5)](https://man7.org/linux/man-pages/man5/proc.5.html).

### Fase 4: historial y restauración

- [x] Listar respaldos existentes.
- [x] Permitir restaurar un respaldo.
- [x] Mostrar fecha y carpetas incluidas.
- [x] Si `mods/` o `config/` tienen contenido, informar cuáles y pedir confirmación explícita antes de reemplazarlos. La existencia de una carpeta vacía no cuenta como contenido que se vaya a perder.
- [x] En cada restauración, preguntar si se conserva el respaldo original o si se consume al restaurar; exigir una selección explícita y no eliminar respaldos sin confirmación.
- [x] No ofrecer una acción de borrar todos los respaldos. Cualquier eliminación de archivos locales requiere una decisión explícita y confirmada para ese caso.
- [x] Al conservar el backup original, copiar su contenido; si contiene enlaces simbólicos, rechazar esa copia para evitar seguir enlaces fuera de `.minecraft`.
- [x] Permitir elegir un directorio existente para el backup mediante el selector integrado en el panel central; la ruta predeterminada sigue siendo `.minecraft/mctui-backups`. Registrar directorios externos en `.minecraft/mctui-backups/locations.json` para que aparezcan en el historial y se puedan restaurar. Rechazar enlaces y destinos dentro de `mods/` o `config/`; los movimientos entre volúmenes no se copian y se informan como error.
- [x] Conservar el dashboard durante el flujo de reset; separar información y acciones visualmente, indicar líneas ocultas y destacar estados de éxito y ausencia de carpetas.

**Regla de decisión:** mcTUI no debe elegir por el usuario cuando una acción afecte archivos o directorios locales. Debe explicar qué se conservará, moverá o reemplazará, y pedir confirmación antes de ejecutar la operación destructiva. No se borran ni reemplazan carpetas activas o respaldos de forma implícita.

### Fase 5: gestor de mods

- Separar perfiles.
- Activar y desactivar mods.
- Asociar configuraciones a cada perfil.
- Evaluar instancias aisladas.

---

## Criterios de aceptación

La primera versión puede considerarse terminada cuando:

- El usuario puede iniciar `Resets Configs` desde la TUI.
- La TUI muestra las carpetas que serán afectadas.
- La TUI confirma que mundos, versiones y shaders se conservan.
- El usuario puede elegir entre reset sin backup y reset con backup.
- En el modo con backup, `mods/` y `config/` se mueven a un destino exclusivo con timestamp, sin sobrescribir respaldos anteriores.
- En el modo sin backup, solo se eliminan las rutas seleccionadas y protegidas por confirmación explícita.
- `saves/`, `versions/`, `assets/`, `libraries/` y `shaderpacks/` permanecen intactos.
- La operación se puede cancelar antes de ejecutarse.
- Se informa correctamente cuando no hay nada que resetear.
- Se informa correctamente ante errores parciales, indicando el estado y las rutas de cada carpeta afectada.
- Las rutas objetivo enlazadas o junctions inesperadas (incluyendo `mctui-backups/`) se rechazan antes de mover o eliminar.
- La TUI advierte que se debe cerrar Minecraft antes del reset y advierte sobre el riesgo de cargar mundos que dependan de mods eliminados. Los archivos bloqueados o en uso se informan sin anunciar éxito completo.
- Una instalación conocida (Vanilla o Fabric) vuelve a iniciar correctamente tras el reset en pruebas de validación.
- La operación funciona con rutas y separadores propios de Windows.
- La operación funciona con rutas Linux y macOS (`~/.minecraft`) sin depender de comandos del sistema.
- Windows es probado como plataforma principal, incluyendo archivos bloqueados, espacios en rutas y permisos normales de usuario.

---

## Decisión recomendada

Implementar primero `Resets Configs` como una limpieza de:

```text
mods/
config/
```

con backup opcional y protección explícita de:

```text
saves/
versions/
assets/
libraries/
shaderpacks/
```

Es una funcionalidad acotada, de alto valor para el usuario real de mcTUI y compatible con la arquitectura actual. Además, establece una base limpia para el futuro gestor de mods sin obligar todavía a rediseñar todo el sistema como un administrador de instancias.
