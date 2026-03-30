# oramalama

Automated lifecycle management for [RamaLama](https://github.com/containers/ramalama) and [OpenCode](https://opencode.ai).

## Features

- **Automatic Lifecycle:** Starts RamaLama when you run `opencode` and stops it when the last instance is closed.
- **Strix Halo Optimized:** Monitors the GTT (Unified Memory) pool on AMD Strix Halo systems.
- **Interactive Memory Management:** Uses `gum` to provide a TUI for killing high-memory processes if the unified pool is low.
- **Multi-instance Support:** Intelligent process tracking ensures RamaLama stays alive until all OpenCode windows are closed.

## Installation

1. Ensure `ramalama`, `opencode`, and `gum` are installed.
2. Copy `oramalama` to your `PATH` (e.g., `~/.local/bin/`).
3. Alias `opencode` to the script in your shell config:
   ```bash
   alias opencode='~/.local/bin/oramalama'
   ```

## Requirements

- Linux
- AMD GPU (specifically tested on Strix Halo)
- `ramalama`
- `opencode-ai`
- `gum`
