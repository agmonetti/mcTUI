# mcTUI

![mcTUI screenshot](assets/main.png)

<p align="center">
  <b>An extremely lightweight, fast, and pure terminal (TUI) Minecraft: Java Edition launcher.</b><br>
  Written in Go, designed for minimalist environments, power users, and console lovers.
</p>

<p align="center">
  <a href="https://agmonetti.github.io/mcTuiDocs/">📖 <b>Read Official Documentation</b></a> •
  <a href="https://github.com/agmonetti/mcTUI/releases/latest">📦 <b>Download Release v1.5</b></a>
</p>

---

> ⚠️ **MANDATORY PREREQUISITE / REQUISITO OBLIGATORIO:**  
> **mcTUI ONLY works if you have purchased and downloaded Minecraft: Java Edition on your system.**  
> mcTUI is strictly a terminal frontend and local management utility that operates on top of an existing, official `.minecraft` installation. It **does not** provide, distribute, or pirate the game, nor does it bypass Mojang or Microsoft DRM/authentication. **Without an official, legitimately purchased copy of Minecraft already downloaded and installed on your computer, mcTUI will NOT function.**

---

## ⚡ Highlights

* **Pure TUI Interface:** Smooth keyboard navigation powered by [Bubble Tea](https://github.com/charmbracelet/bubbletea).
* **Isolated Instance Management:** Create, duplicate, and manage independent Minecraft instances with dedicated versions, Fabric loaders, Java runtimes, and RAM allocation.
* **Modrinth Hub Integration:** Search, filter, and install mods, resource packs, and shader packs with automated dependency resolution and channel selection.
* **Modpack Support (.mrpack):** Full lifecycle management for Modrinth modpacks (import, update, author export, and safe recovery).
* **World Management:** Atomic copying of saved worlds across instances and the legacy `.minecraft` folder with version compatibility warnings.
* **Resets Configs:** One-click restoration of conflicted mod setups with optional timestamped backups and zero world/shader data loss.
* **Parallel Downloads:** High-throughput concurrent asset and library downloads with verified SHA integrity.
* **Smart Java Detection:** Automatically detects required Java versions and locates compatible JREs on your system.
* **Multi-Language (i18n):** Native English and Spanish UI translations saved in your global configuration.
* **Cross-Platform:** Native support for Linux, macOS, and Windows (with embedded executable icon).

---

## 🚀 Quick Install

### Linux & macOS
If you have Go installed, compile and install mcTUI in a single command:
```bash
git clone https://github.com/agmonetti/mcTUI.git /tmp/mctui && cd /tmp/mctui && go mod tidy && go build -ldflags="-s -w" -o ~/.local/bin/mctui main.go && rm -rf /tmp/mctui
```
*(Ensure `~/.local/bin` is in your `$PATH`)*

### Windows
1. Download `mctui-windows-amd64.exe` from the [Releases](https://github.com/agmonetti/mcTUI/releases/latest) page.
2. Ensure you have a compatible Java JRE (17 or 21) installed and added to your system `PATH`.
3. Open PowerShell and run:
```powershell
.\mctui-windows-amd64.exe
```

---

## 🎮 Basic Controls

| Key | Action |
| --- | --- |
| `↑` / `↓` | Navigate lists and options |
| `Enter` | Select / Toggle / Confirm |
| `Esc` / `q` | Go back / Quit |
| `i` | Quick access to Instances |
| `m` | Quick access to Modrinth Hub |

---

## 📚 Full Documentation

For detailed architecture diagrams, configuration options, advanced modpack handling, and developer guides, visit our documentation site:

👉 **[https://agmonetti.github.io/mcTuiDocs/](https://agmonetti.github.io/mcTuiDocs/)**

* [Getting Started & Installation](https://agmonetti.github.io/mcTuiDocs/)
* [Feature Breakdown & Instances](https://agmonetti.github.io/mcTuiDocs/features)
* [Internal Architecture & Design](https://agmonetti.github.io/mcTuiDocs/architecture)
* [Configuration & Paths](https://agmonetti.github.io/mcTuiDocs/configuration)
* [Development & Source Compilation](https://agmonetti.github.io/mcTuiDocs/development)
* [Roadmap & Project History](https://agmonetti.github.io/mcTuiDocs/roadmap)

---

## ⚖️ Disclaimer & Legitimacy Policy

**Strict Requirement:** mcTUI **ONLY** works if you have purchased and downloaded an official copy of Minecraft: Java Edition on your machine.

This project is strictly an educational tool demonstrating concurrency in Go, REST API consumption, TUI design, and OS subprocess management.

**mcTUI is designed to operate solely as a local utility over existing, officially acquired, and downloaded Minecraft installations.** It does not act as a distributor for proprietary Mojang binaries (such as `client.jar`) and does not facilitate any unauthorized access.

The networking capabilities implemented rely entirely on the official, documented Minecraft protocol for Local Area Networks (LAN) and development environments. It connects exclusively to local servers explicitly configured with `online-mode=false`. **This project does not circumvent DRM, nor does it encourage or facilitate software piracy.** To play Minecraft, you must own a legitimate copy of the game purchased via [minecraft.net](https://www.minecraft.net/).

*Not affiliated with, endorsed by, or associated with Mojang AB, Microsoft, or any of their affiliates.*
