# Token Benchmark

Compares tool-output token consumption: KV path (kv.mcp tools) vs baseline (Grep + targeted Read).

## Methodology

**Lower-bound indicator:** This benchmark uses a scripted, deterministic harness — not a live LLM agent. It measures raw tool-output tokens, not full agent context tokens. Token counts use the `characters ÷ 4` heuristic (ceiling) — reproducible and dependency-free, but not exact for all LLM tokenizers. Treat results as directional lower-bound estimates. A full LLM-in-the-loop measurement is v2 scope.

**Baseline:** ripgrep for symbol location + 80-line Read window around match (up to 3 files). Realistic, competent non-KV agent — not a strawman.

**KV path:** search() → get_function() → get_code(loc). Only the 4 kv.mcp tools, no grep/read.

## Reproduce

```bash
# One command: clones Xray-core (if absent), indexes, runs benchmark
make bench
```

Or manually:

```bash
# Clone and index Xray-core
git clone --depth=1 https://github.com/XTLS/Xray-core.git /tmp/xray-core
./kv.mcp index --db /tmp/xray-bench.db /tmp/xray-core

# Run benchmark
go run bench/tokens/bench.go --db /tmp/xray-bench.db --root /tmp/xray-core
```

## Results

See [results.md](results.md).
