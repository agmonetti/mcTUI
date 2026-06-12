# mcTUI

Un launcher de Minecraft (Java Edition) extremadamente ligero, rápido y puramente de terminal (TUI). Escrito en Go, diseñado para entornos minimalistas y usuarios que prefieren la consola.

---

## Características

- **Interfaz TUI:** Navegación por teclado fluida gracias a [Bubble Tea](https://github.com/charmbracelet/bubbletea).
- **Descargas Concurrentes:** Exprime tu ancho de banda utilizando *Goroutines* para descargar cientos de assets y librerías en paralelo.
- **Validación Inteligente:** Comprueba el peso y existencia de los archivos locales (`os.Stat`) para garantizar arranques casi instantáneos en sesiones posteriores.
- **Multijugador LAN Seguro:** Generación de UUIDs v4 dinámicos en cada sesión para evitar conflictos de "nombre duplicado" en servidores locales.
- **Persistencia XDG:** Guarda tus configuraciones (usuario y última versión) respetando el estándar de Linux en `~/.config/mctui/config.json`.

## Requisitos

- Sistema Operativo: Linux (Testeado en Arch Linux) / macOS.
- Entorno de ejecución de Java:
  - `jre17-openjdk` (Para Minecraft 1.17 a 1.20.4)
  - `jre21-openjdk` (Para Minecraft 1.20.5+)

## Instalación y Compilación

1. Clona el repositorio:
   ```bash
   git clone https://github.com/agmonetti/mcTUI.git
   cd mcTUI
   ```

2. Descarga las dependencias de Go:
   ```bash
   go mod tidy
   ```

3. Compila el binario optimizado (sin info de debug para menor tamaño):
   ```bash
   go build -ldflags="-s -w" -o mctui main.go
   ```

4. Instala en tu sistema:
   ```bash
   mkdir -p ~/.local/bin
   mv mctui ~/.local/bin/
   ```
   *(Asegúrate de que `~/.local/bin` esté en tu variable de entorno `$PATH`)*

## Uso

Simplemente ejecuta el binario desde cualquier emulador de terminal:

```bash
mctui
```

> **Nota:** Usa las flechas `↑/↓` para navegar, `Enter` para seleccionar y `Esc/q` para salir o volver atrás.

## Próximas versiones

- [ ] Implementar parseo dinámico de `mainClass` y `minecraftArguments` para soportar versiones Legacy (<= 1.12).
- [ ] Soporte para inyección de modloaders (Fabric/Forge).
- [ ] Descarga automática y mapeo de JREs específicos por versión.

## Disclaimer

Este proyecto es una herramienta educativa sobre concurrencia en Go, consumo de APIs REST y ejecución de subprocesos. Funciona exclusivamente en modo Offline/LAN de diseño nativo. **No fomenta ni facilita la piratería**. Para jugar en servidores públicos con `online-mode=true`, debes adquirir el juego oficialmente en [minecraft.net](https://www.minecraft.net/).
