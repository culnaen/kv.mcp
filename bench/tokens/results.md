# Token Benchmark Results

| # | Query | Baseline Turns | Baseline Tokens | KV Turns | KV Tokens | Δ Tokens | Win |
|---|-------|---------------|----------------|---------|----------|---------|-----|
| 1 | Dial | 4 | 14442 | 3 | 8655 | -5787 | KV |
| 2 | Register | 4 | 8629 | 3 | 3939 | -4690 | KV |
| 3 | ServeHTTP | 4 | 1497 | 3 | 1445 | -52 | KV |
| 4 | ParseConfig | 1 | 0 | 1 | 18 | 18 | No match |
| 5 | NewHandler | 4 | 2136 | 3 | 611 | -1525 | KV |
| 6 | Process | 4 | 7363 | 3 | 5069 | -2294 | KV |
| 7 | WriteLog | 1 | 0 | 1 | 18 | 18 | No match |
| 8 | Dispatch | 4 | 9447 | 3 | 12854 | 3407 | Baseline |
| 9 | ReadHeader | 3 | 1083 | 1 | 18 | -1065 | KV |
| 10 | AddUser | 4 | 2758 | 3 | 2396 | -362 | KV |

**Summary:** KV wins: 7 / 8 meaningful queries (excluding 2 no-match queries).
