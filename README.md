# mcTUI

![mcTUI screenshot](assets/main.png)

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
- `internal/i18n` — embedded English and Spanish UI translations
- `internal/nbt` / `internal/worlds` — reads level.dat to list saved worlds
- `internal/java` — detects compatible JRE installations
- `internal/mojang` — talks to Mojang's manifest and download servers
- `internal/fabric` — resolves Fabric Loader profiles
- `internal/launcher` — orchestrates the actual game launch
- `internal/modrinth` — Modrinth search, release metadata and verified downloads
- `internal/mods` — local mods and per-instance installation
- `internal/packs` — local resource packs and shader packs per instance
- `internal/ui` — the Bubble Tea model, update, and view

`main.go` loads config and runs the UI, which prepares and supervises the game process.

The dashboard's current setup banner shows the active instance name alongside its version, loader, RAM, and file status.

## Copy saved worlds between instances

In **Worlds**, select a saved world and press `c`. Choose another existing
instance and confirm the copy. Both managed instances and the existing
`.minecraft` installation can be sources or destinations. The source is kept;
if the destination already has a folder with the same name, the copy receives
a distinct folder name. The TUI warns when the world's last-saved Minecraft
version differs from the destination's version, but does not block the copy or
change either instance's version. Opening the copy in a different version or
without its original mods can damage it; back up important worlds first.

## Modrinth and instance content

The main menu opens **Modrinth**. Its submenu separates **Mods**, **Resource
Packs**, **Shaders**, and **Modpacks**. Phase 5D import has been exercised on
Arch Linux; update, export, and interrupted-update recovery still need separate
manual end-to-end validation (see [the implementation plan](PHASE_5_IMPLEMENTATION_PLAN.md)).

Content panels keep their size while browsing; their shortcuts appear inside
the panel, and the bottom bar shows installation status.

Resource packs and shader packs are listed per instance. Press `a` to open the
local picker, then `Enter` to import a ZIP or `Ctrl+A` to import the selected
unpacked folder. Press `s` (or `b`) to browse and search compatible Modrinth releases, or `d` to
delete a selected local pack after confirmation. Downloads are checked against the
published SHA-512 hash and stored in that instance's `resourcepacks/` or
`shaderpacks/` directory; existing filenames are never silently replaced.
Packs are **not automatically enabled**: select them inside Minecraft.
Shader packs additionally need a compatible shader engine, such as Iris or
OptiFine, installed separately. In the shader catalog, `Ctrl+E` switches
between releases declaring Iris and OptiFine compatibility.

Modrinth's Minecraft-version and shader-engine labels are author metadata,
not proof of correct rendering on a particular GPU. Local resource pack
`pack.mcmeta` formats are inspected, but files without Modrinth metadata
cannot be certified compatible with the instance's Minecraft version.

In **Modrinth → Modpacks**, use `l` for a local `.mrpack`, `h` for an
allowlisted HTTPS URL, or `s` to browse Modrinth modpacks without filtering by
the active instance. Select a modpack, then choose its Minecraft version and a
release (showing its loader). The selected archive declares the exact Minecraft
and loader versions; review them before installing. `Enter` creates a new
managed instance with that runtime by default, independently of the previously
selected instance. Press `Esc` from releases to choose another Minecraft version.
For an active instance with exactly matching Minecraft and loader versions and
no existing modpack, press `i` to
review every incoming path and install there instead. Existing files are kept
unless explicitly marked for replacement; worlds and logs cannot be overwritten
by this operation. If the instance already has a pack, use `u` to review an
update from another local `.mrpack`. Use `e` to export to a new `.mrpack`,
selecting authored text overrides explicitly. Exports reject unverifiable
downloads and do not bundle worlds, logs, credentials or arbitrary local JARs.
Use `r` to recover an interrupted install or update without launching the game.
The legacy installation keeps pack metadata under mcTUI's data root. These
operations are verified with isolated tests on Linux, not yet on a real Windows
system.

While a modpack version is staged or installed, the panel remains visible.
Installation reports verified pack-file progress; Minecraft runtime preparation
continues after pack files reach 100%. `Esc` requests cancellation and returns
to the previous modpack screen when the operation has stopped.

When starting an instance, mcTUI shows the current Minecraft preparation stage.
Where the number of files is known (client, libraries, assets), the bar shows
the percentage of **verified files in that stage**, not transferred bytes or
overall download progress. Fabric, native libraries and Java checks can have
an indeterminate bar. Press `Esc` to cancel preparation.

## Features

* **TUI Interface:** Smooth keyboard navigation using [Bubble Tea](https://github.com/charmbracelet/bubbletea).
* **Concurrent Downloads:** Maximize your bandwidth using *Goroutines* to download hundreds of assets and libraries in parallel.
* **Download Integrity:** Validates HTTP responses, requires published SHA-1 for prepared artifacts, and publishes completed files from temporary downloads. Some legacy releases without a published hash cannot be prepared.
* **Fabric Injection:** On-the-fly modloader integration. Parses Maven coordinates and seamlessly intercepts the vanilla boot process to load your `~/.minecraft/mods`.
* **Cross-Platform:** Native path resolution and execution for Linux, macOS, and Windows.
* **Secure LAN Multiplayer:** Generates dynamic v4 UUIDs in each session to avoid "duplicate name" conflicts on local servers.
* **XDG Persistence:** Saves your configuration (username, modloader state, and last played version) following Linux and Windows OS standards.
* **Legitimacy First:** Verifies existing official Minecraft installations for proprietary binaries (`client.jar`) before taking action, acting as a local utility rather than a direct distributor.
* **Smart Java Detection:** Automatically detects the Java version required
  by each Minecraft version and locates a compatible JRE on your system
  (PATH, common install directories, or IDE-managed JDKs), even if it's not
  your system default.
* **Language:** Switch between English and Spanish from the main menu; the choice is saved in your configuration.


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

To build the icon-bearing Windows executable from source (on Linux, macOS, or
Windows), regenerate the resource only when `assets/icon.ico` changes:

```bash
go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -ico assets/icon.ico -o rsrc_windows_amd64.syso
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o mctui-nuevo.exe .
```

On Windows, set `GOOS`, `GOARCH`, and `CGO_ENABLED` in your shell or omit them
when compiling on 64-bit Windows. Build the **package** (`.`), not `main.go`:
Go links `rsrc_windows_amd64.syso` from the package into the executable.
`assets/icon.svg` is the editable artwork; `assets/icon.ico` contains the
Windows icon sizes. The launcher remains a console application, so run
`.\mctui-nuevo.exe` from a terminal. The icon appears on the executable and
its shortcuts, not inside the terminal UI.

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
