# iq Plan

## Product Direction

iq is a command-line tool for running offline generative AI on Apple Silicon. `lm` downloads MLX models from Hugging Face into the local cache and benchmarks them, `iq` starts `mlx_lm` inference sidecars and routes each prompt through cue classification, tool detection, knowledge-base retrieval, and inference, and `kb` keeps a private knowledge base. Everything runs on-device with no cloud dependency and no data leaving the machine. The project is a research vehicle for a practical, inspectable inference router where the user stays in control of every layer.

The project is frozen as of 2026-03-20 and kept as a learning artifact. Active AI-assisted development has moved to OpenCode with externally hosted models, and `kb` is superseded by open source alternatives. Direction from here is maintenance: keep the three binaries building and installable, keep governance current, and accept small ergonomic fixes to local model management. No new cloud code paths and no scope expansion.

## Ideas To Explore

Ideas captured for future reference. A bullet list — each line starts with `- IE<N>: ` (sequential N) for stable references. Two kinds: (a) **pre-rubric IE** — `IE<N>: <one-liner>`, awaiting director discussion and the objective-fit rubric (see `AGENTS.md` Approval Boundaries); (b) **AC-pointer** — `IE<N>: <one-liner> → govna/ac<N>-<slug>.md`, pointing at a drafted AC stub not yet through critique. A pre-rubric entry that clears the rubric converts to an AC-pointer at AC-draft time, keeping its `IE<N>` number. Remove entries when the idea is rejected, retired, or (for AC-pointers) the AC has shipped and its file deleted. Not a historical record.

