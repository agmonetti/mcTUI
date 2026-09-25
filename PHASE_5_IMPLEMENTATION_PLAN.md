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
- [ ] Progreso con totales reales cuando se conocen, estado indeterminado cuando no.
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

Todas las casillas están pendientes. Completar un hito requiere código, comprobaciones y evidencia registrada, no solo pantallas navegables.

### 5A — Instancias, runtime y migración

Depende de: estado actual de fases 1–4.

- [x] **A01** Resolver decisiones de rutas/esquema y documentarlas en la sección 10 (diseño; implementación pendiente).
- [ ] **A02** Separar gameDir y runtime; revisar todos los callers de `MinecraftDir()` (lanzador separado; falta conectar preparación/TUI a la instancia activa).
- [ ] **A03** Persistir/listar/crear/seleccionar/renombrar instancias con ID estable (API y pruebas listas; falta exponerlo en la TUI).
- [ ] **A04** Instancia heredada y copia selectiva no destructiva.
- [ ] **A05** Duplicación independiente; política explícita de inclusión de mundos.
- [ ] **A06** Ajustes por instancia; Minecraft y loader fijados, Java y memoria validados.
- [ ] **A07** Preparación común con errores; corregir éxito falso y fallback silencioso (descargador ahora propaga fallos y evita publicar archivos incompletos; aún falta unificar preparación y validar assets/natives).
- [ ] **A08** Lanzamiento integrado con resultado/proceso/log en TUI; fijar `cmd.Dir`.
- [ ] **A09** Mundos y avisos de compatibilidad referidos a la instancia seleccionada.
- [ ] **A10** Reset/restore con raíz explícita e historial identificado por instancia.
- [ ] **A11** Controles de concurrencia y confirmación de operaciones destructivas.
- [ ] **A12** Retirar instancia: distinguir quitar referencia de borrar datos; confirmar claramente.

**Salida:** A y B mantienen mods, configuraciones y mundos independientes; jugar, resetear o restaurar A no cambia B. La instalación heredada y sus backups siguen accesibles. Fallar Fabric no inicia Vanilla.

### 5B — Gestor local de mods

Depende de: 5A.

- [ ] **B01** Escanear carpeta, registrar origen y mostrar archivos desconocidos.
- [ ] **B02** Leer metadata de JAR como ZIP sin ejecutarlo, inicialmente formatos soportados; limitar lecturas.
- [ ] **B03** Agregar `.jar` desde selector integrado; validar origen, destino y colisiones.
- [ ] **B04** Activar/desactivar de forma reversible; decidir convención de archivo/carpeta y verificar que el loader no lo cargue.
- [ ] **B05** Retirar con confirmación; no borrar configuraciones ni dependencias usadas por otros mods automáticamente.
- [ ] **B06** Señalar IDs duplicados cuando puedan identificarse, archivos corruptos y metadata desconocida.
- [ ] **B07** Detectar cambios externos al refrescar y antes de modificar.
- [ ] **B08** Mantener advertencia sobre mundos dependientes de mods eliminados/desactivados.

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

### Plataformas

- [ ] Windows real: prioridad principal; espacios, Unicode, archivos bloqueados, rutas, permisos normales y antivirus.
- [ ] Linux real: permisos, enlaces y lanzamiento de instancia aislada.
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
| DEC-03 | Comportamiento de TUI al cerrar con juego activo | Informar si se deja ejecutando; no matar implícitamente | A08 |
| DEC-04 | Cómo desactivar archivos | Convención reversible comprobada con loaders soportados | B04 |
| DEC-05 | Política de versión/canal | Loader fijado; releases por defecto; cambios visibles | A06/C08 |
| DEC-06 | Recuperación/retención de actualizaciones | Proponer respaldo del conjunto reemplazado y explicitar consumo de disco | C11/D12 |
| DEC-07 | Loaders/versions que se anuncian soportados | Vanilla/Fabric primero; ampliar con pruebas reales | D06/E03 |
| DEC-08 | Alcance contractual CurseForge | No implementación de acceso hasta tener vía autorizada | E01/E02 |
| DEC-09 | Autenticación oficial en esta entrega o siguiente | Mantener bloque separado; nunca confundir mods con autenticación | E04 |
| DEC-10 | Límites de descargas/ZIP y política de hosts | Límites explícitos y verificables por tests | C07/D02 |
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

**No cerrar A07 aún:** UI y lanzador siguen duplicando preparación; falta verificar integridad del índice en caché y extracción de natives, URLs sin hash, timeout/contexto de metadata, cancelación y retorno del proceso dentro de TUI. La instalación gestionada sigue sin estar conectada a Play. No se ha validado en Windows/macOS reales.

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
| 5A | En curso (A01; A02/A03/A07 parciales) | — | §10.1–10.3; TUI aún heredada, DEC-03/05 pendientes |
| 5B | Pendiente | — | Requiere 5A; DEC-04 |
| 5C | Pendiente | — | Requiere 5A/5B; DEC-06/10 |
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
