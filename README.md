# mcTUI

An extremely lightweight, fast, and pure terminal (TUI) Minecraft (Java Edition) launcher. Written in Go, designed for minimalist environments and users who prefer the console.

---

## Features

- **TUI Interface:** Smooth keyboard navigation using [Bubble Tea](https://github.com/charmbracelet/bubbletea).
- **Concurrent Downloads:** Maximize your bandwidth using *Goroutines* to download hundreds of assets and libraries in parallel.
- **Smart Validation:** Checks local file size and existence (`os.Stat`) to guarantee near-instant startups in subsequent sessions.
- **Secure LAN Multiplayer:** Generates dynamic v4 UUIDs in each session to avoid "duplicate name" conflicts on local servers.
- **XDG Persistence:** Saves your configuration (username and last played version) following Linux standards in `~/.config/mctui/config.json`.

## Requirements

- Operating System: Linux (Tested on Arch Linux) / macOS.
- Java Runtime Environment:
  - `jre17-openjdk` (For Minecraft 1.17 to 1.20.4)
  - `jre21-openjdk` (For Minecraft 1.20.5+)

## Installation and Compilation

1. Clone the repository:
   ```bash
   git clone https://github.com/agmonetti/mcTUI.git
   cd mcTUI
   ```

2. Download Go dependencies:
   ```bash
   go mod tidy
   ```

3. Compile the optimized binary (without debug info for smaller size):
   ```bash
   go build -ldflags="-s -w" -o mctui main.go
   ```

4. Install on your system:
   ```bash
   mkdir -p ~/.local/bin
   mv mctui ~/.local/bin/
   ```
   *(Make sure `~/.local/bin` is in your `$PATH` environment variable)*

## Usage

Simply run the binary from any terminal emulator:

```bash
mctui
```

> **Note:** Use `↑/↓` arrows to navigate, `Enter` to select, and `Esc/q` to quit or go back.

## Upcoming Features

- [ ] Implement dynamic parsing of `mainClass` and `minecraftArguments` to support Legacy versions (<= 1.12).
- [ ] Support for modloader injection (Fabric/Forge).
- [ ] Automatic download and mapping of specific JREs per version.

## Disclaimer

This project is an educational tool about concurrency in Go, consuming REST APIs, and subprocess execution. It works exclusively in natively designed Offline/LAN mode. **It does not encourage or facilitate piracy**. To play on public servers with `online-mode=true`, you must purchase the game officially at [minecraft.net](https://www.minecraft.net/).
