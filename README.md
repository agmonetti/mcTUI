# mcTUI

An extremely lightweight, fast, and pure terminal (TUI) Minecraft (Java Edition) launcher. Written in Go, designed for minimalist environments and users who prefer the console.

---

## Quick Install (Linux/macOS)
If you have Go installed, you can compile and install mcTUI in a single command:

```bash
git clone [https://github.com/agmonetti/mcTUI.git](https://github.com/agmonetti/mcTUI.git) /tmp/mctui && cd /tmp/mctui && go mod tidy && go build -ldflags="-s -w" -o ~/.local/bin/mctui main.go && rm -rf /tmp/mctui

```

*(Ensure `~/.local/bin` is in your `$PATH`)*

---

## Project Structure

mcTUI is organized into focused internal packages, each with a single
responsibility:

- `internal/config` — settings persistence and OS-specific paths
- `internal/roadmap` — fetches the "Future Changes" panel content
- `internal/nbt` / `internal/worlds` — reads level.dat to list saved worlds
- `internal/java` — detects compatible JRE installations
- `internal/mojang` — talks to Mojang's manifest and download servers
- `internal/fabric` — resolves Fabric Loader profiles
- `internal/launcher` — orchestrates the actual game launch
- `internal/ui` — the Bubble Tea model, update, and view

`main.go` itself is just wiring: ~25 lines that load config, build the
UI, and hand off to the launcher.

## Features

* **TUI Interface:** Smooth keyboard navigation using [Bubble Tea](https://github.com/charmbracelet/bubbletea).
* **Concurrent Downloads:** Maximize your bandwidth using *Goroutines* to download hundreds of assets and libraries in parallel.
* **Smart Validation:** Checks local file size and existence (`os.Stat`) to guarantee near-instant startups in subsequent sessions.
* **Fabric Injection:** On-the-fly modloader integration. Parses Maven coordinates and seamlessly intercepts the vanilla boot process to load your `~/.minecraft/mods`.
* **Cross-Platform:** Native path resolution and execution for Linux, macOS, and Windows.
* **Secure LAN Multiplayer:** Generates dynamic v4 UUIDs in each session to avoid "duplicate name" conflicts on local servers.
* **XDG Persistence:** Saves your configuration (username, modloader state, and last played version) following Linux and Windows OS standards.
* **Legitimacy First:** Verifies existing official Minecraft installations for proprietary binaries (`client.jar`) before taking action, acting as a local utility rather than a direct distributor.
* **Smart Java Detection:** Automatically detects the Java version required
  by each Minecraft version and locates a compatible JRE on your system
  (PATH, common install directories, or IDE-managed JDKs), even if it's not
  your system default.
* **Live Roadmap:** The "Future Changes" panel fetches its content from this
  repository, so you always see the latest plans without updating mcTUI.


## Requirements

* **Operating System:** Linux (Tested on Arch Linux), macOS, or Windows.
* **Java Runtime Environment:**
* `jre17-openjdk` (For Minecraft 1.17 to 1.20.4)
* `jre21-openjdk` (For Minecraft 1.20.5+)



---

## Windows Installation & Usage

For Windows users who do not wish to compile from source, pre-built binaries are provided.

1. Download the latest `mctui-windows-amd64.exe` from the [Releases](https://www.google.com/search?q=https://github.com/agmonetti/mcTUI/releases) page.
2. Ensure you have the correct Java version installed (e.g., Eclipse Temurin). **Crucial:** You must check the option to add Java to your system's `PATH` variable during installation.
3. Open a **PowerShell** window in the directory where you downloaded the executable.
4. Run the launcher using the following command:

```powershell
.\mctui-windows-amd64.exe

```

---

## Manual Installation & Compilation (Linux/macOS)

1. Clone the repository:

```bash
git clone [https://github.com/agmonetti/mcTUI.git](https://github.com/agmonetti/mcTUI.git)
cd mcTUI

```

2. Download Go dependencies:

```bash
go mod tidy

```

3. Compile the optimized binary:

```bash
go build -ldflags="-s -w" -o mctui main.go

```

4. Install on your system:

```bash
mkdir -p ~/.local/bin
mv mctui ~/.local/bin/

```

## Usage

Simply run the binary from any terminal emulator:

```bash
mctui

```

> **Note:** Use `↑/↓` arrows to navigate, `Enter` to select/toggle, and `Esc/q` to quit or go back.
> **Note:** Usernames must be 1-16 characters, using only letters, numbers, and underscores — this matches Minecraft's own requirements.

## Disclaimer

This project is strictly an educational tool demonstrating concurrency in Go, REST API consumption, TUI design, and OS subprocess management.

**mcTUI is designed to operate as a local utility over existing, officially acquired Minecraft installations.** It does not act as a primary distributor for proprietary Mojang binaries (such as `client.jar`).

The networking capabilities implemented rely entirely on the official, documented Minecraft protocol for Local Area Networks (LAN) and development environments. It connects exclusively to local servers explicitly configured with `online-mode=false`. **This project does not circumvent DRM, nor does it encourage or facilitate software piracy.** To play on standard, authenticated public servers, you must own a legitimate copy of the game via [minecraft.net](https://www.minecraft.net/).

*Not affiliated with Mojang AB or Microsoft.*
