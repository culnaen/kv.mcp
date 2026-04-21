# Token Benchmark

Measures raw tool-output token consumption for two approaches to code navigation: the KV path (kv.mcp tools) versus the baseline path (ripgrep + targeted file read). No LLM is involved — this is a measurement of what each approach returns to an agent, not of agent reasoning itself.

## What This Measures

Two navigation paths answer the same symbol query:

| Path | Tools used |
|------|-----------|
| **Baseline** | ripgrep to locate the symbol, then an 80-line Read window around each match (up to 3 files) |
| **KV** | `search()` → `get_function()` → `get_code(loc)` — kv.mcp tools only |

The benchmark counts the tokens each path produces as output. Fewer output tokens means less context consumed per navigation step, which compounds across a long agent session.

## Methodology

**Token estimation.** Token counts use `ceil(characters / 4)`. This is reproducible without any tokenizer dependency. It is a lower-bound estimate: real LLM tokenizers (BPE-based) encode common subwords more efficiently, so actual token counts are higher. Treat results as directional, not exact.

**Baseline path.** ripgrep locates matching files, then a fixed 80-line window is read around each match, for up to 3 files. This represents a competent non-KV agent — not a strawman that reads entire files.

**KV path.** `search()` returns candidate locations, `get_function()` extracts the function body, `get_code(loc)` retrieves surrounding context. Only kv.mcp tools are used; no grep or direct file reads.

**Harness.** The benchmark is scripted and deterministic. It does not run a live LLM. It records raw tool output sizes only.

## How to Reproduce

**One command from the repo root:**

```bash
make bench
```

This does the following:

1. Builds the `kv.mcp` binary.
2. Clones `XTLS/Xray-core` to `/tmp/xray-core` if not already present (shallow clone).
3. Indexes Xray-core into `/tmp/xray-bench.db`.
4. Runs the benchmark binary and writes results to `bench/tokens/results.md`.

**Manual equivalent (if you want more control):**

```bash
# Build
CGO_ENABLED=0 go build -trimpath -o kv.mcp .

# Clone Xray-core (skip if already present)
git clone --depth=1 https://github.com/XTLS/Xray-core.git /tmp/xray-core

# Index
./kv.mcp index --db /tmp/xray-bench.db /tmp/xray-core

# Run benchmark
go run bench/tokens/bench.go --db /tmp/xray-bench.db --root /tmp/xray-core --output bench/tokens/results.md
```

## Results Table

See [results.md](results.md). Column definitions:

| Column | Meaning |
|--------|---------|
| `Query` | The symbol name searched |
| `Baseline Turns` | Number of tool calls in the baseline path |
| `Baseline Tokens` | Estimated tokens returned by the baseline path |
| `KV Turns` | Number of tool calls in the KV path |
| `KV Tokens` | Estimated tokens returned by the KV path |
| `Δ Tokens` | `KV Tokens - Baseline Tokens` (negative = KV used fewer) |
| `Win` | Which path consumed fewer tokens; `No match` when neither path found the symbol |

**Reduction%** (not shown in the table but referenced in summaries) is computed as:

```
Reduction% = (Baseline Tokens - KV Tokens) / Baseline Tokens × 100
```

## The 10 Queries

Queries come from [`testdata/xray-core-queries.json`](testdata/xray-core-queries.json). They are real symbol names an agent would navigate to when exploring a proxy codebase, not synthetic constructs.

| # | Query | Why it was chosen |
|---|-------|-------------------|
| 1 | `Dial` | Traces the outbound connection establishment path — central to understanding how proxy connections are initiated |
| 2 | `Register` | Finds protocol handler and feature registration points — reveals the plugin and extension architecture |
| 3 | `ServeHTTP` | Locates HTTP handler implementations — entry points for inbound HTTP/HTTPS traffic |
| 4 | `ParseConfig` | Finds configuration parsing logic — how JSON/TOML config maps to internal structures |
| 5 | `NewHandler` | Finds handler constructors across proxy protocols — shows factory patterns and dependency injection wiring |
| 6 | `Process` | Finds the core packet/connection processing pipeline — the main data-plane flow for any proxy protocol |
| 7 | `WriteLog` | Locates logging infrastructure — understand access log format, structured fields, and log levels |
| 8 | `Dispatch` | Finds traffic dispatcher logic — how connections are routed from inbound to outbound handlers |
| 9 | `ReadHeader` | Locates protocol header parsing — reveals wire format for VMess, VLESS, Trojan, and similar protocols |
| 10 | `AddUser` | Finds dynamic user management — how users are added or removed at runtime without a restart |

Queries 4 (`ParseConfig`) and 7 (`WriteLog`) returned no matches in the indexed corpus. Both paths scored zero meaningful tokens on those queries, so they are excluded from win/loss tallies.

## Limitations

**char/4 heuristic is not exact.** The estimate consistently underestimates real token counts. BPE tokenizers produce fewer tokens for common English substrings and Go keywords than the character-based heuristic predicts. Use these numbers as relative comparisons, not absolute token budgets.

**No LLM in the loop.** The benchmark measures tool output only. It does not account for agent reasoning tokens, system prompts, tool definitions, or the cost of iterating when a first result is insufficient. A live agent session will consume more tokens than these numbers show on both paths.

**Host platform only.** Indexing captures functions compiled for the GOOS/GOARCH of the machine running the index command. Functions gated by build tags for other operating systems are absent from the index. The baseline (ripgrep) is not affected by this.

**Single corpus.** All queries run against Xray-core. Results may differ on codebases with different size, naming conventions, or function granularity.

**KV path requires prior indexing.** The baseline works on any directory without setup. The KV path requires a pre-built index. Index build time is not counted in the benchmark.
