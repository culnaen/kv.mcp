package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// runServer feeds newline-delimited JSON requests to Server.Run and returns
// the parsed responses. Each non-empty line in output is one response.
func runServer(t *testing.T, s *Server, requests ...string) []jsonrpcResponse {
	t.Helper()
	input := strings.Join(requests, "\n") + "\n"
	var out bytes.Buffer
	if err := s.Run(strings.NewReader(input), &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var responses []jsonrpcResponse
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var r jsonrpcResponse
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("decode response %q: %v", line, err)
		}
		responses = append(responses, r)
	}
	return responses
}

func newRunServer(t *testing.T) *Server {
	t.Helper()
	store, root := newTempStore(t)
	return NewServer(store, root, 500)
}

func TestRunParseError(t *testing.T) {
	s := newRunServer(t)
	resps := runServer(t, s, "not json at all")
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error == nil || resps[0].Error.Code != codeParseError {
		t.Errorf("expected parse error code, got %+v", resps[0].Error)
	}
	if string(resps[0].ID) != "null" {
		t.Errorf("expected id=null for parse error, got %s", resps[0].ID)
	}
}

func TestRunInvalidVersion(t *testing.T) {
	s := newRunServer(t)
	req := `{"jsonrpc":"1.0","id":1,"method":"ping"}`
	resps := runServer(t, s, req)
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error == nil || resps[0].Error.Code != codeInvalidRequest {
		t.Errorf("expected invalid request code, got %+v", resps[0].Error)
	}
}

func TestRunNotificationNoResponse(t *testing.T) {
	s := newRunServer(t)
	// Notifications have no id field — server must send no response.
	notif := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	shutdown := `{"jsonrpc":"2.0","id":1,"method":"shutdown"}`
	resps := runServer(t, s, notif, shutdown)
	// Only shutdown response expected; notification gets no response.
	if len(resps) != 1 {
		t.Fatalf("expected 1 response (shutdown only), got %d: %+v", len(resps), resps)
	}
	if resps[0].Error != nil {
		t.Errorf("unexpected error on shutdown: %+v", resps[0].Error)
	}
}

func TestRunInitializeRoundtrip(t *testing.T) {
	s := newRunServer(t)
	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	shutdown := `{"jsonrpc":"2.0","id":2,"method":"shutdown"}`
	resps := runServer(t, s, req, shutdown)
	if len(resps) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Fatalf("initialize error: %+v", resps[0].Error)
	}
	result, ok := resps[0].Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", resps[0].Result)
	}
	if result["protocolVersion"] != ProtocolVersion {
		t.Errorf("protocolVersion=%v want %v", result["protocolVersion"], ProtocolVersion)
	}
}

func TestRunShutdownExitsCleanly(t *testing.T) {
	s := newRunServer(t)
	req := `{"jsonrpc":"2.0","id":1,"method":"shutdown"}`
	resps := runServer(t, s, req)
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Errorf("unexpected error: %+v", resps[0].Error)
	}
}

func TestRunMultiRequestSequence(t *testing.T) {
	s := newRunServer(t)
	init_ := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	list := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	shutdown := `{"jsonrpc":"2.0","id":3,"method":"shutdown"}`
	resps := runServer(t, s, init_, list, shutdown)
	if len(resps) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(resps))
	}
	for i, r := range resps {
		if r.Error != nil {
			t.Errorf("response[%d] error: %+v", i, r.Error)
		}
	}
}

func TestRunUnknownMethod(t *testing.T) {
	s := newRunServer(t)
	req := `{"jsonrpc":"2.0","id":1,"method":"no_such_method"}`
	shutdown := `{"jsonrpc":"2.0","id":2,"method":"shutdown"}`
	resps := runServer(t, s, req, shutdown)
	if len(resps) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resps))
	}
	if resps[0].Error == nil || resps[0].Error.Code != codeMethodNotFound {
		t.Errorf("expected method not found, got %+v", resps[0].Error)
	}
}

func TestRunEmptyLinesSkipped(t *testing.T) {
	s := newRunServer(t)
	// Empty lines between requests must be silently skipped.
	var out bytes.Buffer
	input := "\n\n" + `{"jsonrpc":"2.0","id":1,"method":"shutdown"}` + "\n"
	if err := s.Run(strings.NewReader(input), &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 response, got %d: %v", len(lines), lines)
	}
}
