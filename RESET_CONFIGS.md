# Resets Configs

## Propuesta funcional

`Resets Configs` es una funcionalidad de mcTUI destinada a recuperar una instalación de Minecraft que dejó de iniciar correctamente por conflictos entre mods, loaders, configuraciones o archivos generados por distintas versiones.

El objetivo es ofrecer una limpieza segura desde la TUI, sin necesidad de abrir manualmente el explorador de archivos ni conocer la estructura interna de `.minecraft`.

La operación debe ser conservadora: no debe borrar mundos guardados, versiones descargadas, librerías, assets ni shaders.

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

En Linux se debe conservar el comportamiento actual de `config.MinecraftDir()`. En macOS se debe utilizar la ruta oficial de la plataforma:

```text
Linux:  ~/.minecraft/
macOS: ~/Library/Application Support/minecraft/
```

La resolución de estas rutas debe centralizarse en `internal/config`. `Resets Configs` no debe construir rutas suponiendo que el sistema utiliza `/`, `~` o una carpeta concreta del usuario.

### Estado actual de mcTUI

`internal/config.MinecraftDir()` ya contempla la ruta de Windows mediante `%APPDATA%`, pero actualmente agrupa Linux y macOS bajo la misma ruta `~/.minecraft`.

Para cumplir completamente esta especificación, esa función deberá distinguir macOS y utilizar:

```text
~/Library/Application Support/minecraft/
```

Este ajuste debe realizarse antes o junto con la implementación de `Resets Configs`, y debe cubrir también las demás funcionalidades que consumen `MinecraftDir()` para evitar que cada componente utilice una ubicación diferente.

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

La instalación oficial suele ubicarse en:

```text
~/Library/Application Support/minecraft/
```

La implementación debe utilizar `filepath.Join()` porque la ruta contiene espacios. No debe asumir que todas las instalaciones están bajo `~/.minecraft`.

También debe contemplarse que macOS puede bloquear archivos mientras Minecraft está abierto, de forma análoga a Windows.

### Matriz mínima de comportamiento

| Sistema | Ruta esperada | Prioridad | Riesgo principal |
|---|---|---:|---|
| Windows | `%APPDATA%\\.minecraft` | Alta | Archivos bloqueados y permisos |
| Linux | `~/.minecraft` | Media | Permisos y launchers alternativos |
| macOS | `~/Library/Application Support/minecraft` | Media | Espacios en rutas y archivos bloqueados |

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

Antes de operar, comprobar con `os.Lstat()` las rutas de `.minecraft`, `mods/`, `config/` y el destino de backup. Si alguna ruta que se va a operar es un enlace simbólico o una junction inesperada, rechazar la operación y explicarlo. En la primera versión se mueven o eliminan las carpetas objetivo completas; no se recorren sus archivos internos para decidir qué resetear. Volver a validar inmediatamente antes de la operación reduce el riesgo de que las rutas hayan cambiado desde la pantalla informativa.

### Minecraft cerrado

La primera versión debe advertir que Minecraft debe estar cerrado antes de confirmar. Si se incorpora detección de procesos y se encuentra el juego activo, debe detenerse la operación; no detectar un proceso no garantiza que esté cerrado. Los bloqueos de archivos detectados durante el movimiento o borrado deben informarse como errores, sin anunciar éxito completo.

Modificar `mods/` o `config/` mientras el juego está escribiendo archivos podría generar un respaldo inconsistente.

### Operación reversible o destructiva

Cuando se elige backup, mover carpetas dentro de `.minecraft` con `os.Rename()` evita copiar y borrar. Si el movimiento falla, mcTUI debe informar el error y conservar el origen; no debe intentar copiar entre volúmenes en la primera versión.

Cuando se elige el reset sin backup, la eliminación permanente debe ejecutarse únicamente sobre las rutas objetivo permitidas y después de una confirmación específica para ese modo. No debe reutilizarse una confirmación genérica de movimiento.

### Fallos parciales

La implementación debe contemplar que `mods/` exista pero `config/` no, o viceversa.

Si una carpeta se mueve o elimina correctamente y la otra falla, no se debe anunciar éxito completo ni intentar revertir automáticamente lo ya modificado. Se deben detener las operaciones restantes e informar qué carpeta quedó en origen, cuál fue movida o eliminada y, en modo backup, la ruta exacta de su destino. En modo sin backup, una carpeta ya eliminada no puede recuperarse desde mcTUI.

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

- Un nuevo estado de pantalla para `Resets Configs`.
- Una pantalla de información y confirmación.
- Una pantalla de progreso o resultado.
- Mensajes de éxito y error.
- Una nueva opción en el menú.

El patrón puede reutilizar la pantalla existente de confirmación y el sistema de mensajes de descarga, pero el reset debe ser sincrónico o ejecutarse en una tarea controlada, ya que normalmente mover carpetas es rápido.

### `main.go`

No debería ejecutar directamente la operación. La lógica debe vivir en su paquete propio y ser invocada por la UI, igual que la UI actualmente decide si el usuario seleccionó jugar.

---

## Modelo de resultado sugerido

El paquete podría devolver un resultado estructurado similar a:

```go
type Result struct {
    BackupPath       string
    Moved            []string
    Skipped          []string
    Preserved        []string
    Errors           []error
    NothingToReset   bool
}
```

Esto permitiría que la UI muestre información precisa sin interpretar mensajes de texto del sistema de archivos.

Los nombres y campos pueden ajustarse durante la implementación.

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

- Ajustar `internal/config.MinecraftDir()` para macOS, compartiendo la ruta con el resto de funcionalidades.
- Crear `internal/resetconfigs` para detectar `mods/` y `config/`.
- Validar las rutas objetivo, rechazar enlaces simbólicos o junctions inesperadas y repetir la validación antes de operar.
- Crear un destino de backup exclusivo dentro de `.minecraft` cuando corresponda.
- Mover carpetas con `os.Rename()` o eliminarlas según la elección del usuario; no copiar entre volúmenes.
- Verificar el resultado y devolver errores parciales con las rutas afectadas.
- Agregar pruebas con carpetas temporales para rutas, carpetas inexistentes, enlaces y fallos parciales.

### Fase 2: integración con la TUI

- Agregar la opción `Resets Configs` al menú y actualizar sus límites de navegación.
- Mostrar la ruta concreta, las carpetas afectadas y la advertencia de cerrar Minecraft.
- Permitir elegir el modo en cada ejecución y cancelar antes de operar.
- Exigir una confirmación explícita y distinta para la eliminación permanente.
- Mostrar éxito, ausencia de carpetas o fallos parciales sin prometer restauración desde mcTUI.

### Fase 3: protección adicional

- Evaluar si es posible detectar procesos de Minecraft con un alcance definido y comprobable en cada plataforma. La falta de detección no debe presentarse como garantía de que el juego esté cerrado.

### Fase 4: historial y restauración

- Listar respaldos existentes.
- Permitir restaurar un respaldo.
- Mostrar fecha y carpetas incluidas.
- Evitar sobrescribir una configuración activa durante una restauración.

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
- Las rutas objetivo enlazadas o junctions inesperadas se rechazan antes de mover o eliminar.
- La TUI advierte que se debe cerrar Minecraft antes del reset; los archivos bloqueados o en uso se informan sin anunciar éxito completo. Si se agrega detección de procesos, un Minecraft detectado bloquea la operación.
- El juego puede volver a iniciarse después del reset.
- La operación funciona con rutas y separadores propios de Windows.
- La operación funciona con rutas Linux y macOS sin depender de comandos del sistema.
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
