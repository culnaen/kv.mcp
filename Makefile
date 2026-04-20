.PHONY: build test e2e bench

VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o kv.mcp .

test:
	go test ./... -race

e2e: build
	@echo "Cloning Xray-core (if needed)..."
	@if [ ! -d /tmp/xray-core ]; then git clone --depth=1 https://github.com/XTLS/Xray-core.git /tmp/xray-core; fi
	@echo "Indexing Xray-core..."
	@./kv.mcp index --db /tmp/xray-e2e.db /tmp/xray-core
	@echo "Running 10 canned queries..."
	@for q in $$(jq -r '.[].query' testdata/xray-core-queries.json); do \
		echo "Query: $$q"; \
		printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"e2e","version":"0.1"}}}\n{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search","arguments":{"query":"'"$$q"'"}}}\n{"jsonrpc":"2.0","id":3,"method":"shutdown","params":{}}\n' | ./kv.mcp serve --db /tmp/xray-e2e.db --root /tmp/xray-core 2>/dev/null | grep -q "result" && echo "  OK" || echo "  FAIL"; \
	done

bench: build
	@echo "Cloning Xray-core (if needed)..."
	@if [ ! -d /tmp/xray-core ]; then git clone --depth=1 https://github.com/XTLS/Xray-core.git /tmp/xray-core; fi
	@echo "Indexing Xray-core..."
	@./kv.mcp index --db /tmp/xray-bench.db /tmp/xray-core
	@echo "Running token benchmark..."
	@go run bench/tokens/bench.go --db /tmp/xray-bench.db --root /tmp/xray-core --output bench/tokens/results.md
