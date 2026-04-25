# oramalama Go rewrite design: Ollama-style parity on RamaLama

## Goal

Turn the Go rewrite into the long-term `oramalama` implementation, with as much Ollama-style UX and API compatibility as practical, while keeping the existing Bash script as the short-term production fallback.

The target is **Ollama-like behavior on top of RamaLama/llama.cpp**, tuned for this Strix Halo machine and the current local model setup.

## Current state

### Working today

- Bash `oramalama` is still the broadest implementation.
- The Go spike already supports:
  - `list`
  - `ps`
  - `show`
  - `pull`
  - `rm`
  - `stop`
  - `run` (one-shot prompt)
  - `serve`
  - `launch --tool goose --prompt ...`
- The Go spike correctly detects ad-hoc mapped ports instead of assuming `8080`.
- Goose was successfully run against **`hf://batiai/Qwen3.6-35B-A3B-GGUF:Q6_K`** through the Go path.

### Important machine-specific facts

- Host is Strix Halo with a large unified AMD VRAM pool.
- Default always-on model is now **Qwen 3.6 35B-A3B Q6_K**.
- Current default model:
  - **`hf://batiai/Qwen3.6-35B-A3B-GGUF:Q6_K`**
- Only one large local model should be active at a time.

## Product direction

### What to copy from Ollama

1. **Command surface**
   - `run`
   - `serve`
   - `pull`
   - `rm`
   - `ps`
   - `show`
   - `stop`
   - `cp`
   - `create`
   - `launch`

2. **API shape**
   - Native-style management endpoints and flows for model lifecycle
   - OpenAI-compatible endpoints as the main integration surface
   - Stable model listing/details behavior for tools like Goose/OpenCode

3. **Interactive UX**
   - Ollama-style launcher menu
   - model picker
   - direct tool launch flows
   - noninteractive command behavior that is script-safe

4. **Operational behavior**
   - clear readiness checks
   - strong progress/output during pulls and startup
   - reliable single-model enforcement

### What to keep unique to oramalama

- Strix Halo hardware heuristics
- RamaLama container/service integration
- quadlet-aware default model handling
- llmfit-guided model fit and recommendations
- remote discovery / SSH model switching

## Non-goals for the next tranche

- Replacing RamaLama with a custom inference daemon
- Copying Ollama cloud auth/sign-in flows first
- Deep blob/registry plumbing before local UX is solid
- Rebuilding every Bash feature before the Go UX becomes coherent

## Reference files

### Ollama

- `.tmp/ollama/cmd/cmd.go`
- `.tmp/ollama/cmd/launch/launch.go`
- `.tmp/ollama/cmd/tui/tui.go`
- `.tmp/ollama/server/routes.go`
- `.tmp/ollama/api/client.go`
- `.tmp/ollama/api/types.go`

### oramalama

- `internal/gospike/app.go`
- `cmd/oramalama-go/main.go`
- `oramalama`
- `README.md`

## Architecture

### 1. CLI layer

Keep a single Go CLI entrypoint and continue growing subcommands around a shared `App`.

Near-term structure:

- `cmd/oramalama-go/main.go`
- `internal/gospike/app.go`

Likely next refactor:

- `internal/cli/`
- `internal/runtime/`
- `internal/models/`
- `internal/integrations/`
- `internal/ui/`

The current `app.go` is still acceptable for the next slice, but API handling and launcher logic should move out before the file grows much further.

### 2. Runtime/model orchestration

Keep RamaLama as the local runtime backend.

Responsibilities:

- inspect installed models
- choose context size heuristically
- enforce one active local model
- prefer quadlet for the default model
- use ad-hoc detached RamaLama containers for alternate models
- detect active endpoint dynamically from Podman port mapping

### 3. API compatibility layer

Short term:

- continue consuming the OpenAI-compatible surface exposed by the running server
- standardize helpers around:
  - `/v1/models`
  - `/v1/chat/completions`

Next step:

- introduce a small Go client wrapper inside oramalama so subcommands stop issuing ad-hoc raw HTTP calls
- keep response shapes aligned with Ollama/OpenAI expectations where that improves tool compatibility

### 4. Integration launch layer

Treat integrations as first-class launch targets.

Immediate targets:

- `server`
- `goose`
- `opencode`

Expected behavior:

- ensure requested model is running
- export OpenAI-compatible env vars
- launch the target tool
- keep noninteractive prompt mode working for smoke tests

### 5. Interactive UI layer

Borrow Ollama's launcher/TUI concepts, but do not clone blindly.

Required adaptations:

- expose local model fit hints
- prefer locally available models
- surface Strix Halo-safe recommendations
- preserve `oramalama` concepts like “default always-on model” vs “ad-hoc alternate model”

## Implementation plan

### Phase 1: command-surface parity

Status: **mostly done for the first slice**

Done:

- `ps`
- `show`
- `pull`
- `rm`
- `stop`
- one-shot `run`
- `serve`
- `launch --tool goose --prompt`

Remaining in this area:

- `cp`
- stronger `run` flag parity
- cleaner `launch` ergonomics
- optional JSON/table output consistency

### Phase 2: API/client shaping

Status: **next**

Deliverables:

- introduce shared HTTP helpers/client for:
  - model listing
  - model detail lookup
  - chat completions
  - readiness checks
- reduce direct subprocess dependence where HTTP is already the better abstraction
- make OpenCode/Goose-facing behavior more predictable

Why this matters:

- it makes the Go CLI feel more like Ollama
- it simplifies integration launchers
- it keeps the CLI from duplicating parsing logic in every command

### Phase 3: launcher + picker TUI

Status: **after API/client cleanup**

Deliverables:

- root launcher menu
- “chat with model” path
- “launch integration” path
- model picker with local metadata
- hooks for llmfit fit hints

This is the part that will make the rewrite feel visibly closer to Ollama.

### Phase 4: model lifecycle expansion

Deliverables:

- `cp`
- `create`
- better `show`
- progress-rich pull behavior
- optional Modelfile-oriented flows

## Risks and design constraints

### RamaLama is not Ollama

The UX can be copied, but the runtime behavior is still constrained by RamaLama and llama.cpp. Some Ollama features will need translation rather than direct porting.

### Mixed service modes

There are two local serving modes:

- persistent quadlet-backed default service
- detached ad-hoc RamaLama containers for alternate models

The Go code must preserve both without confusing the active endpoint.

### Context sizing is safety-critical

Wrong context sizing can make startup fail or degrade the desktop. Unknown model names must default conservatively.

### Tool integrations depend on stable model IDs

Goose/OpenCode flows only stay smooth if:

- the endpoint is correct
- the model ID is readable from `/v1/models`
- the selected model is the one actually serving

## Recommended immediate next tasks

1. Create a small shared client/helper layer for `/v1/models` and `/v1/chat/completions`.
2. Add `opencode` as a first-class Go launch target.
3. Start a Bubble Tea launcher modeled on Ollama's `launch` + `tui` packages.
4. Add `cp` and clean up `show`/`run` output modes.

## OpenCode continuation notes

If continuing with OpenCode on Qwen 3.6, use:

- model: **`hf://batiai/Qwen3.6-35B-A3B-GGUF:Q6_K`**

Relevant current state:

- Qwen 3.6 is present locally in the RamaLama store
- Goose against Qwen 3.6 was already proven through the Go path
- Qwen 3.6 is now the normal default service on `8080`

Good continuation prompt for OpenCode:

> Continue the Go rewrite toward Ollama parity using `docs/oramalama-ollama-parity-design.md`. Start with the API/client shaping tranche, then add `opencode` as a Go launch target, preserving Strix Halo heuristics and single-model enforcement.

## Definition of success

The rewrite is on the right track when:

- everyday usage no longer requires the Bash script
- OpenCode and Goose can both switch to Qwen 3.6 cleanly
- the launcher/picker feels Ollama-like
- local model switching is safe on this machine
- Bash remains only as a fallback, not the main path
