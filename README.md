# iq

> **Project status: reopened (2026-10-06) as the local-model substrate for pi.**
> iq was frozen on 2026-03-20 as a learning artifact in LLM orchestration, embedding pipelines, and local inference on Apple Silicon. It was reopened with one purpose: run a local MLX model that [pi](https://pi.dev/) can use. Day-to-day AI-assisted development uses [Claude Code](https://claude.com/claude-code) with hosted models. The former `lm` and `kb` binaries, the prompt pipeline, and the knowledge base were removed; see [arch.md](arch.md) for what they did and why they went.

iq is a small command-line controller for **one offline MLX model on Apple Silicon**. It picks the largest verified model that fits the machine's memory, downloads it from Hugging Face when needed, runs it as a managed `mlx_lm.server` sidecar, and hands the endpoint to pi. The model runs entirely **on-device**; pi provides the coding-agent harness, tools, sessions, and extensions.

## Why

Pi is the harness. What it cannot do is decide which model a given Mac can hold, download it, and keep an OpenAI-compatible server running for it. iq does exactly that and nothing else.

## Requirements

- Apple Silicon Mac (M1 or later)
- Go (for building)
- Python 3 with `mlx-lm` 0.30.7 or newer (`pipx install mlx-lm`); its `mlx_lm.server` runs the sidecar
- `hf` CLI (`pipx install huggingface_hub`) for model downloads
- [pi](https://pi.dev/) to use the model

For gated models and to avoid rate limits, set a Hugging Face token:

```bash
export HF_TOKEN=hf_...   # replace with your token
```

## Getting Started

Requires Go installed with `$GOPATH/bin` in your `$PATH`.

```bash
git clone https://github.com/kquo/iq
cd iq
./build.sh
```

Builds and installs one binary, `iq`, into `$GOPATH/bin`.

## Quick Start

```bash
iq doc                           # check python3, mlx_lm.server and its flags, hf, and the model
iq pick -w                       # choose the largest verified catalog model that fits this machine
iq start                         # download if needed, then run it as an mlx_lm.server sidecar
iq pi -w                         # write the `iq` provider into ~/.pi/agent/models.json
pi --model iq/default_model      # use it from pi
iq stop                          # stop the sidecar
```

`iq start` with no configured model runs the pick for you and saves it. `iq start <model>` runs a specific Hugging Face model id instead.

## Configuration

`~/.config/iq/config.yaml` holds one model and the limits pi should know about:

```yaml
version: 3
model: mlx-community/Qwen3.5-4B-OptiQ-4bit
context_window: 32768
max_tokens: 4096
chat_template_args: {enable_thinking: false}
```

`chat_template_args` is passed verbatim as JSON to `mlx_lm.server --chat-template-args`. Qwen3.5 needs `enable_thinking: false` for plain tool use. A version-2 file from an earlier release is migrated in memory on load; the first `iq start` or `iq pick -w` saves it as version 3 and the migration notice lists every dropped key.

## Commands

```
iq doc        — check runtime dependencies and model readiness
iq pick       — pick the catalog model that fits this machine (-w writes it to config.yaml)
iq start      — start the model's mlx_lm.server sidecar, downloading the model when needed
iq stop       — stop the sidecar and sweep orphaned servers
iq restart    — stop then start
iq status     — show running sidecars and memory use
iq pi         — print pi's provider entry for the running sidecar (-w writes ~/.pi/agent/models.json)
iq config     — show or validate config.yaml
```

Run `iq` without arguments for full usage.

## Upgrading from the three-binary release

The `lm` and `kb` binaries are gone, and the inference sidecar is now mlx-lm's own server. Optional cleanup:

```bash
rm -f "$(go env GOPATH)/bin/lm" "$(go env GOPATH)/bin/kb"
rm -rf ~/.config/kb
rm -f ~/.config/iq/infer_server.py ~/.config/iq/embed_server.py ~/.config/iq/cues.yaml \
      ~/.config/iq/kb.json ~/.config/iq/response_cache.json ~/.config/iq/cue_embeddings.json \
      ~/.config/iq/tool_embeddings.json ~/.config/iq/models.json ~/.config/iq/benchmarks.json
rm -rf ~/.config/iq/sessions
```

Model downloads, cache listing, and benchmarks now come from `hf download`, `hf cache ls`, `mlx_lm.manage`, and `mlx_lm.benchmark`.
