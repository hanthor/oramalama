# oramalama

**oramalama** brings the CLI nicities of [ollama](https://ollama.com) to [RamaLama](https://github.com/containers/ramalama) — hardware-aware model management, a full interactive REPL, coding-tool integrations, and TUI polish, while using ramalama + llama.cpp as the inference backend.

```
oramalama [--model <name>] [--dry-run] [--version] <subcommand>
```

---

## Features

- **ollama-style subcommands** — `run`, `serve`, `launch`, `list`, `pull`, `ps`, `stop`, `show`, `rm`, `search`, `close`
- **Always-on router daemon** — `oramalama-router` starts a small Qwen3-4B dispatcher that routes requests to the right large backend, with idle-timeout to reclaim VRAM
- **Interactive REPL** — streaming chat with OpenAI-compatible API
- **tok/s stats** — `--verbose` prints timing after every response
- **Hardware auto-detection** — Strix Halo (AMD Ryzen AI Max) detected automatically; ramalama picks the right image for NVIDIA / standard AMD / CPU on all other hardware
- **Remote inference** — point at any ramalama server over Tailscale / LAN
- **llmfit integration** — `oramalama search` recommends models that fit your GPU; the model picker shows a full hardware-fit panel
- **OpenAI / Ollama / Anthropic compatible API server** — `oramalama serve` starts a full proxy server that translates between OpenAI, Ollama, and Anthropic protocols
- **Full tool ecosystem** — `oramalama launch` supports 6+ tools:
  - *Coding:* OpenCode, Pi, Goose CLI, VS Code (Continue/Cline)
  - *Terminal:* aichat, tgpt
  - Always-on router daemon
- **Systemd quadlet** — default model runs as a persistent user service with auto-restart
- **Tab completion** — bash and zsh completions included

---

## Install

### Homebrew (macOS / Linux)

```bash
brew install hanthor/tap/oramalama
```

### Direct download

Download the latest binary for your OS/architecture from [GitHub Releases](https://github.com/hanthor/oramalama/releases).

```bash
# Example: Linux x86_64
curl -fsSL https://github.com/hanthor/oramalama/releases/latest/download/oramalama_linux_amd64.tar.gz | tar xz
sudo mv oramalama /usr/local/bin
```

### From source (Go)

```bash
git clone https://github.com/hanthor/oramalama.git
cd oramalama
go build -o oramalama ./cmd/oramalama-go
go build -o oramalama-router ./cmd/oramalama-router  # optional
sudo mv oramalama* /usr/local/bin
```

### Legacy: Bash script

The original bash implementation lives in `legacy/oramalama.bash`. It's still available for reference but is no longer the primary distribution.

```bash
bash install.sh           # installs to ~/.local/bin
bash install.sh --legacy  # force bash install
```

### Requirements

| Dep | Required | Install |
|---|---|---|
| `ramalama` | ✅ | https://github.com/containers/ramalama |
| `curl` | ✅ | system package manager |
| `llmfit` | optional | https://github.com/jmorganca/llmfit |
| `opencode` | optional | https://opencode.ai |
| `pi` | optional | https://pi.dev |
| `goose` | optional | https://block.github.io/goose/ |

> **Note:** The legacy bash version also requires `gum` and `jq`. The Go binary does not need them.

---

## Subcommands

### `run [model] [prompt]` — chat completion

```bash
oramalama run                    # prompts for model + prompt interactively
oramalama run hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M "hello"
oramalama run --verbose
```

Starts the inference server (if not already running) and sends a chat completion.

**Flags:**

| Flag | Effect |
|---|---|
| `--verbose` | Print tok/s, prompt tokens, generated tokens after each reply |
| `--format json` | Return raw JSON response instead of formatted text |

---

### `launch` — start server + launch a tool

```bash
oramalama launch                        # two-level interactive menu
oramalama launch --tool opencode        # jump straight to a tool
oramalama launch --tool goose
oramalama launch --router               # use always-on router daemon
```

Starts the inference server (if not already running), then presents a **two-level menu**:

```
What would you like to launch?
  Coding Tools
  Start Server Only
  Search Models
```

#### Coding Tools

| Tool | `--tool` | Notes |
|---|---|---|
| OpenCode | `opencode` | Configured automatically via `opencode.json` |
| Pi | `pi` | Configured automatically via `~/.pi/agent/models.json` |
| Goose CLI | `goose` | Launched with `OPENAI_HOST` / `OPENAI_API_KEY` env vars |
| VS Code | `vscode` | Opens `code .`; install [Continue](https://continue.dev) or [Cline](https://github.com/cline/cline) extension |

#### Router daemon

The `--router` flag (or "OpenCode [router daemon]" in the TUI menu) starts the always-on router daemon (`oramalama-router`). This runs a small Qwen3-4B model that dispatches requests to the right large backend, with idle-timeout to reclaim VRAM.

---

### `serve [model]` — start inference server

```bash
oramalama serve
oramalama serve hf://unsloth/Qwen3-Coder-Next-GGUF:Q4_K_M
```

Starts the server detached. Context window and serve flags are auto-tuned per model size and hardware.

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
```

---

### `ps` — show running models

```bash
oramalama ps
```

---

### `stop [container]` — stop the server

```bash
oramalama stop          # stops systemd quadlet or ramalama container
```

---

### `show [model]` — model metadata

```bash
oramalama show
oramalama show hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M
```

Displays architecture, quantization, size, context window, license, path. Shows endpoint if currently running.

If `llmfit` is installed, also shows a hardware-fit panel: memory required, estimated tok/s, fit level (🟢 Perfect / 🟡 Tight / 🔴 Too large), score breakdown, and GGUF sources.

---

### `rm [model]` — remove a model

```bash
oramalama rm hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M
```

---

### `search` — find models for your hardware

```bash
oramalama search
```

Runs `llmfit` to recommend coding models that fit your GPU pool. Offers to pull the selection via ramalama.

---

### `close [model]` — unload a model

```bash
oramalama close hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M
```

Sends a keep_alive=0 request to unload the model from GPU memory without stopping the server.

---

## API Server

`oramalama serve` (the Go binary's built-in server, not to be confused with `oramalama launch` which starts ramalama) starts an HTTP API server on port **8090** that proxies requests to your ramalama backend. It provides:

- **Ollama API**: `/api/generate`, `/api/chat`, `/api/tags`, etc.
- **OpenAI API**: `/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`, `/v1/models`
- **Anthropic API**: `/v1/messages`

This is useful when you want a standalone API gateway without using the CLI subcommands.

```bash
oramalama serve
# listening on 0.0.0.0:8090
```

---

## Router Daemon

`oramalama-router` is a separate binary for the always-on request dispatcher. It:

1. Starts a small Qwen3-4B model via ramalama (if not running)
2. Listens on port **8083** for OpenAI-compatible requests
3. For each `/v1/chat/completions` request, calls the small model with a tool-use prompt to decide which large backend to use
4. Starts the chosen backend on demand (cold start ~15–45 s)
5. Proxies the request and response transparently
6. Stops idle backends after a configurable timeout to reclaim VRAM

```bash
oramalama-router [--port 8083] [--idle-timeout 10m]
```

Configure OpenCode to use `http://127.0.0.1:8083` as its ramalama provider — the daemon handles all model-switching automatically.

---

## Global flags

| Flag | Effect |
|---|---|
| `--model <name>` | Pre-select a model (skips interactive picker) |
| `--dry-run` | Print actions without executing them |
| `--version` | Print version and exit |

---

## Config file

`~/.config/oramalama/config`:

```bash
# Override the default model
DEFAULT_MODEL=hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M

# Remote endpoint (use a remote ramalama server)
RAMALAMA_ENDPOINT=http://karnataka:8080

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

For the default model, oramalama uses a systemd user service backed by the included quadlet file. This survives logout with `loginctl enable-linger`.

```bash
cp oramalama.container ~/.config/containers/systemd/
systemctl --user daemon-reload
systemctl --user start oramalama

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

# Chat with a specific model
oramalama run hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M "write a haiku about Go"

# Start opencode pointed at local model
oramalama launch --tool opencode

# Start pi coding agent
oramalama launch --tool pi

# Use the router daemon (small dispatcher model)
oramalama launch --router

# Use a remote ramalama server
oramalama launch --tool opencode --remote http://karnataka:8080

# Use the API proxy server
oramalama serve

# Start the router daemon standalone
oramalama-router

# See what's running
oramalama ps

# Show metadata + llmfit hardware-fit panel
oramalama show

# Pull a model recommended for your GPU
oramalama search

# Unload a model from GPU memory
oramalama close hf://unsloth/gemma-4-31B-it-GGUF:Q4_K_M
```

---

## Development

Built with Go 1.26+. The project has two binaries:

| Binary | Source | Purpose |
|---|---|---|
| `oramalama` | `cmd/oramalama-go/` | Main CLI — subcommands, tool launching, API server |
| `oramalama-router` | `cmd/oramalama-router/` | Always-on request dispatcher daemon |

```bash
# Build both
go build -o oramalama ./cmd/oramalama-go
go build -o oramalama-router ./cmd/oramalama-router

# Cross-compile
GOOS=linux GOARCH=amd64 go build -o oramalama-linux-amd64 ./cmd/oramalama-go
GOOS=darwin GOARCH=arm64 go build -o oramalama-darwin-arm64 ./cmd/oramalama-go
```

### Project structure

```
cmd/
  oramalama-go/        # Main CLI entry point
  oramalama-router/    # Router daemon
internal/
  cli/                 # CLI subcommand implementations
  config/              # Config file loading and hardware detection
  runtime/             # Ramalama server management
  server/              # Ollama/OpenAI/Anthropic API proxy server
  tui/                 # Interactive TUI components
  api/                 # API types and client
  client/              # HTTP client for ramalama backend
  container/           # Container manager
  model/               # Model manager
  progress/            # Progress bars and spinners
  readline/            # Readline library for REPL
  gospike/             # Initial Go spike (replaced by cli/ package)
legacy/
  oramalama.bash       # Original bash implementation (archived)
completions/           # Shell completion files
docs/                  # Design docs
```
