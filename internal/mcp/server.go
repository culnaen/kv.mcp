// Package mcp implements a hand-rolled JSON-RPC 2.0 MCP server over stdio.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/culnaen/kv.mcp/internal/kv"
)

// ProtocolVersion is the MCP protocol version we implement.
const ProtocolVersion = "2024-11-05"

// Server handles MCP JSON-RPC 2.0 over stdin/stdout.
type Server struct {
	store    kv.Store
	root     string
	maxLines int
	cache    *searchCache
	logger   *log.Logger
}

// NewServer constructs a Server. Logs are written to stderr.
func NewServer(store kv.Store, root string, maxLines int) *Server {
	return &Server{
		store:    store,
		root:     root,
		maxLines: maxLines,
		cache:    &searchCache{},
		logger:   log.New(os.Stderr, "kv.mcp ", log.LstdFlags|log.Lmicroseconds),
	}
}

// jsonrpcRequest is a JSON-RPC 2.0 request or notification.
// id absent => notification. id may be a number, string, or null.
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonrpcResponse is a JSON-RPC 2.0 response.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

// jsonrpcError is a JSON-RPC 2.0 error object.
type jsonrpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error codes per JSON-RPC 2.0 spec.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// Run reads JSON-RPC requests from r, writes responses to w, logs to stderr.
// Blocks until stdin closes or shutdown is received.
func (s *Server) Run(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	// Allow long lines; default bufio.MaxScanTokenSize is 64KB which is too small for some payloads.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	writer := bufio.NewWriter(w)
	defer writer.Flush() //nolint:errcheck

	encode := func(resp jsonrpcResponse) error {
		resp.JSONRPC = "2.0"
		data, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		if _, err := writer.Write(data); err != nil {
			return err
		}
		if _, err := writer.Write([]byte("\n")); err != nil {
			return err
		}
		return writer.Flush()
	}

	shuttingDown := false
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req jsonrpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.logger.Printf("parse error: %v", err)
			_ = encode(jsonrpcResponse{
				ID:    json.RawMessage("null"),
				Error: &jsonrpcError{Code: codeParseError, Message: "parse error: " + err.Error()},
			})
			continue
		}
		if req.JSONRPC != "2.0" {
			if len(req.ID) > 0 {
				_ = encode(jsonrpcResponse{
					ID:    req.ID,
					Error: &jsonrpcError{Code: codeInvalidRequest, Message: "jsonrpc must be \"2.0\""},
				})
			}
			continue
		}

		isNotification := len(req.ID) == 0

		result, rpcErr := s.dispatch(req.Method, req.Params)

		if isNotification {
			// Notifications get no response.
			if req.Method == "notifications/initialized" {
				s.logger.Printf("client initialized")
			}
			continue
		}

		resp := jsonrpcResponse{ID: req.ID}
		if rpcErr != nil {
			resp.Error = rpcErr
		} else {
			resp.Result = result
		}
		if err := encode(resp); err != nil {
			s.logger.Printf("write error: %v", err)
			return err
		}

		if req.Method == "shutdown" {
			shuttingDown = true
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	if shuttingDown {
		s.logger.Printf("shutdown complete")
	}
	return nil
}

// dispatch routes a method to the appropriate handler.
// Returns (result, nil) on success or (nil, *jsonrpcError) on failure.
func (s *Server) dispatch(method string, params json.RawMessage) (interface{}, *jsonrpcError) {
	switch method {
	case "initialize":
		return s.handleInitialize(params)
	case "notifications/initialized":
		return nil, nil
	case "tools/list":
		return toolsList(), nil
	case "tools/call":
		return s.handleToolsCall(params)
	case "shutdown":
		return nil, nil
	case "ping":
		return map[string]interface{}{}, nil
	default:
		return nil, &jsonrpcError{Code: codeMethodNotFound, Message: "method not found: " + method}
	}
}

// handleInitialize returns the server capability descriptor.
func (s *Server) handleInitialize(_ json.RawMessage) (interface{}, *jsonrpcError) {
	return map[string]interface{}{
		"protocolVersion": ProtocolVersion,
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{"listChanged": false},
		},
		"serverInfo": map[string]interface{}{
			"name":    "kv.mcp",
			"version": "0.1.0",
		},
	}, nil
}

// toolsCallParams is the JSON shape of tools/call params.
type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// handleToolsCall dispatches to the concrete tool handler.
func (s *Server) handleToolsCall(params json.RawMessage) (interface{}, *jsonrpcError) {
	var p toolsCallParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &jsonrpcError{Code: codeInvalidParams, Message: "invalid params: " + err.Error()}
		}
	}
	switch p.Name {
	case "get_function":
		return s.handleGetFunction(p.Arguments)
	case "search":
		return s.handleSearch(p.Arguments)
	case "get_code":
		return s.handleGetCode(p.Arguments)
	case "update_function":
		return s.handleUpdateFunction(p.Arguments)
	default:
		return nil, &jsonrpcError{Code: codeInvalidParams, Message: "unknown tool: " + p.Name}
	}
}

// toolResult wraps a successful tool response in MCP content blocks.
func toolResult(payload interface{}) interface{} {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(fmt.Sprintf("%q", err.Error()))
	}
	return map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": string(data),
			},
		},
	}
}
