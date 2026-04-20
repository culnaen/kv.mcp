# Token Benchmark

Compares tool-output token consumption: KV path (kv.mcp tools) vs baseline (Grep + targeted Read).

## Methodology

**Lower-bound indicator:** This benchmark uses a scripted, deterministic harness — not a live LLM agent. It measures raw tool-output tokens, not full agent context tokens. A full LLM-in-the-loop measurement is v2 scope.

**Baseline:** ripgrep for symbol location + 80-line Read window around match (up to 3 files). Realistic, competent non-KV agent — not a strawman.

**KV path:** search() → get_function() → get_code(loc). Only the 4 kv.mcp tools, no grep/read.

**Token approximation:** characters ÷ 4 (ceiling). Reproducible, dependency-free, good enough for relative comparison.

## Reproduce

```bash
# Ensure Xray-core is indexed:
./kv.mcp index --db /tmp/xray.db /tmp/xray-core

# Run benchmark:
cd bench/tokens
go run bench.go --db /tmp/xray.db --root /tmp/xray-core
```

## Results

See results.md.
