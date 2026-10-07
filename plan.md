# iq Plan

## Product Direction

iq runs one local MLX model for pi. `iq pick` reads unified memory and chooses the largest verified model in its embedded catalog that fits; `iq start` downloads it with `hf` when needed and runs it as a managed `mlx_lm.server` sidecar; `iq pi` writes the provider entry pi needs. Tool use, sessions, RAG, and web search belong to pi and its packages. Everything runs on-device with no cloud dependency and no data leaving the machine.

The project was frozen on 2026-03-20 as a learning artifact and reopened on 2026-10-06 with this single purpose. The `lm` and `kb` binaries, the prompt pipeline, the cue library, the tool registry, the response cache, web search, the knowledge base, and the embed sidecar were removed; `arch.md` records what they did and why they went. Day-to-day AI-assisted development uses Claude Code with hosted models. Direction from here: keep `iq` building and installable, keep governance current, keep the catalog honest through verification runs, and take on only work that makes iq a better counterpart to pi. No new cloud code paths.

## Ideas To Explore

Ideas captured for future reference. A bullet list — each line starts with `- IE<N>: ` (sequential N) for stable references. Two kinds: (a) **pre-rubric IE** — `IE<N>: <one-liner>`, awaiting director discussion and the objective-fit rubric (see `AGENTS.md` Approval Boundaries); (b) **AC-pointer** — `IE<N>: <one-liner> → govna/ac<N>-<slug>.md`, pointing at a drafted AC stub not yet through critique. A pre-rubric entry that clears the rubric converts to an AC-pointer at AC-draft time, keeping its `IE<N>` number. Remove entries when the idea is rejected, retired, or (for AC-pointers) the AC has shipped and its file deleted. Not a historical record.


