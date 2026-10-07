# iq Architecture

> **Project status: reopened (2026-10-06) as the local-model substrate for pi.**
> Frozen on 2026-03-20 as a learning artifact; reopened with one purpose: run a local MLX model that [pi](https://pi.dev/) can use. This document describes the single-model controller and records what was removed.

## Purpose

iq runs one offline MLX model on Apple Silicon for pi. It answers three questions pi cannot: which model this machine can hold, how to get it, and how to keep an OpenAI-compatible server running for it. Everything else that an agent needs — the loop, tools, sessions, compaction, RAG, web search — lives in pi and its packages.

## System Diagram

```
┌──────────────────────────────┐        ┌──────────────────────────────┐
│          iq (Go)             │        │           pi (TS)            │
│ doc  pick  start  stop       │        │ harness, tools, sessions,    │
│ restart  status  pi  config  │        │ extensions, packages         │
└──────┬───────────────────────┘        └──────────────┬───────────────┘
       │ writes provider entry                         │ reads
       ▼                                               ▼
┌──────────────────────────────┐        ┌──────────────────────────────┐
│ ~/.config/iq/config.yaml     │        │ ~/.pi/agent/models.json      │
│ ~/.config/iq/run/*.json      │        │ providers.iq → :27001/v1     │
└──────┬───────────────────────┘        └──────────────┬───────────────┘
       │ spawns                                        │ POST /v1/chat/completions
       ▼                                               ▼
┌──────────────────────────────────────────────────────────────────────┐
│ python -m mlx_lm server --model <snapshot> --port 27001              │
│ OpenAI-compatible: tools, array content, stream_options, tool_calls  │
└──────────────────────────────┬───────────────────────────────────────┘
                               │ loads
                               ▼
                  ~/.cache/huggingface/hub/models--<org>--<name>/snapshots/<hash>/
                  (downloaded by `hf download` on first `iq start`)
```

## Package Structure

| Package | Domain |
|---------|--------|
| `internal/config` | Schema-3 `config.yaml`: one model, `context_window`, `max_tokens`, `chat_template_args`; in-memory migration from schema 2 |
| `internal/model` | Embedded catalog, memory probe, fit rule, `Pick`, Hugging Face cache resolution, `hf download` |
| `internal/sidecar` | Sidecar lifecycle: port allocation, `mlx_lm.server` launch, readiness probe, state and log files, orphan cleanup, VLM guard, venv discovery |
| `internal/usage` | Shared help-page renderer |
| `internal/color` | Terminal colour helpers |

`cmd/iq` wires the commands: `doc`, `pick`, `start`, `stop`, `restart`, `status`, `pi`, `config`, `version`.

## Current Platform

- Apple Silicon Mac (M1 or later)
- Go 1.26+ for the single `iq` binary
- Python 3 with `mlx-lm` 0.30.7 or newer (via pipx); its `mlx_lm.server` is the inference sidecar
- `hf` CLI (via pipx) for downloads
- Hugging Face Hub as the model registry

## Core Files

- `AGENTS.md`: governance contract
- `arch.md`: this document — system structure and durable decisions
- `plan.md`: direction and ideas to explore

## Major Components

### Model Pick

`iq pick` reads unified memory with `sysctl -n hw.memsize` and walks the embedded catalog (`internal/model/catalog.yaml`). Each entry carries `id`, `disk_gb`, `context_window`, optional `chat_template_args`, `verified`, and `notes`.

Fit rule: budget = 70% of memory; estimate = `disk_gb × 1.15 + 2 GB` for weights plus key-value cache. The pick is the largest **verified** entry whose estimate fits. Unverified entries are never picked; they become verified only after the catalog verification run (structured tool calls under `mlx_lm.server`, no end-of-sequence leak). On a 24 GiB machine the budget is about 18 GB, so a 5.7 GB model fits and a 16.4 GB mixture-of-experts model does not.

`iq pick -w` writes the entry's id, `context_window`, and `chat_template_args` to `config.yaml`. `iq start` without a configured model runs the same pick and saves it.

### Service Daemon

`iq start` / `iq stop` / `iq restart` manage one detached `mlx_lm.server` process — mlx-lm's own OpenAI-compatible server, launched as `python -m mlx_lm server` from the `mlx-lm` pipx venv. The server accepts `tools`, array-form message content, and `stream_options`, and returns structured `tool_calls`, which is what pi needs. It prints a harmless "not recommended for production" warning to the sidecar log on every start.

Start sequence:
1. Resolve the model: argument, else `config.yaml`, else pick and save
2. Resolve the Hugging Face snapshot directory; run `hf download <id>` when it is missing
3. **VLM guard** — reject vision-language models by inspecting `config.json`; `mlx_lm.server` cannot serve them
4. Allocate the next free port from 27001+
5. Spawn the detached subprocess (`Setsid: true`) with `--chat-template-args` when configured
6. Poll `GET /v1/models` until 200 OK or a 120-second timeout; a goroutine on `cmd.Wait()` detects early crashes
7. On failure, print the last ten log lines and the log path

`iq stop` with no argument also sweeps orphaned servers: anything matching `infer_server.py`, `mlx_lm.server`, `mlx_lm server`, or `embed_server.py` on an iq-managed port. The first two legacy patterns clean up processes from the three-binary release.

**pi integration (`iq pi`)** — prints the provider entry pi needs for the one running sidecar, or merges it into `~/.pi/agent/models.json` with `-w` (`-f PATH` targets another file). The entry is provider `iq` with `baseUrl http://127.0.0.1:<port>/v1`, `api openai-completions`, a dummy `apiKey`, and one model whose id is `default_model` — the only id `mlx_lm.server` accepts for its loaded model — plus `contextWindow` and `maxTokens` when `config.yaml` sets them for the running model. `iq pi` refuses to run while more than one inference sidecar is live. Select the model in pi with `pi --model iq/default_model`.

`iq status` shows MODEL / ENDPOINT / PID / UPTIME / RUNNING / MEM for every live sidecar plus the configured model when it is stopped, then iq's own memory and the total.

`iq doc` checks Apple Silicon, `python3`, `mlx_lm.server` and its `--model` and `--chat-template-args` flags, `hf`, and whether the configured model's snapshot is in the cache.

### Configuration

`~/.config/iq/config.yaml`, schema version 3:

```yaml
version: 3
model: mlx-community/Qwen3.5-4B-OptiQ-4bit
context_window: 32768
max_tokens: 4096
chat_template_args: {enable_thinking: false}
```

`chat_template_args` is a map passed verbatim as JSON to `mlx_lm.server --chat-template-args`. Qwen3.5 needs `{enable_thinking: false}` for plain tool use. `context_window` and `max_tokens` are exported to pi; iq itself sends no inference requests.

**Schema versioning** — the version is peeked first. Version 3 loads directly. Version 2 (the flat model pool) is migrated in memory: the first pool model, its `context_window`, `max_tokens`, and `chat_template_args` are kept, everything else is dropped, and a notice names every dropped key. `iq start` and `iq pick -w` persist the migrated file; `iq config show` and `iq config validate` never write. Versions below 2 and above 3 are errors with recovery guidance.

## Storage Layout

```
~/.config/iq/
├── config.yaml                  # schema-3 config (see above)
└── run/
    ├── <model-slug>.json        # sidecar state: tier, model, pid, port, started
    └── <model-slug>.log         # sidecar stdout/stderr

~/.cache/huggingface/hub/        # model snapshots, written by hf download
~/.pi/agent/models.json          # pi's provider config; iq pi -w merges the `iq` provider
```

## Source Files

| File | Purpose |
|------|---------|
| `cmd/iq/main.go` | CLI entry point, root command, version, help routing |
| `cmd/iq/svc.go` | `start`, `stop`, `restart`, `status`, `doc`; model resolution and download |
| `cmd/iq/pick.go` | `iq pick`: memory probe, catalog pick, config write |
| `cmd/iq/pi.go` | `iq pi`: pi provider entry (print, or merge into `~/.pi/agent/models.json` with `-w`) |
| `cmd/iq/cfg.go` | `iq config`: show and validate |
| `internal/config/config.go` | Schema-3 config, load/save, version-2 migration |
| `internal/model/model.go` | Catalog, fit rule, `Pick`, cache helpers, `hf download` |
| `internal/model/catalog.yaml` | Curated model list (embedded) |
| `internal/sidecar/sidecar.go` | Sidecar state, lifecycle via `mlx_lm.server` (`InferArgs`), port allocation, orphan cleanup, venv discovery, VLM guard |
| `internal/usage/usage.go` | Shared help-page renderer |
| `internal/color/color.go` | Terminal colour helpers |

## History

iq began in 2026 as a three-binary research vehicle: `iq` routed prompts through embedding-based cue classification, tool detection, knowledge-base retrieval, a response cache, and a hand-rolled tool-call loop against a custom Python inference server; `lm` downloaded, listed, and benchmarked models; `kb` kept a private knowledge base. The project froze on 2026-03-20.

It reopened on 2026-10-06 after pi proved to be the harness iq had been reaching for. Pi owns the loop, tools, sessions, and extensions, and runs sub-1k-token system prompts that suit small local models. The `hf` CLI covers downloads and cache management, `mlx_lm.benchmark` covers benchmarks, and mlx-lm's own `mlx_lm.server` handles array content, tool schemas, and tool-call parsing that the custom server lacked. The multi-model pool was retired because one good model behind pi beats several small ones behind a router on current hardware. About 12,400 lines of Go became about 2,000.
