# Fase 5 — Plan de implementación: instancias y gestor de mods integrado en la TUI

> Estado: 5A en curso; el núcleo 5A–5D y la ampliación 5E siguen pendientes.
> Base de código revisada: `c7e7af9` — `Integrate reset configs backup restore flow`.
> Antecedente: [RESET_CONFIGS.md](RESET_CONFIGS.md), sección «Fase 5: gestor de mods».
> Objetivo solicitado: integración completa en mcTUI, tomando Prism Launcher como referencia funcional.
> Este documento conserva la investigación, la propuesta técnica, las tareas y las decisiones abiertas. Una propuesta aquí no equivale a una funcionalidad implementada ni a una decisión de producto aprobada.

## 1. Objetivo y definición de alcance

Pasar de una única instalación global a instancias independientes que se puedan crear, configurar, administrar y lanzar desde la TUI. Instalar mods no debe exigir manipular carpetas a mano ni contaminar otras instalaciones.

La referencia a Prism es funcional: aislamiento por instancia, gestión de mods locales, catálogo, versiones, dependencias, modpacks y recuperación. No implica copiar su interfaz gráfica, su implementación ni prometer compatibilidad automática con su formato de instancias.

### Resultado esperado

- Crear, seleccionar, renombrar y duplicar instancias.
- Elegir Minecraft, loader y su versión exacta, Java y memoria por instancia.
- Separar mods, configuraciones, mundos y ajustes del juego.
- Agregar mods locales; activarlos, desactivarlos y retirarlos con confirmación.
- Buscar e instalar desde Modrinth con validación de compatibilidad y dependencias.
- Revisar actualizaciones compatibles y confirmar el conjunto de cambios.
- Importar `.mrpack` a una instancia nueva; desarrollar exportación y actualización de packs con protección de cambios locales.
- Aplicar Reset Configs y restauraciones únicamente a la instancia elegida.
- Mantener la navegación, los resultados, el progreso y los errores dentro de la TUI.
- Gestionar el lanzamiento y la salida del juego sin confundirlos con el fin de una descarga.

### Dos niveles de cierre, sin ocultar alcance

1. **Núcleo completo (5A–5D):** instancias, TUI integrada, mods locales, Modrinth, actualizaciones, importación/exportación/actualización de packs y recuperación segura. Vanilla y Fabric son los primeros entornos verificables.
2. **Paridad ampliada (5E):** CurseForge, otros loaders, autenticación Microsoft y funciones avanzadas. Se conserva todo como trabajo explícito; depende de decisiones de producto, aprobaciones y validación técnica. No anunciar paridad total con Prism mientras estos puntos estén pendientes.

No borrar del seguimiento una capacidad por quedar para una entrega posterior. Registrar si está pendiente, bloqueada o excluida mediante decisión explícita.

## 2. Hallazgos de la investigación

### 2.1. Referencia Prism Launcher

Su documentación describe:

- Creación de instancias seleccionando Minecraft y loader o importando un pack [S1].
- Lista de mods que refleja la carpeta de cada instancia, con nombre/versión y desactivación temporal [S2].
- Descarga desde Modrinth y CurseForge, con filtros y selección previa [S3].
- Copia selectiva de mundos, mods, opciones, recursos, shaders y otros datos [S4].
- Importación de packs `.zip` y `.mrpack`; algunos archivos de CurseForge pueden requerir intervención fuera del launcher [S5].
- Selección de Java y memoria, incluidos ajustes específicos de instancia [S6].

**Aplicación a mcTUI:** adoptar el modelo centrado en instancias, no limitarse a agregar una pantalla de descarga de `.jar`.

### 2.2. Modrinth

- La búsqueda admite filtros por tipo de proyecto, versión de Minecraft y loader [S7].
- Verificar también la versión concreta del archivo: un resultado de búsqueda no acredita compatibilidad de todos sus releases [S8].
- Dependencias declaradas: `required`, `optional`, `incompatible`, `embedded` [S8].
- Persistir IDs estables de proyecto y versión, no usar slugs o nombres como identidad [S9].
- API con `User-Agent` identificable obligatorio. La documentación consultada indica 300 solicitudes/minuto por IP; leer encabezados de límite y tratar 429, no asumir que la cifra será permanente [S9].
- Metadata de compatibilidad no garantiza ausencia de conflictos de ejecución. Mostrar «compatible según metadata», no «garantizado sin crashes».

### 2.3. CurseForge

- Requiere solicitar acceso y clave propia aprobada [S10].
- Revisar términos antes de diseñar distribución de credenciales, caché y persistencia: incluyen restricciones sobre compartir claves, guardar datos de API y uso del servicio [S11].
- Respetar permisos de distribución por proyecto/archivo y las respuestas de la API [S12].
- No reutilizar claves de otro launcher, adivinar URLs para eludir restricciones ni construir un proxy como supuesto atajo permitido.
- Un binario de escritorio no puede mantener secreta una clave simplemente incrustándola. Acordar un mecanismo autorizado con el proveedor.
- Revalidar condiciones y requisitos de descarga al implementar. No considerar rumores o resúmenes de buscador como contrato vigente.

**Consecuencia:** integración condicionada, no promesa de descarga universal ni de experiencia 100% interna para contenido restringido.

### 2.4. `.mrpack`

ZIP con `modrinth.index.json` en la raíz, versión de formato, juego, dependencias de Minecraft/loader y lista de archivos con destinos relativos, tamaños, URLs HTTPS y hashes SHA-1/SHA-512 [S13].

- Respetar `env.client`: required, optional o unsupported.
- Aplicar `overrides/` y después `client-overrides/`; no aplicar `server-overrides/` en un cliente.
- Las dependencias del índice (Minecraft/loader) no son el mismo esquema que las dependencias entre versiones de mods de la API.
- Rechazar formatos o dependencias de runtime no soportados con explicación, sin inventar equivalencias.
- Importación no es extracción directa: validar rutas, enlaces, límites, URLs, hashes y colisiones.
- Exportar exige respetar licencias y permisos: preferir referencias a descargas autorizadas; no redistribuir indiscriminadamente JARs locales.

### 2.5. Loaders y autenticación

Fabric Meta permite descubrir versiones y solicitar el perfil correspondiente al par Minecraft/loader [S14]. Fabric API es un mod/dependencia, no el propio Fabric Loader.

Forge/NeoForge no se implementan cambiando el nombre del filtro: requieren instalación y lanzamiento compatibles con sus perfiles, bibliotecas y procesadores. Definir versiones soportadas y validar el flujo real de cliente; no extrapolar instrucciones de servidor [S15].

mcTUI actualmente lanza offline/LAN. Gestión de mods no incorpora autenticación oficial por sí sola. Si la meta incluye servidores autenticados, planificar Microsoft/Minecraft como trabajo separado con credenciales protegidas y sin mecanismos de evasión.

## 3. Estado real del repositorio y cambios necesarios

| Ubicación | Estado observado | Cambio necesario |
| --- | --- | --- |
| `internal/config/config.go` | Una versión, loader y memoria globales; ruta Minecraft global | Preferencias globales separadas de metadata de instancia; rutas explícitas y persistencia con errores |
| `main.go` | Sale de Bubble Tea y después invoca `launcher.Launch` | Integrar ciclo de lanzamiento con mensajes y resultados en la TUI |
| `internal/launcher/launcher.go` | Misma raíz para juego, assets y bibliotecas; fallback a Vanilla si Fabric falla | Separar gameDir de artefactos; abortar si el entorno solicitado no puede prepararse |
| `internal/fabric/fabric.go` | Elige primer loader devuelto; caché por versión Minecraft y classpath con rutas | Fijar loader exacto; cachear por identidad completa; no exportar rutas absolutas como configuración portable |
| `internal/mojang/manifest.go` | Descargador sin error retornado; acepta archivo existente no vacío; escribe al destino | Propagar errores, comprobar HTTP y contenido, temporales y publicación segura |
| `internal/ui/download.go` | Puede finalizar sin error si falta metadata; lógica de preparación duplicada con launcher | Una preparación reutilizable, con progreso y fallos estructurados |
| `internal/ui/update.go` | `DownloadDoneMsg` activa Play y cierra TUI; cambios globales | Operaciones distintas para preparar, instalar mods y jugar; capturar instancia objetivo |
| `internal/ui/model.go`, `view.go` | Estado centrado en versión/loader globales | Instancia activa visible, subflujos de mods, catálogo, packs y tareas |
| `internal/worlds/worlds.go` | Ya recibe un directorio | Reutilizar pasando el gameDir seleccionado, sin cambiar de versión global por un mundo |
| `internal/resetconfigs/reset.go` | Entradas públicas resuelven raíz global; historial por carpeta | Recibir contexto de instancia y separar historial/origen de backups |
| `internal/java/java.go` | Detección de runtime existente | Reutilizar, agregando override y validación por instancia |

### Bloqueantes previos al catálogo

- [ ] No caer automáticamente de Fabric a Vanilla: mostrar error y conservar la instancia.
- [ ] No anunciar instalación completa cuando falta metadata o falló una descarga.
- [ ] No usar «existe y no está vacío» como única evidencia de instalación válida.
- [ ] No duplicar motores de preparación entre la UI y el launcher.
- [ ] No permitir que cambiar de selección durante una tarea cambie su destino.
- [ ] No compartir estado mutable de juego entre instancias.

Las comprobaciones previas a este plan pasaron tests, race, vet y compilación cruzada. Eso valida el estado anterior, no esta fase aún inexistente ni ejecución real en Windows/macOS.

## 4. Propuesta de arquitectura

### 4.1. Rutas y almacenamiento

Layout conceptual, pendiente de fijar ubicaciones por sistema:

```text
datos-mctui/
├── instances/
│   └── <id-estable>/
│       ├── instance.json
│       ├── mods.json              # origen/versiones/hash de archivos gestionados
│       └── minecraft/
│           ├── mods/
│           ├── config/
│           ├── saves/
│           ├── resourcepacks/
│           ├── shaderpacks/
│           └── options.txt
├── runtime/                       # artefactos reutilizables, no datos del jugador
│   ├── assets/
│   ├── libraries/
│   └── versions/
└── backups/
    └── <id-estable>/
```

- `gameDir`: datos mutables de una instancia; se pasa a `--gameDir` y como directorio de trabajo del proceso.
- Raíz de runtime: cliente, assets, bibliotecas y perfiles identificados por versión/loader/OS/arquitectura según corresponda.
- No es seguro compartir natives solo por versión si difieren OS o arquitectura.
- Compartir artefactos verificados mediante rutas, no enlazar saves o config entre instancias.
- No eliminar artefactos compartidos al borrar una instancia.
- Revisar la raíz actual `AppDataDir`: hoy usa `~/.config` fuera de Windows. Separar configuración y datos voluminosos y documentar la elección por OS antes de migrar.
- El nombre visible no es una ruta. IDs y destinos deben validarse en la frontera de entrada.

### 4.2. Metadata mínima

`instance.json` necesita versión de esquema, ID estable, nombre, tipo (gestionada/heredada), Minecraft, loader y versión exacta, ajustes de Java/memoria y origen de pack si existe.

`mods.json` registra únicamente información gestionada útil: proveedor, ID de proyecto/versión, archivo, hash, estado y origen de instalación. No reemplaza el escaneo del filesystem.

- Un `.jar` desconocido debe aparecer aunque no esté en el registro.
- Si un archivo fue modificado externamente, marcarlo; no sobrescribirlo automáticamente.
- Una instancia con modpack necesita la lista/hash de archivos de la versión base para comparar actualizaciones con cambios locales.
- JSON versionado y escrituras seguras con errores visibles; no hace falta una base de datos ni un sistema de plugins.
- Separar preferencias globales (última instancia, UI, cuenta) de configuración de juego.

### 4.3. Límites de implementación

Reutilizar paquetes existentes. Agregar módulos concretos para instancias, mods, Modrinth y packs cuando su hito los necesite; los nombres exactos se deciden al implementar. La UI no debe resolver dependencias ni realizar transacciones de archivos.

Usar Go estándar para HTTP, JSON, ZIP, hashes, procesos y contexto, y Bubble Tea/Bubbles/Lip Gloss existentes para la interfaz. Si se necesita TOML para metadata de loaders, evaluar una biblioteca mantenida en ese momento; no escribir un parser incompleto ni incorporar dependencias preventivamente.

No hace falta copiar arquitectura C++/Qt, introducir ORM, servicio remoto o abstracción de proveedores con una sola implementación. Si se reutiliza código de terceros, revisar licencia; estudiar comportamiento no autoriza copiarlo sin condiciones.

## 5. Migración de la instalación existente

- [ ] Representar la `.minecraft` actual como «Instalación existente», indicando que otros launchers pueden modificarla.
- [ ] Mantener la resolución actual Windows y Linux/macOS para esa instalación; no convertir automáticamente macOS a otra ruta.
- [ ] Crear nuevas instancias sin mover ni borrar la instalación heredada.
- [ ] Ofrecer copia selectiva de mods, configuraciones, mundos, recursos, shaders y opciones.
- [ ] Mostrar origen, destino, contenido, espacio estimado y consecuencias antes de copiar.
- [ ] Copiar, no mover, en la migración inicial. No seguir enlaces ni compartir mundos por hard links.
- [ ] Publicar la instancia nueva solo cuando esté lista; informar y permitir recuperar/limpiar restos parciales con consentimiento.
- [ ] Mantener backups históricos de Reset Configs accesibles desde su contexto heredado.
- [ ] No atribuir backups externos antiguos a una instancia nueva por coincidencia de nombre.
- [ ] Configuración antigua debe seguir cargando; guardar selección nueva sin perder datos previos.

Eliminar una instancia gestionada y quitar una referencia heredada son acciones distintas. Nunca ofrecer borrar toda la `.minecraft` heredada como equivalente a quitarla de la lista.

## 6. Experiencia integrada en la TUI

### Navegación propuesta

```text
Instancias → Instancia seleccionada
               ├── Jugar / estado del proceso
               ├── Mods instalados → activar / desactivar / agregar / retirar
               │                      └── Buscar → detalles → plan → confirmar
               ├── Actualizaciones → cambios → confirmar
               ├── Mundos
               ├── Ajustes: Minecraft / loader / Java / memoria
               ├── Modpack: importar / exportar / actualizar
               ├── Reset Configs / historial / restaurar
               └── Logs y resultado de operaciones
```

- [ ] Mostrar nombre, versión, loader y ruta de la instancia en confirmaciones.
- [ ] Teclado completo, foco visible, ayuda de atajos; mouse opcional, nunca obligatorio.
- [ ] Filtros y búsqueda no deben bloquear `Update` ni `View`.
- [ ] Paginación y cancelación de búsquedas; descartar respuestas antiguas por ID de operación.
- [x] Mostrar progreso de carga en la TUI, no el contador de archivos completados como indicador principal. Usar progreso determinado solo cuando exista un total real (idealmente bytes); si no, indicador animado indeterminado, sin inventar porcentajes. Reutilizar Bubble Tea/Bubbles existentes; no agregar otra biblioteca solo por el indicador. Play muestra un segmento horizontal móvil de izquierda a derecha, sin porcentaje ni nombre de archivo.
- [ ] Errores distinguibles: red, permisos, disco, hash, incompatibilidad, contenido restringido.
- [ ] Confirmación de instalación con dependencias, reemplazos, descargas y destino.
- [ ] Cancelación segura de descargas; no interrumpir una publicación dejando archivos ambiguos.
- [ ] Logs acotados en memoria, con ruta al archivo; no inundar ni corromper el terminal.
- [ ] Sanitizar secuencias de control de títulos, descripciones y logs externos antes de renderizar.
- [ ] Diseño verificable en terminal pequeña, con scroll y rutas largas; no depender solo de colores.
- [ ] Finalizar una descarga de mods nunca dispara Play.
- [ ] Al terminar el juego, volver a una pantalla útil con estado/salida y acceso al log.

## 7. Seguridad e integridad de operaciones

### Descargas

- [ ] HTTP con contexto, timeout, validación de status, tamaños máximos y errores propagados.
- [ ] Política de redirecciones y hosts; no reenviar credenciales a destinos ajenos.
- [ ] Temporales en el volumen de destino, hash esperado y publicación solo después de verificar.
- [ ] Concurrencia acotada, backoff para errores transitorios y respeto de rate limits.
- [ ] No instalar respuestas HTML, contenido truncado o archivos con hash incorrecto.
- [ ] Revalidar archivos conocidos modificados; mantener arranque offline con artefactos completos.
- [ ] Un hash asegura integridad respecto de la fuente, no ausencia de malware: los mods ejecutan código.

### Archivos y packs

- [ ] Rechazar rutas absolutas, traversal, enlaces/junctions inesperados y escapes del destino.
- [ ] Tratar diferencias de mayúsculas, nombres reservados y colisiones de archivos en Windows.
- [ ] Limitar cantidad de entradas, tamaño expandido y profundidad de archivos ZIP.
- [ ] No ejecutar scripts incluidos en un pack como parte de una extracción.
- [ ] Validar URLs de packs; restringir esquemas/hosts y evitar accesos arbitrarios a servicios locales.
- [ ] No sobreescribir cambios locales ni eliminar archivos no gestionados de manera implícita.
- [ ] Recuperar o informar fallos parciales con rutas exactas; nunca prometer atomicidad multiarchivo de `os.Rename`.
- [ ] Definir publicación y recuperación compatibles con Windows; probar archivos abiertos y antivirus.

### Concurrencia y juego en ejecución

- [ ] Serializar mutaciones por instancia, incluyendo procesos simultáneos de mcTUI.
- [ ] Si mcTUI conoce que esa instancia está ejecutándose, bloquear operaciones incompatibles.
- [ ] No usar únicamente PID como prueba persistente: puede reutilizarse.
- [ ] Procesos iniciados por otros launchers siguen siendo «no verificados» si no se detectan con certeza.
- [ ] Mantener advertencia de cerrar Minecraft y manejo de archivos en uso. La fase 3 aplazada no se convierte mágicamente en detección multiplataforma completa.
- [ ] Al actualizar varios mods: preparar todos, verificar, aplicar el conjunto y registrar recuperación ante interrupción. Una falla no debe anunciar éxito global.

## 8. Roadmap ejecutable

Las casillas reflejan el cierre completo de cada tarea; los avances parciales se detallan en §10. Completar un hito requiere código, comprobaciones y evidencia registrada, no solo pantallas navegables.

### 5A — Instancias, runtime y migración

Depende de: estado actual de fases 1–4.

- [x] **A01** Resolver decisiones de rutas/esquema y documentarlas en la sección 10 (diseño; implementación pendiente).
- [x] **A02** Separar gameDir y runtime; revisar todos los callers de `MinecraftDir()` (Play, mundos y Reset/restore usan rutas de instancia; validación de ejecución real completada y confirmada en §9).
- [x] **A03** Persistir/listar/crear/seleccionar/renombrar instancias con ID estable (API y TUI completas para listar, crear Vanilla, cambiar activa y renombrar; validación de corte en ejecución completada y confirmada en §9).
- [x] **A04** Instancia heredada y copia selectiva no destructiva (copia manual de `saves/`; usuario confirma origen intacto e independencia del destino; ver §10.12).
- [x] **A05** Duplicación independiente reutilizando la copia selectiva de A04; `saves/` solo se incluye si se selecciona explícitamente. Validación manual reportada por el usuario (§10.13).
- [x] **A06** Ajustes por instancia; versión Minecraft y loader inmutables tras crear managed; Java y memoria validados (selector Java automático/override persistido y preflight unificado). La TUI permite elegir versión al crear Vanilla y fijar versión exacta de Fabric Loader con caché aislada (commit `784fc78`, DEC-05 resuelta).
- [x] **A07** Preparación común con errores; corregir éxito falso y fallback silencioso (UI y lanzador reutilizan `launcher.Prepare`; índice/natives en §10.4 y contexto/integridad/cancelación en §10.5; errores tipados con `PrepareError` en `8f32920` y preparación con loader exacto en `8da7bf9`; validación física en Windows/macOS diferida sin bloquear hitos posteriores).
- [x] **A08** Lanzamiento integrado con resultado/proceso/log en TUI; fijar `cmd.Dir` (supervisión en TUI, aislamiento de log y lock durante `Wait` validados en ejecución real en §9; API de proceso con `Kill()` en `9058a72` y confirmación explícita integrada en la TUI).
- [x] **A09** Mundos referidos a la instancia seleccionada (mundos aislados por instancia validados en §9; aviso de versión en TUI descartado por decisión de diseño: redundante frente a la advertencia nativa del propio Minecraft).
- [x] **A10** Reset/restore con raíz explícita e historial identificado por instancia (legacy conserva historial; managed usa `backups/<id>` con selección fijada; lock entre procesos e independencia validados en ejecución real en §9).
- [x] **A11** Controles de concurrencia y confirmación de operaciones destructivas (Reset/restore y Play comparten lock OS por gameDir probado en ejecución real con «instance busy»; mutaciones de copia/retiro cubiertas con locks; cierre forzado desde TUI integrado; límite de orfandad externa documentado en §10.8).
- [x] **A12** Soft delete a papelera recuperable desde la TUI y full delete de juego + backups con confirmación; pruebas manuales confirmadas por el usuario (§10.14).

**Salida:** A y B mantienen mods, configuraciones y mundos independientes; jugar, resetear o restaurar A no cambia B. La instalación heredada y sus backups siguen accesibles. Fallar Fabric no inicia Vanilla.

### 5B — Gestor local de mods

Depende de: 5A.

- [x] **B01** Escanear carpeta, registrar origen y mostrar archivos desconocidos (`internal/mods.Store.Scan` y registro versionado en `instances/<id>/mods.json`).
- [x] **B02** Leer metadata de JAR como ZIP sin ejecutarlo, inicialmente formatos soportados; limitar lecturas (`Inspect` con límites estrictos, verificación de CRC, cuota de descompresión y lectura segura de `fabric.mod.json`).
- [x] **B03** Agregar `.jar` desde selector integrado; validar origen, destino y colisiones (`Store.Add` con staging temporal, reserva atómica de destino y picker de archivos `.jar` en TUI).
- [x] **B04** Activar/desactivar de forma reversible; decidir convención de archivo/carpeta y verificar que el loader no lo cargue (`.jar` ↔ `.jar.disabled`, DEC-04 en §10.16; upstream Fabric ignora `.disabled`).
- [x] **B05** Retirar con confirmación; no borrar configuraciones ni dependencias usadas por otros mods automáticamente (`Store.Remove` con diálogo de confirmación explícito `[y/n]` en TUI; `config/` intacto).
- [x] **B06** Señalar IDs duplicados cuando puedan identificarse, archivos corruptos y metadata desconocida (marcado en UI como Duplicado, Corrupto o Desconocido).
- [x] **B07** Detectar cambios externos al refrescar y antes de modificar (validación de hashes SHA-256 y timestamps mtime bajo `operationlock.Acquire`).
- [x] **B08** Mantener advertencia sobre mundos dependientes de mods eliminados/desactivados (advertencia visible permanente en cabecera de la TUI de mods).

**Salida:** importar, desactivar, reactivar y retirar funcionan offline, sin pérdida de contenido al alternar estados ni modificaciones en otra instancia.

### 5C — Modrinth, dependencias y actualizaciones

Depende de: 5A, 5B y descargas seguras.

- [ ] **C01** Cliente API con User-Agent, límites, errores, cancelación y caché permitida.
- [ ] **C02** Búsqueda paginada filtrada por instancia y detalles del proyecto.
- [ ] **C03** Elegir archivo/version compatible; no confiar en `latest_version` del buscador.
- [ ] **C04** Grafo de dependencias: IDs/versiones exactas, required/optional/incompatible/embedded, ciclos y conflictos.
- [ ] **C05** Si una dependencia no puede resolverse con certeza, detener y explicar; no instalar un conjunto parcialmente válido en silencio.
- [ ] **C06** Presentar plan completo y selección explícita de opcionales antes de instalar.
- [ ] **C07** Descarga verificada y aplicación del plan; persistir procedencia y hashes.
- [ ] **C08** Buscar actualizaciones compatibles, fijar versiones y elegir canal release/beta/alpha mediante política visible.
- [ ] **C09** Resolver dependencias para el conjunto actualizado, no actualizar cada archivo a «lo último» independientemente.
- [ ] **C10** Detectar archivos locales modificados; confirmación diferenciada antes de reemplazar.
- [ ] **C11** Retener información de recuperación del conjunto anterior y mostrar fallos parciales.
- [ ] **C12** No autoeliminar dependencias compartidas ni instalar nuevamente dependencias embedded.

**Salida:** instalar un mod con dependencias y actualizarlo deja una selección reproducible. Incompatibilidad, 429, desconexión o hash inválido no se anuncian como éxito ni disparan Play.

### 5D — Modpacks completos

Depende de: 5A–5C.

- [ ] **D01** Importar `.mrpack` local; catálogo/URL autorizado reutiliza descarga validada.
- [ ] **D02** Validar índice, versión de formato, runtime, rutas, URLs y límites de ZIP.
- [ ] **D03** Mostrar nombre, Minecraft, loader, archivos, tamaño y opciones cliente.
- [ ] **D04** Instalar en instancia nueva preparada fuera de la lista activa hasta completarse.
- [ ] **D05** Aplicar archivos verificados, overrides y client-overrides en orden; excluir server-only.
- [ ] **D06** Loader no soportado: explicar y rechazar activación, nunca sustituirlo por Fabric/Vanilla.
- [ ] **D07** Guardar identidad/version de pack y manifiesto de archivos base.
- [ ] **D08** Actualización mediante comparación base anterior / local / nueva versión; listar agregados, retirados, reemplazos y conflictos.
- [ ] **D09** Preservar modificaciones locales o pedir decisión concreta; no pisar `config/` u `options.txt` implícitamente.
- [ ] **D10** Exportar `.mrpack` con referencias permitidas y overrides seleccionados; excluir credenciales, logs y mundos por defecto.
- [ ] **D11** Reportar archivos no exportables por falta de fuente/permisos, sin omitirlos silenciosamente ni redistribuirlos sin autorización.
- [ ] **D12** Recuperación ante fallo/interrupción y validación de importación del pack exportado.

**Salida:** importar, actualizar y exportar un pack soportado es operable desde la TUI; permisos o conflictos no resolubles se presentan como bloqueos explícitos. No se afirma compatibilidad con todo ZIP ni importación de Prism/MultiMC.

### 5E — Paridad ampliada y dependencias externas

Depende de: núcleo y decisiones de alcance. Cada bloque tiene cierre independiente.

- [ ] **E01 — CurseForge:** aprobación/clave propia, revisión de términos, estrategia autorizada de credenciales y almacenamiento.
- [ ] **E02 — CurseForge:** búsqueda, versiones, dependencias, permisos y actualización; archivos restringidos con explicación e importación manual autorizada cuando corresponda.
- [ ] **E03 — Loaders:** decidir Forge/NeoForge/Quilt y matriz Minecraft/OS/Java; implementar instalación/lanzamiento real y pruebas por cada combinación soportada.
- [ ] **E04 — Cuentas oficiales:** definir integración Microsoft/Minecraft autorizada, validar propiedad/perfil, refresh/revocación y almacenamiento seguro; no registrar tokens en logs ni exportaciones.
- [ ] **E05 — Java administrado:** decidir si descargar runtimes o conservar detección/selección local; licencias, origen, integridad y arquitectura verificadas si se agrega descarga.
- [ ] **E06 — Recursos/shaders:** gestión local/remota en paneles propios, sin incluirlos silenciosamente en Reset Configs.
- [ ] **E07 — Interoperabilidad Prism/MultiMC:** investigar formatos y versiones antes de prometer importación; mostrar componentes no soportados.
- [ ] **E08 — Copia avanzada:** evaluar reflinks/CoW si copiar resulta costoso; no enlazar datos mutables por defecto.

**Salida:** solo marcar cada capacidad como soportada tras evidencia de funcionamiento. Una aprobación externa pendiente bloquea ese bloque, no debe quedar escondida tras «fase completa».

## 9. Matriz de validación

Usar tests Go con directorios temporales, `httptest` y procesos simulados para lógica local; no tocar `.minecraft` real en pruebas automatizadas. No agregar framework por defecto.

| Área | Caso mínimo | Resultado obligatorio |
| --- | --- | --- |
| Aislamiento | Dos instancias, misma versión/loader | Mods/config/saves independientes |
| Migración | Copia de instalación existente, error a mitad | Origen intacto; destino parcial identificable |
| Migración manual | Copiar datos seleccionados desde legacy | Usuario confirma origen intacto y copia independiente (§10.12) |
| Runtime | Dos loaders exactos para un Minecraft | Cachés y arranques no se confunden |
| Loader | Fabric no resoluble | Error, nunca Vanilla implícito |
| Descargas | 404, HTML, timeout, truncado, hash incorrecto | Error visible; ningún archivo activo inválido |
| Dependencias | Requerida, opcional, embedded, ciclo, incompatible | Plan coherente o bloqueo explicado |
| Actualización | Mod local alterado y dependencia compartida | No reemplazar/eliminar sin autorización |
| Packs | Traversal, ruta Windows absoluta, ZIP grande, enlace | Rechazo antes de modificar fuera del destino |
| Overrides | Config editada frente a nueva versión | Conflicto visible y decisión explícita |
| Reset | Restaurar A con B seleccionada después de iniciar | Destino sigue siendo A |
| Proceso | Juego activo o segunda TUI modifica misma instancia | Operación incompatible bloqueada |
| Cancelación | Cancelar descarga/importación | Sin falso éxito ni instancia anunciada lista |
| Offline | Instancia instalada, API inaccesible | Gestión local disponible y Play con runtime completo |
| Terminal | Pantalla pequeña, nombres largos, controles externos | Navegación utilizable y salida sanitizada |
| Exportación | Mod local sin permiso/fuente, posible token | Bloqueo/advertencia; ningún secreto exportado |

### Pruebas manuales de versiones

La entrada real a Minecraft por versión/loader se valida manualmente por el usuario en el equipo objetivo; el agente no debe intentar instalar y arrancar por su cuenta cada versión. El agente cubre la lógica con tests automatizados, ayuda a preparar la matriz y analiza los resultados/logs aportados. Registrar por cada intento: fecha, commit/build, OS/arquitectura, Minecraft, loader y versión, Java, resultado de preparación/descarga, si llegó al menú, salida/código y `mctui_latest.log` (o la ruta disponible). Un test automatizado o una compilación cruzada no sustituye esta evidencia manual.

#### Protocolo para cerrar el corte vertical 5A

Usar un build de este árbol de trabajo (registrar `git rev-parse HEAD` y si hay cambios locales). En la TUI, crear dos instancias Vanilla de la misma versión con `i` → `n`; jugar A hasta el menú, salir normalmente y comprobar que `mctui_latest.log` quedó en el `minecraft/` de A. Repetir con B y confirmar que `mods/`, `config/` y `saves/` de A no aparecen en B. En A, probar backup y restauración desde Reset; cambiar a B y comprobar que sus datos e historial no cambiaron. Volver a la instalación heredada, comprobar que conserva su ruta e historial y jugarla sin alterar A/B. Durante una partida, intentar Reset desde otra TUI sobre esa misma instancia: debe responder «instance busy» sin mutaciones. **No** usar cierre forzado como prueba segura: si Java sobrevive a mcTUI, el lock puede liberarse; cerrar Minecraft antes de volver a operar.

Anotar en la tabla siguiente OS/arquitectura, build, versión, loader, Java, preparación, llegada al menú, salida y log. Hacerlo en cada plataforma que se anuncie compatible; los binarios compilados para otro OS no prueban ejecución. No marcar A02/A03/A06/A08/A10/A11 como cerradas solo por tests o compilación.

**Cierre de validación manual del corte vertical 5A (2026-09-26):** El usuario confirma y da por cerrado el protocolo completo en Linux x86_64:
- Creación de instancias Vanilla independientes (`i` → `n`).
- Arranque y juego hasta el menú principal con salida limpia (código 0) y comprobación de `mctui_latest.log` en el `minecraft/` de cada instancia.
- Aislamiento estricto verificado: `mods/`, `config/` y `saves/` de una instancia no contaminan la otra.
- Reset/restore probado en una instancia sin alterar datos ni historial de otra instancia ni de la instalación heredada.
- Instalación heredada (legacy) accesible e intacta, conservando su ruta e historial.
- Exclusión por lock en concurrencia probada: una segunda TUI recibe «instance busy» al intentar operaciones sobre la instancia activa durante el juego.
- Copia selectiva desde legacy (A04) y duplicación independiente (A05) validadas.
La validación en Windows y macOS queda diferida hasta disponer de entornos físicos de prueba, sin bloquear el avance sobre Linux.

#### Registro de intentos de ejecución real

| Fecha | Commit / Build | OS / Arch | Minecraft | Loader | Java | Prep / Descarga | Menú principal | Salida / Log |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| No confirmada | No confirmado | Arch Linux (reportado; arquitectura/build no confirmados) | 1.20.4 | Vanilla | No confirmado | Sin fallas (reportado; detalles no confirmados) | No confirmado | No confirmado |
| No confirmada | No confirmado | Arch Linux (reportado; arquitectura/build no confirmados) | 1.21.1 | Vanilla | No confirmado | Sin fallas (reportado; detalles no confirmados) | No confirmado | No confirmado |
| No confirmada | No confirmado | Arch Linux (reportado; arquitectura/build no confirmados) | 1.20.4 | Fabric (loader exacto no confirmado) | No confirmado | Sin fallas (reportado; detalles no confirmados) | No confirmado | No confirmado |
| No confirmada | `go run .` (árbol local) | Linux / Arch Linux reportado; arquitectura no confirmada | 1.21.2 | Vanilla, instancia `prueba` | Automático (Java 21+) | Preparación exitosa | Sí; salida normal, código 0 | `instances/50ebfeb76d00e58c87638b8e430aa64f/minecraft/mctui_latest.log` |
| No confirmada | `go run .` (árbol local) | Linux / Arch Linux reportado; arquitectura no confirmada | 1.21.2 | Vanilla, instancia `test Fase 5` | Manual Java 27 | Preparación exitosa | Sí; salida normal, código 0 | `instances/2efef7fa3925f5271b674d528bbda27f/minecraft/mctui_latest.log` |
| 2026-09-26 | `ccb659beffccf3716cca6966de7a48b98ba5f7e0` (modo de ejecución/estado local no registrados) | Linux x86_64 | 26.1 | Vanilla, instancia `A` | Automático; OpenJDK 27 en PATH (binario exacto no registrado) | Usuario reporta funcionamiento exitoso | Usuario reporta funcionamiento exitoso; evidencia detallada no transcrita | GameDir mostrado: `/home/agus-hy/.local/share/mctui/instances/2275cc15ad13672cd0ecad603758d93a/minecraft`; log esperado: `mctui_latest.log` dentro de ese directorio (no se registró su lectura) |
| 2026-09-26 | `2a9f5e9` (`go run .`) | Linux x86_64 | 1.21.2 / 26.1 | Vanilla, instancias A y B | Automático (Java 21+) | Preparación exitosa | Sí; llegada al menú y salida normal (código 0) | **Protocolo 5A cerrado:** aislamiento de carpetas (`mods`, `config`, `saves`), log en `minecraft/mctui_latest.log`, reset/restore independiente, legacy intacta y lock de exclusión («instance busy») confirmado por el usuario. |

### Plataformas

- [ ] Windows real: prioridad principal; espacios, Unicode, archivos bloqueados, rutas, permisos normales y antivirus.
- [x] Linux real: permisos, enlaces y lanzamiento de instancia aislada (protocolo 5A validado y cerrado en §9).
- [ ] macOS real: ubicación heredada conservada, Java y natives apropiados, lanzamiento.
- [ ] Prueba Vanilla y Fabric con una instalación conocida en cada plataforma declarada soportada.
- [ ] `go test ./...`, `go test -race ./...`, `go vet ./...`, formato y `git diff --check`.
- [ ] Compilación para cada OS/arquitectura anunciados. Compilar cruzado no sustituye pruebas de ejecución.

## 10. Decisiones pendientes antes de cada hito

Registrar decisión, motivo, fecha/commit y consecuencias. Las recomendaciones siguientes no autorizan cambios destructivos ni resuelven solas requisitos de producto.

| ID | Decisión | Recomendación de partida | Bloquea |
| --- | --- | --- | --- |
| DEC-01 | Directorios de configuración/datos/runtime por OS | Resuelto en §10.1; sin migración automática | A01 |
| DEC-02 | Caché compartida y compatibilidad con artefactos actuales | Resuelto en §10.2; no copiar ni enlazar artefactos heredados sin verificarlos | A02 |
| DEC-03 | Comportamiento de TUI al cerrar con juego activo | Resuelto en §10.9: `q`/Ctrl+C no desacoplan ni matan Java; esperar salida del juego | A08 |
| DEC-04 | Cómo desactivar archivos | Resuelta en §10.16: `.jar` ↔ `.jar.disabled` | B04 |
| DEC-05 | Política de versión/canal | Resuelta en §10.11: Minecraft y loader managed fijos tras crear; versión se elige al crear. Cambio posterior requiere flujo explícito de compatibilidad. | A06/C08 |
| DEC-06 | Recuperación/retención de actualizaciones | Resuelta en §10.17: respaldo en backups/mods_replaced/ con cuota de 1 versión anterior y rollback en TUI | C11/D12 |
| DEC-07 | Loaders/versions que se anuncian soportados | Vanilla/Fabric primero; ampliar con pruebas reales | D06/E03 |
| DEC-08 | Alcance contractual CurseForge | No implementación de acceso hasta tener vía autorizada | E01/E02 |
| DEC-09 | Autenticación oficial en esta entrega o siguiente | Mantener bloque separado; nunca confundir mods con autenticación | E04 |
| DEC-10 | Límites de descargas/ZIP y política de hosts | Resuelta en §10.18: dominios api.modrinth.com / cdn.modrinth.net, User-Agent canónico, tope 250 MiB y SHA-512 | C07/D02 |
| DEC-11 | Java descargado y formatos externos | Decisión separada con origen, licencia y matriz de soporte | E05/E07 |

### 10.1. A01 — rutas y esquema (2026-09-25; base `c7e7af9`)

**DEC-01, resuelta:** mantener `config.json` donde lo lee hoy `AppDataDir("mctui", "config.json")`: `%APPDATA%/mctui/config.json` en Windows y `~/.config/mctui/config.json` en Linux/macOS. No trasladar ese archivo ni cambiar el comportamiento de `Load`/`Save` en este corte. Los datos nuevos viven bajo una raíz independiente:

| OS | Raíz de datos nuevos | Instalación heredada (sin mover) |
| --- | --- | --- |
| Windows | `%LOCALAPPDATA%/mctui` (si falta: `~/AppData/Local/mctui`) | `%APPDATA%/.minecraft` (fallback actual) |
| Linux | `${XDG_DATA_HOME:-~/.local/share}/mctui` | `~/.minecraft` |
| macOS | `~/Library/Application Support/mctui` | `~/.minecraft` |

No depender de `XDG_CONFIG_HOME` para encontrar configuración anterior: el código actual no lo usa. Si una raíz nueva no puede resolverse, crearse o escribirse, mostrar el error; no usar el directorio de trabajo como fallback. `LOCALAPPDATA`/`XDG_DATA_HOME` deben ser rutas absolutas antes de usarse. Esta decisión no crea carpetas ni cambia las rutas actuales; implementar la resolución y sus pruebas en A02/A03.

Bajo esa raíz: `instances/<id>/instance.json`, `instances/<id>/minecraft/` para datos mutables, `runtime/` para artefactos compartidos y `backups/<id>/` para respaldos nuevos. El runtime permanece en datos, no en una caché descartable: eliminarlo arbitrariamente rompería el arranque offline. La instalación heredada usa `MinecraftDirChecked()` y conserva `.minecraft/mctui-backups` y su historial, incluso si se selecciona otra instancia. No se borran ni enlazan carpetas de jugador para ahorrar espacio. La reutilización de artefactos heredados se decide en DEC-02 (§10.2); su validación aún no está implementada.

**Esquema inicial:** `instance.json` versión 1, con `schema_version`, `id` opaco estable, `name` visible, `kind` (`managed` o `legacy`), `minecraft_version`, `loader` (`Vanilla` o `Fabric` inicialmente), `loader_version` exacta cuando aplique, `java_path` opcional y `memory_mb`. Reservar `legacy` como ID fijo de la referencia heredada; no escribir `instance.json` dentro de la `.minecraft` existente. `config.json` conserva los campos antiguos para esa referencia; la selección nueva se guarda en `selection.json` bajo la raíz de datos, sin reescribir la configuración heredada. Nombre visible y rutas externas nunca se usan como ID ni como segmento de ruta; rechazar IDs inválidos y versiones de esquema desconocidas. Escribir metadata nueva con temporal en el mismo directorio y propagación de errores; la recuperación ante interrupción y la semántica de renombrado en Windows deben validarse antes de prometer atomicidad. El registro de mods y procedencia de packs se añaden en sus hitos, no en A01.

**Consecuencia:** A01 fija el destino sin migrar datos. A02 debe separar `gameDir` de runtime; A03 debe materializar este esquema. Reconsiderar estas rutas solo mediante migración explícita que preserve la instalación previa.

### 10.2. Primer corte de 5A — runtime y persistencia (2026-09-25)

**DEC-02, resuelta:** nuevas instancias comparten `datos-mctui/runtime` y nunca los directorios mutables del juego; la instancia heredada conserva `.minecraft` como gameDir **y** runtime por ahora. No copiar ni enlazar automáticamente artefactos de la instalación heredada: la comprobación actual de «archivo no vacío» no acredita integridad. Solo importar/reutilizar tras verificar hashes y metadata en A07; mientras tanto se permite descargar de nuevo, informando el costo de espacio. No mover ni borrar artefactos previos.

Implementado en este corte: `DataDirChecked()` resuelve la raíz nueva sin crearla; `launcher.Launch` recibe `gameDir` y `runtimeDir` separados, pasa `--gameDir` y `--assetsDir` apropiados, fija `cmd.Dir`, y guarda logs en el gameDir. El aislamiento de natives por OS/arquitectura sigue pendiente antes de usar el runtime compartido en la TUI; no se modifica la caché heredada en este corte. `internal/instances` persiste metadata versionada con ID aleatorio, lista la referencia heredada sin escribir en ella y permite crear, leer, seleccionar y renombrar instancias gestionadas; rechaza IDs/rutas inválidas y enlaces en metadata. Fallar la resolución de Fabric ya no cae a Vanilla en el lanzador.

**Aún no listo para uso desde la TUI:** `main.go`, la preparación/descarga, los mundos, Reset Configs y sus vistas siguen operando sobre la instalación heredada. No seleccionar una instancia gestionada para jugar hasta que todos esos flujos compartan el mismo contexto fijado al iniciar cada tarea. Las descargas todavía pueden ocultar errores: A07 sigue pendiente. La selección persistida en `selection.json` no afecta el lanzamiento anterior hasta integrar A03 en la UI. No se considera A02/A03/A07 cerrada por este trabajo preparatorio.

### 10.3. Continuación de A07 — descargas verificadas (corte parcial)

`mojang.Download` comprueba HTTP, tipo HTML, lectura completa y SHA-1 publicado cuando existe; revalida archivos conocidos en caché y descarga a un temporal antes de publicar. Devuelve errores para que la TUI no anuncie éxito ni lance el juego tras fallar metadata, cliente, librerías o Fabric. El lanzador también bloquea el arranque ante fallos de descarga y ausencia de metadata/índice de assets, y rechaza índices con JSON o hashes malformados. Los tests locales cubren 404, HTML, truncado, caché alterada, índices inválidos y resultado fallido en TUI. SHA-1 procede del manifiesto oficial de Mojang: aquí verifica integridad frente a esa fuente, no es una firma ni prueba ausencia de malware.

**Continuación (este commit):** `launcher.Prepare` reúne metadata, cliente, bibliotecas, Fabric, assets y natives en una sola ruta con errores; tanto el Cmd de TUI como `Launch` la usan. La UI muestra progreso indeterminado real en vez de porcentajes estimados. Se comprueba caché offline, archivo alterado y traversal de rutas de bibliotecas con `httptest` y directorios temporales. La selección y el runtime se fijan al iniciar la tarea, pero `Launch` vuelve a verificar tras salir de TUI; no se considera lanzamiento integrado. Pasan `go test ./...`, `go test -race ./...`, `go vet ./...` y `git diff --check`.

**Pendientes al finalizar ese corte:** integridad del índice en caché y extracción de natives (atendidos en §10.4), URLs sin hash, timeout/contexto de metadata, cancelación y retorno del proceso dentro de TUI. La instalación gestionada sigue sin estar conectada a Play. No se ha validado en Windows/macOS reales.

### 10.4. Continuación de A07 — índice verificado y natives (2026-09-25)

Implementado sobre `e3079af`, en este commit:

- El índice de assets conserva el SHA-1 publicado en metadata y lo comprueba también en caché, antes de enumerar objetos. Un índice alterado aunque sea JSON válido se descarga de nuevo; si no puede repararse, falla la preparación. Hash ausente o inválido no equivale a instalación lista.
- Natives dejan de confiar en una carpeta no vacía. Se verifican los JARs y se reconstruye la extracción incluso offline. Los errores de descarga, ZIP, CRC, escritura y publicación llegan a `launcher.Prepare`, compartido por TUI y lanzamiento.
- Extracción serial en staging privado; solo después de extraer todo se publican los archivos. Se rechazan traversal, rutas Windows absolutas, enlaces, entradas no regulares y colisiones de nombres aplanados, también por mayúsculas. Límites por archivo JAR: 4096 entradas y 256 MiB expandidos de natives. No son los límites de modpacks DEC-10.
- Directorios `versions/<version>/natives-<GOOS>-<GOARCH>`: no se borra ni reutiliza la antigua carpeta `natives`. Se filtran classifiers de otras plataformas/arquitecturas también del classpath; se corrige el identificador Mojang `osx` para macOS y se respetan reglas `allow`/`disallow` en orden. Classifiers antiguos `${arch}` usan bitness x86; el formato antiguo en ARM se rechaza explícitamente, no se sustituye silenciosamente por x86.

**Evidencia:** `go test ./...`, `go test -race ./...`, `go vet ./...` y `git diff --check` pasan. Tests con `httptest` y directorios temporales comprueban reparación online y rechazo offline del índice alterado, reconstrucción offline de natives, archivo corrupto, CRC incorrecto, traversal, colisiones, enlaces de directorios y selección de classifiers. No se toca `.minecraft` real ni se afirma validación de ejecución Windows/macOS.

**Límites y siguiente corte:** A07 sigue abierta: metadata con timeout/contexto y errores estructurados, integridad de URLs sin hash (incluido Fabric), cancelación y revisión completa de reglas de arquitectura/versiones antiguas. Publicar natives es una secuencia de renombrados, **no** una transacción multiarchivo: un fallo intermedio bloquea Play, conserva el error y requiere reintentar. No hay aún exclusión entre procesos; comprobar enlaces no elimina carreras ante cambios externos. No se eliminan automáticamente natives sobrantes. Continuar estos bloqueantes antes de conectar instancias gestionadas a Play; el ciclo del proceso y retorno a la TUI siguen en A08.

### 10.5. Continuación de A07 — metadata, integridad y cancelación

`PrepareContext` propaga cancelación desde la TUI por metadata, cliente, bibliotecas, Fabric y assets. Manifest y metadata usan solicitudes acotadas; la versión se comprueba con el SHA-1 publicado antes de guardar JSON válido. Artefactos sin SHA-1 dejan de considerarse listos, incluidos los JARs de Fabric: se consulta su `.sha1` Maven y se guarda junto al perfil para verificar la caché offline. Si falta o es inválido, se informa el fallo sin caer a Vanilla. Esc cancela la preparación; natives comprueba cancelación entre archivos y antes de publicar cada archivo. Play siempre verifica antes de salir de la TUI, aunque los archivos parezcan presentes. SHA-1 aquí es el formato publicado por Mojang/Maven, **no** una garantía criptográfica contra un proveedor comprometido.

Pruebas locales: fallo de metadata, hash incorrecto, falta de hash, cancelación de una solicitud HTTP en curso y rechazo de Play tras cancelar. Pasan `go test ./...`, `go test -race ./...`, `go vet ./...`, `git diff --check` y compilación cruzada Windows amd64/macOS arm64; compilar no equivale a probar ejecución.

Validación manual reportada en §9: el usuario probó Vanilla 1.20.4, Vanilla 1.21.1 y Fabric 1.20.4 en Arch Linux sin fallas reportadas. Fecha/build/arquitectura/Java/llegada al menú/salida/log no quedaron confirmados; no inferirlos del entorno actual ni tratar el reporte como evidencia de otras plataformas.

**A07 continúa abierta:** faltan errores tipados por causa, revisión de reglas de arquitectura y versiones antiguas, interrupción durante la copia de un único archivo nativo y validación real de Fabric/Vanilla en las plataformas anunciadas. Un runtime con metadata antigua no verificable puede fallar explícitamente; no se lo convierte en éxito falso. A08 (resultado/proceso dentro de TUI) y conectar instancias gestionadas siguen pendientes.

### 10.6. Primer corte vertical A02/A03/A06 — selección, instancias y selector de Java

La TUI carga la selección persistida y permite abrir el selector con `i`, cambiar de instancia, renombrar con `r` y crear instancias Vanilla con `n`. Confirmar Play fija `gameDir`, `runtimeDir`, versión/loader/memoria y Java antes de iniciar preparación; el worker recibe el runtime explícito. `main.go` lanza usando ese mismo snapshot. Estado de caché/Java y mundos consultan runtime/gameDir seleccionados. La instalación heredada continúa usando `.minecraft`; Reset queda etiquetado/restringido a legacy hasta A10. No se escribe la configuración de juego managed en el `config.json` global; la memoria se cicla y persiste en `instance.json`.

El selector de Java adapta automáticamente la versión según el requisito oficial (`javaVersion.majorVersion`) de los metadatos de Minecraft o un ejecutable manual por instancia:
- En modo automático, busca una JRE en `PATH`, en `runtime/` empaquetado por Mojang o instalaciones del sistema que cumpla la versión requerida.
- El picker permite navegar y seleccionar manualmente un binario `java`. Si el requisito de la versión ya está en caché, rechaza binarios incompatibles al momento de elegir.
- Con `d` en el picker se restablece la selección al modo automático.
- Preflight unificado en la TUI: la comprobación de compatibilidad (`ResolvePreparedJava`) se ejecuta al terminar la preparación y antes de salir de la interfaz, evitando lanzar con un Java incompatible.
- En la vista activa, el estado de Java refleja si se usa el modo automático o el override manual y solo alerta incompatibilidad si el ejecutable manual seleccionado no alcanza la versión mayor requerida.

Las rutas managed rechazan `minecraft/` o `runtime/` si son symlinks para evitar romper aislamiento. Instancias Fabric gestionadas quedan bloqueadas al Play hasta honrar `loader_version` exacto y cachear el perfil por la identidad completa; no hay fallback a Vanilla.

`TestInstancePickerAndLaunchPathsStayIsolated`, `TestRenameInstanceFromPicker`, `TestManagedSettingsAndJavaPickerPersist`, `TestResolveBinaryRejectsIncompatibleOverrideAndFindsBundledJava`, `TestManagedFabricLaunchIsBlockedUntilExactLoaderSupport` y la prueba de rechazo de gameDir/runtime enlazados cubren selección persistida, runtime común, renombrado, persistencia y validación de Java, y aislamiento seguro. Pasan `go test ./...`, `go test -race ./...`, `go vet ./...` y `git diff --check`. No se ha ejecutado Minecraft con una instancia gestionada ni se marcan A02/A03/A06 cerradas formalmente hasta validación de juego real.

### 10.7. A10 parcial — raíz fijada para Reset/restore

Las operaciones de Reset exponen variantes con `gameDir` explícito y la TUI fija esa raíz al confirmar el worker, sin resolverla otra vez durante la mutación. Se conservan las entradas heredadas y su historial existente. Un test cambia la selección a B tras confirmar backup/restauración de A y verifica que B no cambia.

**Continuación:** Reset gestionado usa `datos-mctui/backups/<id>`; no ofrece selector de destino externo para evitar mezclar historiales de distintas instancias. El historial legacy y sus destinos externos previos permanecen intactos. La UI muestra la identidad y ruta capturadas al confirmar; tests con A, B y legacy prueban backup, rechazo de restaurar el backup de A en B y recuperación sin contaminación cruzada. Reset/restore y Play comparten lock entre procesos mientras mcTUI supervisa Java (límite de cierre forzado en §10.8). Pasan `go test ./...`, `go test -race ./...`, `go vet ./...` y `git diff --check`. **Validación manual reportada por el usuario:** Reset/restore de una instancia no afectó otra y la instalación legacy conservó acceso a sus datos e historial. Fecha, build y detalles de versión no fueron registrados. La cobertura de otros sistemas operativos queda diferida (§9); evaluar el cierre total de A10 según las plataformas que se declaren compatibles.

### 10.8. A11 parcial — exclusión entre procesos para Reset/restore

`RunAtWithBackups` y `RestoreAtRoot` mantienen un bloqueo no bloqueante del sistema operativo sobre un archivo estable del gameDir durante toda la mutación. No se elimina el archivo de bloqueo al liberar: un segundo proceso podría bloquear otro inode si se borrara. No depende de PID ni requiere recuperar locks obsoletos tras cerrar el proceso. Una TUI competidora recibe un error «instance busy» sin tocar `mods/` ni `config/`. Las confirmaciones de borrado y reemplazo siguen siendo explícitas. Tests prueban exclusión entre dos procesos y rechazo de purge/restore mientras se mantiene el bloqueo; compilación cruzada Windows/macOS, `go test ./...`, `go test -race ./...`, `go vet ./...` y `git diff --check` pasan.

**Validación manual reportada por el usuario:** una segunda TUI recibió el bloqueo al intentar Reset sobre la instancia con el juego activo, sin mutaciones. Fecha, build y detalles de versión no fueron registrados. **No declarar A11 cerrada:** una terminación forzada de mcTUI puede soltar el lock mientras Java continúa; operaciones externas a mcTUI siguen siendo no verificables.

### 10.9. A08 — ciclo de juego supervisado (DEC-03 resuelta)

**DEC-03, acordada:** mcTUI permanece abierta durante la partida. `q` y Ctrl+C no desacoplan ni matan Java mientras arranca o está activo; al salir el juego se muestra código/error y ruta del log, y Enter/Esc vuelve al menú. Cerrar por señal o fallo del proceso mcTUI no tiene esa garantía: no afirmar exclusión si Java sobrevive. No se ofrece «dejar ejecutando» hasta contar con un supervisor independiente.

La preparación entrega `Prepared` y la ruta de Java validada al Cmd que inicia el juego sin volver a descargar/verificar. El proceso conserva `cmd.Dir`, argumentos, log y el lock del gameDir hasta completar `Wait`; fallo de arranque o salida devuelve un mensaje a Bubble Tea sin abandonar la interfaz. Tests con scripts Java simulados y `httptest` verifican handoff, argumentos/directorios, log, bloqueo durante ejecución, salida normal y no cero. Validación manual reportada en §9: Vanilla 1.21.2 llegó al menú en dos instancias nuevas mediante `go run .`, con salida normal/código 0; la ruta del log se guardó bajo cada gameDir. Java automático y manual Java 27 fueron probados. La prueba de aislamiento confirmó que `test-A` creado en `prueba` no aparece en `test Fase 5` y sigue en la instancia original al volver. Quedan pendientes repetir `test Fase 5` con Java automático, Reset/restore y las demás plataformas; no cerrar A08 por este único corte.

### 10.10. Revisión del árbol local antes de validación manual

El corte sin commit de A08/A10/A11 pasa `go test ./...`, `go test -race ./...`, `go vet ./...`, `git diff --check` y `gofmt -l` sin pendientes. Compila para Windows amd64, macOS arm64 y Linux amd64; esto **no** valida ejecución ni justifica cerrar 5A. Validación manual reportada: dos instancias Vanilla 1.21.2 distintas llegaron al menú y salieron con código 0; un mundo `test-A` solo aparece en la instancia donde se creó, y persiste al volver a ella. Ver §9 para Java, `go run .` y logs; el test de Java automático queda en curso. Se mantiene sin commit para no mezclar ni asumir autoría de los cambios locales existentes. DEC-05 queda resuelta en §10.11; la política de canales para actualizaciones de mods sigue pendiente en C08.

### 10.11. DEC-05 — versión y loader fijos por instancia managed

**Decisión acordada:** elegir la versión de Minecraft al crear una instancia managed; una vez creada, su versión y loader no cambian. Java y memoria sí son ajustes mutables. Cambiar la versión más adelante requerirá un flujo explícito que evalúe compatibilidad de mods y paquetes de recursos; no se reutiliza silenciosamente el perfil actual. La instalación legacy conserva su selector de versión.

En el corte actual, la creación managed ofrece Vanilla. Fabric managed no debe ofrecerse hasta que Play pueda preparar y respetar la versión exacta de Fabric Loader; habilitarlo antes produciría una instancia que no se puede lanzar. La selección de canal para futuras actualizaciones de mods es una decisión separada de C08.

### 10.12. A04 parcial — copia selectiva desde legacy

El selector ofrece copiar desde la instalación heredada sin mover sus datos. El usuario elige nombre, versión Vanilla y cualquiera de `mods/`, `config/`, `saves/`, `resourcepacks/`, `shaderpacks/` y `options.txt`; la confirmación muestra origen, destino, contenido, tamaño estimado y advertencia sobre compatibilidad de mods/mundos. Runtime, logs e historial de Reset quedan fuera.

El motor rechaza enlaces y archivos especiales en el contenido seleccionado, adquiere el lock compartido sobre el gameDir heredado y copia con `os.Root`/`os.CopyFS`. Prepara metadata y archivos en `.mctui-copy-*` bajo la raíz de datos, fuera de `instances/`; solo publica la instancia mediante rename después de completar todo. Un error conserva el temporal y muestra su ruta; eliminarlo requiere confirmación explícita. El lock compartido crea/reutiliza `.mctui-operation.lock` en el origen, igual que Play/Reset; los datos seleccionados no se mueven ni modifican.

Tests cubren selección, copia independiente, rechazo de enlaces, no publicación ante preflight fallido, limpieza acotada del temporal y escritura de la letra `c` en el nombre. Validación manual reportada: se copió `saves/` a una instancia Vanilla 1.21.10; inicialmente el atajo `c` consumía la primera letra (`copia` quedaba como `opia`), se corrigió y el usuario confirmó manualmente que `c` ya funciona. **Validación manual reportada por el usuario:** legacy permaneció intacta y la copia resultó independiente; junto con la copia de `saves/` y la prueba de la tecla `c`, esto cierra la validación manual de A04 en el entorno probado. Fecha, build y detalles de versión no fueron registrados. La cobertura de otros sistemas operativos queda diferida (§9); launchers externos no coordinan con el lock de mcTUI.

### 10.13. A05 — duplicación selectiva de instancias managed

`c` sobre una instancia managed crea otra con ID nuevo y conserva versión Minecraft, loader exacto, Java y memoria del origen. El usuario elige el nombre y cuáles de `mods/`, `config/`, `saves/`, `resourcepacks/`, `shaderpacks/` y `options.txt` copiar: **los mundos (`saves/`) no se incluyen salvo selección explícita**. No se copian runtime, logs ni historial de Reset; los archivos son copias independientes. Se reutilizan lock, preflight, staging y confirmación de borrado parcial de A04. La duplicación no cambia la instancia origen y selecciona la nueva tras publicar. Tests de copia de metadata, omisión de mundos e independencia añadidos. El test puntual `TestCopyInstancePreservesSettingsAndCopiesOnlySelectedData` pasó tras corregir la preparación de su directorio temporal; la suite completa no se ejecutó. **Validación manual reportada por el usuario:** copia de `saves/` correcta y checklist de A05 aprobado: copia desde una instancia managed sin cambiar versión/loader, nueva identidad y selección, omisión de `saves/` cuando no se elige, ajustes heredados, historial de Reset no copiado e independencia de archivos en ambas direcciones. Fecha, build, OS/arquitectura y versión exacta de estas pruebas no registrados; otros sistemas operativos se validarán más adelante (§9). A05 queda cerrada en el entorno probado.

### 10.14. A12 — retirar y recuperar instancias

En el selector, `d` sobre una instancia managed ofrece soft delete o borrado permanente; legacy no puede retirarse. Soft delete mueve `instances/<id>/` a `retired/<id>/`, conserva los backups y, si era la activa, selecciona legacy. `t` muestra las retiradas y permite restaurarlas sin cambiar su ID o borrarlas definitivamente. El borrado permanente confirma explícitamente las rutas del directorio de la instancia y de `backups/<id>` antes de eliminar ambos. El traslado y la restauración comprueban colisiones y comparten exclusión; no se retira una instancia con juego activo bajo el lock de mcTUI. Los errores muestran el estado parcial sin afirmar éxito. Pruebas automatizadas de retiro bloqueado, recuperación, selección y borrado de backups pasan en Linux. **Validación manual confirmada por el usuario:** todas las operaciones de A12 funcionaron; fecha, build y detalles de cada intento no registrados. A12 queda cerrada en el entorno probado; ejecución en otros OS diferida. El cierre forzado de mcTUI con Java vivo sigue siendo límite de A11.

### 10.15. Integración coordinada de A06, A07 y A08/A11 (2026-09-26)

- **A07 (`8f32920`, `8da7bf9`):** Errores tipados con `PrepareError` clasificando la etapa (`metadata`, `client`, `library`, `assets`, `natives`, `Fabric`), cancelación durante la extracción nativa mediante `contextReader`, y soporte para `PrepareWithLoaderContext` preservando la compatibilidad de `PrepareContext`.
- **A06 (`784fc78`):** Soporte en `fabric.Resolve` de `loader_version` exacto con caché separada por `(Minecraft, loader)`, creación de instancias Fabric con prompt de versión de loader en la TUI, e integración con `launcher.PrepareWithLoaderContext`.
- **A08/A11 (`9058a72` y conexión en TUI):** Ciclo de vida del proceso supervisado mediante `launcher.Game`, con `Wait()` y `Kill()` expuestos. En la TUI, confirmación explícita con `k` (`¿Forzar cierre de Java? [y/N]`), `y` invoca `Kill()`, conservando la espera del comando `Wait()` hasta la salida de Java para mostrar código de salida, error y log.
- **Estado de cierre / bloqueos:**
  - **A06:** Cerrada en el entorno probado (DEC-05 resuelta).
  - **A07:** Cerrada en alcance de 5A (`8f32920`, `8da7bf9`); validación física en Windows/macOS diferida sin bloquear 5B.
  - **A08:** Cerrada en código y UI (`9058a72` y conexión de `Kill()` con confirmación explícita en TUI).
  - **A11:** Cerrada en alcance de 5A con exclusión mutua OS para Reset/Play y terminación desde TUI; límite de orfandad ante muerte externa de mcTUI documentado en §10.8 como techo de diseño conocido.
  - **Fase 5A:** Cerrada para el entorno probado (Linux x86_64) con compilación cruzada limpia para Windows amd64 y macOS arm64. La validación física en otros sistemas queda diferida sin bloquear el avance hacia 5B.

### 10.16. DEC-04 — mods locales (5B)

Decisión previa a implementación: activar/desactivar mediante renombrado `mod.jar` ↔ `mod.jar.disabled`, sin copiar contenido y sin sobrescribir destinos existentes. Prism reconoce el sufijo `disabled` en `launcher/minecraft/mod/Mod.cpp`; Fabric filtra exclusivamente nombres terminados en `.jar` en `DirectoryModCandidateFinder.isValidFile` (fuentes upstream consultadas durante este corte). Por tanto `.jar.disabled` no es candidato del loader Fabric; Vanilla no carga mods. No se extiende esta verificación a loaders todavía no soportados ni sustituye pruebas físicas por versión.

Registro versionado en `instances/<id>/mods.json` (incluido `instances/legacy/mods.json` para no guardar metadata en la instalación heredada). Archivos desconocidos se muestran como externos; modificar uno explícitamente registra procedencia local/external. Hash y mtime se revalidan bajo el lock común antes de mutar. No borrar config ni resolver/eliminar dependencias. Advertir siempre sobre mundos dependientes y launchers externos que no respetan el lock. Los errores de persistencia posteriores a una operación deben informar el estado real y permitir refrescar; no prometer transacciones multiarchivo ni exclusión frente a programas ajenos a mcTUI.

Fuentes: https://raw.githubusercontent.com/FabricMC/fabric-loader/master/src/main/java/net/fabricmc/loader/impl/discovery/DirectoryModCandidateFinder.java y https://raw.githubusercontent.com/PrismLauncher/PrismLauncher/develop/launcher/minecraft/mod/Mod.cpp .

### 10.17. DEC-06 — Recuperación y retención de actualizaciones de mods (5C)

**DEC-06, resuelta (2026-09-26):** Al actualizar un mod mediante Modrinth, el archivo `.jar` reemplazado no se destruye de inmediato ni se envía a la papelera del sistema operativo (lo cual en entornos headless/SSH o ciertos escritorios no garantiza persistencia predecible). En su lugar:
1. **Directorio de respaldo por instancia:** El archivo anterior se traslada a:
   `instances/<id>/backups/mods_replaced/<timestamp>_<archivo_anterior>.jar`
2. **Metadata de reversión:** Se registra la procedencia en `history.json` dentro de dicho directorio con nombre, `project_id`, versión previa, versión nueva, hash SHA-512 y fecha del reemplazo.
3. **Cuota de retención acotada:** Se conserva como máximo **1 versión anterior por mod** (o tope de 500 MiB por instancia). Versiones de respaldo previas para ese mismo mod se depuran automáticamente para no generar un consumo descontrolado de disco.
4. **Rollback desde la TUI:** Si el juego crashea con la nueva versión, el usuario puede ejecutar una acción de rollback que restaura el JAR anterior y actualiza `mods.json`.
5. **Detección de modificaciones manuales (C10):** Si un mod local o externo tiene hash alterado respecto de la metadata oficial, la TUI exige confirmación explícita diferenciada antes de sobreescribir.

### 10.18. DEC-10 — Red, límites de descarga, staging y seguridad de Modrinth (5C)

**DEC-10, resuelta (2026-09-26):** Definición de límites y políticas de red para la integración con la API de Modrinth y descarga de binarios:
1. **Restricción estricta de dominios:** Solo se permiten conexiones HTTPS a:
   - `https://api.modrinth.com/v2` (API REST)
   - `https://cdn.modrinth.net` (Descarga de binarios `.jar`)
   Cualquier redirección a otros dominios o protocolos inseguros (`http://`) debe abortar con error de seguridad inmediato.
2. **User-Agent canónico estructurado:** Todas las solicitudes HTTP deben identificarse con:
   `agmonetti/mcTUI/<version> (https://github.com/agmonetti/mcTUI)`
   para cumplir con el contrato de servicio de Modrinth y evitar bloqueos por IP.
3. **Manejo de rate limits (300 req/min):** Lectura e interpretación de `X-Ratelimit-Limit`, `X-Ratelimit-Remaining` y `X-Ratelimit-Reset`. Ante HTTP 429, se notifica en la TUI el tiempo de espera restante (`Retry-After`) y en tareas secundarias se aplica backoff exponencial acotado.
4. **Límites de tamaño y memoria:** Máximo **250 MiB** por archivo `.jar` descargado (controlado con `io.LimitReader`) y **10 MiB** para payloads JSON de respuesta de la API.
5. **Staging y Content-Addressing:** Las descargas se realizan en un archivo temporal (`.tmp`) en el mismo filesystem. El hashing SHA-512 se calcula en stream durante la descarga (`io.TeeReader`). Solo tras confirmar que el hash coincide exactamente con el publicado por Modrinth se renombra al destino final bajo el `operationlock` de la instancia. Si el hash no coincide o la descarga se corta, el temporal se purga inmediatamente.

### 10.19. Especificación y arquitectura técnica de 5C

Para evitar ambigüedades entre sesiones de trabajo, la arquitectura completa, modelos JSON, endpoints, algoritmo de resolución de dependencias, diseño de pantallas Bubble Tea y matriz de tests han sido documentados exhaustivamente en el archivo independiente:
👉 [PHASE_5C_SPECIFICATION.md](PHASE_5C_SPECIFICATION.md).

## 11. Trazabilidad y seguimiento

| Requisito original | Implementación rastreable |
| --- | --- |
| Separar perfiles | A02–A06 |
| Activar/desactivar mods | B01–B08 |
| Configuraciones por perfil | A02/A04/A05 |
| Instancias aisladas | 5A completo |
| Catálogo tipo Prism | C01–C07, E01–E02 |
| Dependencias/compatibilidad | C03–C06/C09/C12 |
| Actualizar mods | C08–C11 |
| Packs completos | D01–D12 |
| Reset e historial integrados | A10 y matriz de validación |
| No salir de la TUI para gestionar | A08 y sección 6 en todos los hitos |
| Otros loaders/cuentas | E03/E04, decisiones DEC-07/DEC-09 |

### Registro de entregas

Al cerrar tareas, sustituir «pendiente» por commit y evidencia. Documentar diferencias respecto del plan, sin marcar casillas por implementación parcial.

| Hito | Estado | Commit(s) | Evidencia / bloqueos |
| --- | --- | --- | --- |
| 5A | Cerrado (A01–A12 completadas; validación física en otros OS diferida sin bloqueo) | `8e3c89d`, `c834078`, `e3079af`, `7c12b8a`, `c57fbe2`, `ed03252`, `2a9f5e9`, `9058a72`, `8f32920`, `8da7bf9`, `784fc78` | Protocolo de validación manual en ejecución real completado y confirmado por el usuario en Linux x86_64 (§9). A06 cerrado con Fabric loader exacto (`784fc78`); A07 cerrado con errores tipados y cancelación (`8f32920`, `8da7bf9`); A08 cerrado con supervisión y cierre forzado en TUI (`9058a72`); A10/A11 cerrados con lock OS («instance busy») y límite de orfandad documentado; A12 cerrado con soft delete/restauración/purge. Compilación cruzada Windows amd64 y macOS arm64 verificada; validación física en dichos OS diferida sin bloquear 5B. |
| 5B | Cerrado | árbol local | B01–B08 implementadas y verificadas con tests unitarios y de integración TUI. Escaneo y registro en `mods.json`, renombrado reversible `.jar` ↔ `.jar.disabled` (DEC-04), lectura segura de ZIP/`fabric.mod.json`, adición, toggle y borrado con confirmación explícita bajo lock OS (`operationlock`). Detección de colisiones, cambios externos y duplicados. Tests automatizados, `test -race`, `go vet`, compilación cruzada Windows amd64 y Darwin arm64 y `git diff --check` pasando limpiamente. Sin red ni dependencias externas (5C no iniciada). |
| 5C | Implementación parcial, no cerrada | Árbol local sin commit de cierre; `PHASE_5C_SPECIFICATION.md` | Cliente, resolutor, instalación y TUI presentes; en este corte se añadió revalidación bajo lock de versiones de todo el plan elegido y conflictos registrados, además de mostrar y exigir consentimiento para reemplazos modificados en opcionales. La TUI ya muestra bytes acumulados durante la instalación y `Esc` cancela esperando la limpieza; tests de cancelación durante descarga y tras el primer archivo de un lote sin publicación. Una prueba integrada cubre la publicación de una rama opcional transitiva; la ruta de backups legacy se comprobó bajo `instances/legacy/`. En este corte también se cancelan y descartan consultas obsoletas de búsqueda, detalle, plan y updates, y el cliente compartido conserva el rate limit entre operaciones. En el corte actual, una prueba integrada publica dos updates y su dependencia compartida, corrige la reconciliación de `required_by` al dejar de requerirla un mod y evita atribuir dependencias a opcionales omitidos. Se rechazan ediciones externas durante la descarga incluso con consentimiento previo; el journal verifica el hash real del JAR local consentido y el rollback restaura sus bytes. Los conflictos de ramas opcionales se conservan sin bloquear ramas omitidas y se validan bajo lock solo al elegirlas (instalados, targets y pares opcionales); la revisión TUI los anuncia/bloquea y distingue descendientes requeridos. La recuperación ahora hace preflight de todos los archivos del lote antes de modificar cualquiera; pruebas comprueban que un backup corrupto o un JAR modificado bloquean sin recuperación parcial. Una prueba con proceso hijo interrumpido tras publicar el primer JAR de dos comprueba recuperación al reiniciar en Linux. Se omite la caché de Modrinth por riesgo de resultados obsoletos hasta que exista evidencia de necesidad. Por pedido del usuario se omiten pruebas Windows/macOS en este corte. Pasan `go test ./...`, `go test -race ./internal/mods ./internal/modrinth ./internal/ui`, `go vet ./...` y `git diff --check`; los siete fallos anteriores de UI ya no se reproducen. La evidencia y las brechas restantes están en §11 de la especificación. Brechas de C01–C12, integración y validación manual detalladas en §11 de la especificación; no marcar casillas aún. |
| 5D | Pendiente | — | Requiere 5A–5C; DEC-07/10 |
| 5E | Pendiente por bloque | — | Aprobaciones y decisiones DEC-07/08/09/11 |

### Definición de terminado del núcleo

- [ ] Desde la TUI se crean dos instancias, se configuran, se instalan/actualizan mods con dependencias y se juega sin contaminación cruzada.
- [ ] Mods locales, catálogo, importación/exportación/actualización de packs y recuperación están integrados, no son comandos externos obligatorios.
- [ ] Reset/restauración conservan las garantías de fases 1–4 y respetan identidad de instancia.
- [ ] Fallos/cancelaciones no producen éxito falso, lanzamiento imprevisto ni pérdida silenciosa de datos.
- [ ] Instalación heredada preservada y migración documentada.
- [ ] Pruebas automatizadas y manuales por plataforma registradas.
- [ ] README actualizado con alcance real, loaders, offline, formatos soportados y límites.
- [ ] La paridad ampliada pendiente se declara expresamente; no se etiqueta como paridad total con Prism.

## 12. Fuentes y límites de la investigación

Fuentes primarias consultadas en la investigación que originó este plan. Revalidar APIs, términos y formatos al implementar; no inferir garantías a partir de etiquetas de compatibilidad o documentación histórica.

- **S1:** Prism, crear instancias: https://prismlauncher.org/wiki/getting-started/create-instance/
- **S2:** Prism, mods locales y desactivación: https://prismlauncher.org/wiki/help-pages/loader-mods/
- **S3:** Prism, descargador de mods: https://prismlauncher.org/wiki/help-pages/mod-platform/
- **S4:** Prism, copia selectiva y opciones avanzadas: https://prismlauncher.org/wiki/help-pages/instance-copy/
- **S5:** Prism, packs: https://prismlauncher.org/wiki/getting-started/download-modpacks/ y https://prismlauncher.org/wiki/help-pages/zip-import/
- **S6:** Prism, Java: https://prismlauncher.org/wiki/help-pages/java-settings/ y https://prismlauncher.org/wiki/getting-started/installing-java/
- **S7:** Modrinth, búsqueda: https://docs.modrinth.com/api/operations/searchprojects/
- **S8:** Modrinth, versiones/dependencias: https://docs.modrinth.com/api/operations/getprojectversions/
- **S9:** Modrinth, IDs, User-Agent y límites: https://docs.modrinth.com/api/
- **S10:** CurseForge, solicitud de acceso: https://support.curseforge.com/support/solutions/articles/9000208346
- **S11:** CurseForge, términos: https://support.curseforge.com/support/solutions/articles/9000207405-curseforge-3rd-party-api-terms-and-conditions
- **S12:** CurseForge, API REST: https://docs.curseforge.com/rest-api/
- **S13:** Formato `.mrpack`: https://support.modrinth.com/en/articles/8802351-modrinth-modpack-format-mrpack
- **S14:** Fabric Meta: https://github.com/FabricMC/fabric-meta/blob/master/README.md
- **S15:** NeoForge, referencia para investigación adicional de instalación: https://github.com/neoforged/NeoForge y https://github.com/neoforged/InstallerTools
- **Referencia de implementación, no especificación de mcTUI:** https://github.com/PrismLauncher/PrismLauncher

No se verificó una implementación de fase 5, aprobación de CurseForge ni login oficial. La investigación distingue documentación externa, hallazgos del código base y propuestas. No incluye estimaciones de calendario: fijarlas después de resolver decisiones bloqueantes y ejecutar el primer corte vertical de 5A.
