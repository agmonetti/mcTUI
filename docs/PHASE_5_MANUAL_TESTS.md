# Fase 5 — Pruebas manuales pendientes

Archivo local ignorado por Git. Hacer estas pruebas cuando haya tiempo, en **Arch y Windows**, con instancias de prueba y sin tocar mundos importantes. Anotar en cada caso: sistema, versión de mcTUI, versión de Minecraft/loader, mod o pack usado y qué ocurrió. Un error concreto sirve más que marcar «falló».

**Ya confirmado en Arch:** instancias (5A), mods locales (5B), Modrinth (5C) e instalación de modpacks (5D). No hace falta repetirlo solo para cerrar una casilla. **5D sigue abierta:** faltan los recorridos de actualización, exportación y recuperación. En Windows aún no hay pruebas reales registradas.

## Recorridos

- [ ] **Windows — instancias y juego:** crear dos instancias, jugar en ambas y comprobar que mods y mundos de una no aparecen en la otra.
- [ ] **Windows — mods:** instalar varios mods desde Modrinth en una instancia y probar uno que no tenga versión compatible con su Minecraft/loader. Comprobar que la TUI instala una versión *del mod* compatible si existe; si no existe, que lo explique y no instale algo incompatible. **No se espera que cambie automáticamente Minecraft ni el loader de la instancia.** Probar también agregar, desactivar y reactivar un `.jar` local.
- [ ] **Windows — modpack:** importar un `.mrpack`, abrir la instancia y comprobar que se puede jugar. Si el pack pide un loader no soportado, comprobar que se rechaza con un mensaje claro, sin cambiarlo por otro.
- [ ] **Arch y Windows — actualizar pack:** importar una versión de prueba, modificar un archivo de `config/` y actualizar desde **Modrinth → Modpacks → Actualizar** usando otra versión `.mrpack` local. Comprobar que se muestra el conflicto y que la decisión tomada conserva o reemplaza ese archivo según se eligió.
- [ ] **Arch y Windows — exportar pack:** exportar la instancia desde **Modrinth → Modpacks → Exportar**, importar el archivo resultante como instancia nueva y comprobar que contiene los mods/configuración esperados, pero no mundos ni datos personales. Si algún archivo no se puede exportar, comprobar que la TUI lo dice en lugar de omitirlo.
- [ ] **Arch y Windows — recuperar:** en una instancia de prueba, si una actualización falla o queda interrumpida, usar **Modrinth → Modpacks → Recuperar** y comprobar que la instancia vuelve a un estado utilizable sin perder cambios propios. No forzar cierres sobre datos importantes; si no se presenta una falla segura de reproducir, dejar esta casilla pendiente.
- [ ] **Windows — casos cotidianos:** repetir una importación o actualización con una ruta que tenga espacios/acentos y con un archivo del pack abierto. Si falla, anotar el mensaje y si la instancia sigue funcionando; no borrar archivos de recuperación.

## Resultado

| Sistema | Prueba | Versión de mcTUI / Minecraft / loader | Mod o pack | Qué pasó | Estado |
| --- | --- | --- | --- | --- | --- |
| Arch | Actualizar / exportar / recuperar pack | — | — | Aún no probado | Pendiente |
| Windows | Instancias / mods / modpacks y casos cotidianos | — | — | Aún no probado | Pendiente |

Registrar cada intento en una fila nueva. Si algo falla, corregirlo y repetir **solo ese recorrido**. Después de los recorridos, revisar por separado los criterios D01–D07 de implementación en `PHASE_5_IMPLEMENTATION_PLAN.md`; las pruebas manuales no reemplazan esa auditoría. No declarar 5D cerrada ni Windows validado hasta tener resultados.
