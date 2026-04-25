# Ollama Parity Status

## Ollama Commands (13 visible)

| Command | Gospike has it? | Notes |
|---|---|---|
| `serve` / `start` | ✓ | Ported |
| `create` | ✗ | Create from Modelfile |
| `show` | ✓ | Ported |
| `run` | ✓ | Ported (non-interactive only) |
| `stop` | ✓ | Ported |
| `pull` | ✓ | Ported |
| `push` | ✗ | Push to registry |
| `signin` / `signout` | ✗ | Auth to ollama.com |
| `list` / `ls` | ✓ | Ported |
| `ps` | ✓ | Ported |
| `cp` | ✗ | Copy a model |
| `rm` | ✓ | Ported |
| `launch` | ✓ | Ported |
| `close` | ✓ | Oramalama-specific (not in ollama) |

## Missing Commands

1. **`cp`** - Copy a model (user requested this)
2. **`create`** - Create from a Modelfile
3. **`push`** - Push to registry
4. **`signin` / `signout`** - Auth to ollama.com

For the ramalama/opencode use case, `cp` is the most immediately useful. The others (`create`, `push`, `signin`) are lower priority since ramalama already handles auth differently via its own backend.
