# oramalama

**oramalama** brings the CLI nicities of [ollama](https://ollama.com) to [RamaLama](https://github.com/containers/ramalama) — hardware-aware model management, a full interactive REPL, coding-tool integrations, and TUI polish, while using ramalama + llama.cpp as the inference backend.

```
oramalama [--remote <url>] [--model <name>] [--dry-run] <subcommand>
```

---

## Features

- **ollama-style subcommands** — `run`, `serve`, `launch`, `list`, `pull`, `ps`, `stop`, `show`, `rm`, `search`
- **Interactive REPL** — streaming chat with slash commands (`/set system`, `/save`, `/load`, `/clear`, …)
- **Thinking mode** — `--think` surfaces model reasoning; `--hidethinking` strips `<think>` blocks
- **tok/s stats** — `--verbose` prints timing after every response
- **Hardware auto-detection** — Strix Halo (AMD Ryzen AI Max) detected automatically; ramalama picks the right image for NVIDIA / standard AMD / CPU on all other hardware
- **Remote inference** — point at any ramalama server over Tailscale / LAN with `--remote`
- **llmfit integration** — `oramalama search` recommends models that fit your GPU
- **Coding tool launchers** — `oramalama launch` wires up [opencode](https://opencode.ai) or [Goose CLI](https://block.github.io/goose/)
- **Systemd quadlet** — default model runs as a persistent user service with auto-restart
- **Tab completion** — bash and zsh completions included

---

## Install

```bash
bash install.sh                     # installs to ~/.local/bin
bash install.sh --prefix /usr/local # system-wide (may need sudo)
bash install.sh --uninstall         # remove
```

### Requirements

| Dep | Required | Install |
|---|---|---|
| `ramalama` | ✅ | https://github.com/containers/ramalama |
| `gum` | ✅ | https://github.com/charmbracelet/gum |
| `jq` | ✅ | https://jqlang.github.io/jq/download/ |
| `curl` | ✅ | system package manager |
| `llmfit` | optional | https://github.com/jmorganca/llmfit |
| `opencode` | optional | https://opencode.ai |
| `goose` | optional | https://block.github.io/goose/ |

---

## Subcommands

### `run [model]` — interactive chat REPL

```bash
oramalama run
oramalama run hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M
oramalama run --think --verbose
oramalama run --hidethinking
```

Starts the inference server (if not already running) and opens a streaming REPL.

**Flags:**

| Flag | Effect |
|---|---|
| `--think` | Show model reasoning/thinking content |
| `--hidethinking` | Suppress `<think>…</think>` blocks |
| `--verbose` | Print tok/s, prompt tokens, generated tokens after each reply |

**Slash commands inside the REPL:**

| Command | Effect |
|---|---|
| `/help` | Show all slash commands |
| `/set system <text>` | Set a system prompt (empty to clear) |
| `/set parameter <key> <value>` | Set a generation param (`temperature`, `top_p`, …) |
| `/clear` | Wipe conversation history |
| `/save <name>` | Save session to `~/.config/oramalama/sessions/<name>` |
| `/load <name>` | Restore a saved session |
| `/show` | Print current model, endpoint, system prompt, params |
| `/bye` | Exit |

---

### `serve [model]` — start inference server

```bash
oramalama serve
oramalama serve hf://unsloth/Qwen3-Coder-Next-GGUF:Q4_K_M
```

Starts the server detached. Context window and serve flags are auto-tuned per model size and hardware.

---

### `launch` — start server + coding tool

```bash
oramalama launch
oramalama launch --tool opencode
oramalama launch --tool goose
```

Starts the server, configures the chosen coding tool to use it, then launches the tool. Cleans up on exit (unless managed by systemd).

---

### `list` / `ls` — show downloaded models

```bash
oramalama list
```

Renders a table: name, size, context window, last modified. Default model marked with ★.

---

### `pull [model]` — pull a model

```bash
oramalama pull hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M
oramalama pull          # opens llmfit recommender if no model given
```

---

### `ps` — show running models

```bash
oramalama ps
```

---

### `stop [container]` — stop the server

```bash
oramalama stop          # stops systemd quadlet or prompts to pick a container
oramalama stop my-container
```

---

### `show [model]` — model metadata

```bash
oramalama show
oramalama show hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M
```

Displays architecture, quantization, size, context window. Shows endpoint if currently running.

---

### `rm [model]` — remove a model

```bash
oramalama rm hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M
oramalama rm            # interactive picker
```

---

### `search` — find models for your hardware

```bash
oramalama search
```

Runs `llmfit` to recommend coding models that fit your GPU pool. Offers to pull the selection via ramalama.

---

## Global flags

| Flag | Effect |
|---|---|
| `--remote <url>` | Use a remote ramalama server (e.g. `http://myserver:8080`) |
| `--model <name>` | Pre-select a model (skips interactive picker) |
| `--dry-run` | Print actions without executing them |

---

## Config file

`~/.config/oramalama/config` (bash `key=value` syntax):

```bash
# Use a remote ramalama server by default (e.g. on your home server)
RAMALAMA_ENDPOINT=http://karnataka:8080

# Override the default model
DEFAULT_MODEL=hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M

# Pre-select a coding tool for 'oramalama launch'
DEFAULT_TOOL=opencode
```

CLI flags always override config file values.

---

## Hardware auto-detection

On **Strix Halo** (AMD Ryzen AI Max / RDNA 4 with >60 GB unified VRAM):
- Uses `docker.io/kyuz0/amd-strix-halo-toolboxes:vulkan-radv` container image
- Sets `--parallel 1 --cache-ram 0` to prevent OOM on the ~31 GB system RAM partition

On all **other hardware** (NVIDIA, standard AMD dGPU, CPU):
- Leaves image selection to ramalama (CUDA / ROCm / CPU auto-detect)

---

## Systemd quadlet (persistent server)

For the default model, oramalama uses a systemd user service (`ramalama-opencode.service`) backed by the included quadlet file. This survives logout with `loginctl enable-linger`.

```bash
# Install the quadlet
cp ramalama-opencode.container ~/.config/containers/systemd/
systemctl --user daemon-reload
systemctl --user start ramalama-opencode

# Enable at boot
loginctl enable-linger "$USER"
```

---

## opencode-rl alias

`opencode-rl` is a convenience wrapper that runs:

```bash
oramalama launch --tool opencode "$@"
```

---

## Examples

```bash
# Quick chat with default model
oramalama run

# Chat showing Gemma 4's reasoning, plus timing stats
oramalama run --think --verbose

# Chat hiding the thinking content (clean output only)
oramalama run --hidethinking

# Start opencode pointed at local model
oramalama launch --tool opencode

# Use a remote ramalama server
oramalama --remote http://karnataka:8080 launch --tool opencode

# See what's running
oramalama ps

# Show metadata for the default model
oramalama show

# Pull a model recommended for your GPU
oramalama search
```

