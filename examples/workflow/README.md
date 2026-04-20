# Agent Workflow: Exploring Proxy Architecture with kv.mcp

This document shows a realistic 10-step agent workflow that uses kv.mcp tools exclusively to explore Xray-core's proxy architecture. The session demonstrates how agents navigate a large codebase efficiently without grep, interactive searching, or manual file reading.

## Scenario

An agent is tasked with understanding how Xray-core's outbound proxy subsystem works. Key questions:
- What proxies are supported?
- How does the SOCKS5 implementation work?
- What is the plugin registration pattern?

**Without kv.mcp:** Grep for patterns, read 300-line files, manually trace imports. Estimated token cost: ~80,000.

**With kv.mcp:** Search indexed metadata, get precise function locations, read only relevant lines, annotate findings. Estimated token cost: ~35,000.

---

## Step 1: Initial Search — Find Connection Handlers

**Agent query:** "Where are the proxy connection handlers?"

**Tool call:**
```
search("Dial")
```

**Response:**
```json
{
  "matches": 50,
  "results": [
    {
      "name": "socks.(*Client).Dial",
      "description": "",
      "file": "proxy/socks/client.go",
      "kind": "method"
    },
    {
      "name": "vmess.(*Handler).Dial",
      "description": "",
      "file": "proxy/vmess/handler.go",
      "kind": "method"
    },
    {
      "name": "trojan.(*Handler).Dial",
      "description": "",
      "file": "proxy/trojan/handler.go",
      "kind": "method"
    },
    {
      "name": "net.Dial",
      "description": "stdlib",
      "file": "internal/net/address.go",
      "kind": "function"
    },
    {
      "name": "shadowsocks.(*Handler).Dial",
      "description": "",
      "file": "proxy/shadowsocks/handler.go",
      "kind": "method"
    }
  ]
}
```

**Agent inference:** Multiple proxy protocols have Dial methods. The pattern suggests each protocol implements a proxy handler interface.

**Tokens used:** ~1,200

---

## Step 2: Inspect SOCKS5 Client Implementation

**Agent action:** "Get the signature and dependencies for the SOCKS5 Dial method."

**Tool call:**
```
get_function("socks.(*Client).Dial")
```

**Response:**
```json
{
  "name": "socks.(*Client).Dial",
  "signature": "func (c *Client) Dial(ctx context.Context, network string, destination *net.Destination) (net.Connection, error)",
  "loc": "proxy/socks/client.go:45-89",
  "file": "proxy/socks/client.go",
  "receiver": "(*Client)",
  "params": ["ctx context.Context", "network string", "destination *net.Destination"],
  "returns": ["net.Connection", "error"],
  "depends": [
    "net.Dial",
    "context.Context",
    "proto.CommandType",
    "socks.ConnectRequest"
  ],
  "test": "socks.TestClientDial"
}
```

**Agent inference:** The method depends on `ConnectRequest` — likely the SOCKS5 protocol message. The location spans lines 45-89, so approximately 45 lines total.

**Tokens used:** ~800

---

## Step 3: Read Only the Relevant Lines

**Agent action:** "Show me the implementation of socks.(*Client).Dial."

**Tool call:**
```
get_code("proxy/socks/client.go:45-89")
```

**Response:**
```
45: func (c *Client) Dial(ctx context.Context, network string, destination *net.Destination) (net.Connection, error) {
46:     rawConn, err := c.netDialer.DialContext(ctx, "tcp", c.ServerAddr.String())
47:     if err != nil {
48:         return nil, newError("failed to connect to SOCKS server").Base(err)
49:     }
50:
51:     defer func() {
52:         if err != nil {
53:             rawConn.Close()
53:         }
54:     }()
55:
56:     conn := net.NewConnection(rawConn)
57:     request := &socks.ConnectRequest{
58:         Command:       socks.CommandConnect,
59:         DestAddress:   destination.Address,
59:         DestPort:      destination.Port,
60:     }
61:
62:     if err := conn.WriteBuffer(request.Marshal()); err != nil {
63:         return nil, newError("failed to send CONNECT request").Base(err)
64:     }
65:
66:     response := &socks.ConnectResponse{}
67:     if err := conn.ReadBuffer(response.Unmarshal); err != nil {
68:         return nil, newError("failed to read CONNECT response").Base(err)
69:     }
69:
70:     if response.Status != socks.StatusSuccess {
71:         return nil, newError("SOCKS server returned error: %v", response.Status)
72:     }
73:
74:     return conn, nil
75: }
```

**Agent inference:** 
- SOCKS5 client connects to a SOCKS server via TCP
- Sends a `ConnectRequest` with the destination address and port
- Reads a `ConnectResponse` and checks for success
- Returns the underlying connection

**Tokens used:** ~1,600

---

## Step 4: Search for the Processing Pipeline

**Agent query:** "How are incoming packets processed?"

**Tool call:**
```
search("Process")
```

**Response:**
```json
{
  "matches": 37,
  "results": [
    {
      "name": "dispatcher.(*DefaultDispatcher).Process",
      "description": "Routes inbound connections to appropriate protocol handlers",
      "file": "app/dispatcher/default.go",
      "kind": "method"
    },
    {
      "name": "proxy.Handler.Process",
      "description": "",
      "file": "proxy/proxy.go",
      "kind": "method"
    },
    {
      "name": "inbound.Manager.Process",
      "description": "",
      "file": "app/inbound/manager.go",
      "kind": "method"
    },
    {
      "name": "routing.Router.Process",
      "description": "Matches connection against routing rules",
      "file": "app/router/router.go",
      "kind": "method"
    },
    {
      "name": "link.Reader.Process",
      "description": "",
      "file": "transport/link/reader.go",
      "kind": "method"
    }
  ]
}
```

**Agent inference:** The dispatcher is the central router. Inbound connections flow through the dispatcher, which routes them based on rules.

**Tokens used:** ~900

---

## Step 5: Inspect the Dispatcher

**Agent action:** "Get metadata for the dispatcher's Process method."

**Tool call:**
```
get_function("dispatcher.(*DefaultDispatcher).Process")
```

**Response:**
```json
{
  "name": "dispatcher.(*DefaultDispatcher).Process",
  "signature": "func (d *DefaultDispatcher) Process(ctx context.Context, link *link.Link) error",
  "loc": "app/dispatcher/default.go:123-198",
  "file": "app/dispatcher/default.go",
  "receiver": "(*DefaultDispatcher)",
  "params": ["ctx context.Context", "link *link.Link"],
  "returns": ["error"],
  "depends": [
    "routing.Router",
    "outbound.Manager",
    "link.Reader",
    "link.Writer",
    "context.Context"
  ],
  "test": "dispatcher.TestDispatcherProcess",
  "description": "Routes inbound connections to appropriate protocol handlers"
}
```

**Agent inference:** The dispatcher coordinates between the router (routing decisions) and outbound manager (selecting outbound proxy). The link contains the connection stream.

**Tokens used:** ~750

---

## Step 6: Read the Dispatcher Implementation

**Agent action:** "Show me how the dispatcher routes connections."

**Tool call:**
```
get_code("app/dispatcher/default.go:123-198")
```

**Response:**
```
123: func (d *DefaultDispatcher) Process(ctx context.Context, link *link.Link) error {
124:     inbound := link.InboundSource
125:
126:     routingCtx := &routing.Context{
127:         Inbound:  inbound,
128:         TargetIP: link.Destination.Address.IP(),
129:     }
130:
131:     route, err := d.router.Route(ctx, routingCtx)
132:     if err != nil {
132:         return newError("routing failed").Base(err)
133:     }
133:
134:     outbound, err := d.outboundManager.GetOutbound(route.OutboundTag)
135:     if err != nil {
136:         return newError("outbound not found").Base(err)
137:     }
137:
138:     handler := outbound.GetHandler()
139:     if handler == nil {
140:         return newError("outbound has no handler")
141:     }
141:
142:     ctx = context.WithValue(ctx, "route", route)
143:
144:     conn, err := handler.Dial(ctx, "tcp", &link.Destination)
145:     if err != nil {
145:         return newError("dial failed").Base(err)
146:     }
146:
147:     if err := link.Forward(conn); err != nil {
148:         return newError("forwarding failed").Base(err)
149:     }
149:
150:     return nil
151: }
```

**Agent inference:** 
- Routing context is created from inbound connection metadata
- Router selects the appropriate outbound
- Handler is retrieved from the outbound
- Handler.Dial establishes the connection
- Data is forwarded through the link

This confirms the handler interface pattern.

**Tokens used:** ~1,500

---

## Step 7: Discover Plugin Registration

**Agent query:** "How are handlers registered?"

**Tool call:**
```
search("Register")
```

**Response:**
```json
{
  "matches": 42,
  "results": [
    {
      "name": "proxy.RegisterInbound",
      "description": "",
      "file": "proxy/proxy.go",
      "kind": "function"
    },
    {
      "name": "proxy.RegisterOutbound",
      "description": "",
      "file": "proxy/proxy.go",
      "kind": "function"
    },
    {
      "name": "inbound.Register",
      "description": "Registers an inbound handler factory",
      "file": "app/inbound/manager.go",
      "kind": "function"
    },
    {
      "name": "outbound.Register",
      "description": "Registers an outbound handler factory",
      "file": "app/outbound/manager.go",
      "kind": "function"
    },
    {
      "name": "socks.init",
      "description": "",
      "file": "proxy/socks/socks.go",
      "kind": "function"
    }
  ]
}
```

**Agent inference:** There's a plugin registration pattern. Each protocol (SOCKS, VMess, etc.) likely calls `register` in its init() function.

**Tokens used:** ~800

---

## Step 8: Inspect the Outbound Registration

**Agent action:** "How does outbound registration work?"

**Tool call:**
```
get_function("outbound.Register")
```

**Response:**
```json
{
  "name": "outbound.Register",
  "signature": "func Register(name string, creator OutboundCreator) error",
  "loc": "app/outbound/manager.go:34-52",
  "file": "app/outbound/manager.go",
  "params": ["name string", "creator OutboundCreator"],
  "returns": ["error"],
  "depends": [
    "OutboundCreator",
    "sync.Map"
  ],
  "test": "outbound.TestRegister",
  "description": "Registers an outbound handler factory"
}
```

**Agent inference:** Registration is type-safe, taking a factory function (creator). The factory pattern allows lazy instantiation.

**Tokens used:** ~650

---

## Step 9: Annotate for Future Sessions

**Agent action:** "Record what we learned about the SOCKS handler."

**Tool call:**
```
update_function("socks.(*Client).Dial", {
  "description": "Establishes SOCKS5 connection to server. Sends ConnectRequest with destination address/port, validates ConnectResponse, and returns connection. Caller responsible for protocol negotiation (auth, version handshake) before dialing."
})
```

**Response:**
```json
{
  "status": "ok",
  "name": "socks.(*Client).Dial",
  "updated_fields": ["description"]
}
```

**Agent inference:** Annotation is persisted in the curated bucket and survives reindexing. Future sessions will see this description in search results.

**Tokens used:** ~600

---

## Step 10: Summary and Token Accounting

**Agent summary:**

Xray-core's proxy architecture follows this pattern:

1. Inbound connection arrives
2. Dispatcher routes to appropriate outbound
3. Outbound handler's Dial method establishes remote connection
4. Data is forwarded through the established connection

Supported proxies (SOCKS, VMess, Trojan, Shadowsocks) each implement a Dial method, allowing the dispatcher to treat them uniformly.

Registration is done via init() functions, storing factories in a registry for lazy instantiation.

**Token Summary:**

| Step | Operation | Estimated Tokens |
|------|-----------|------------------|
| 1 | search("Dial") | 1,200 |
| 2 | get_function() | 800 |
| 3 | get_code() (45 lines) | 1,600 |
| 4 | search("Process") | 900 |
| 5 | get_function() | 750 |
| 6 | get_code() (75 lines) | 1,500 |
| 7 | search("Register") | 800 |
| 8 | get_function() | 650 |
| 9 | update_function() | 600 |
| **Total** | | **~9,200** |

**Comparison:**

- **grep + read approach:** 80-line window per file, ~6-10 files examined, plus grep noise filtering = ~80,000 tokens
- **kv.mcp approach:** Curated metadata + targeted line ranges = ~9,200 tokens
- **Savings:** ~88% token reduction

Real implementations may vary based on search quality and code density, but this 8-10x improvement is representative of typical medium-to-large codebases.

---

## Key Insights

1. **Search first:** Most exploration begins with `search()` to find candidates. The ~50-match limit prevents overwhelming context.

2. **Metadata guides navigation:** `get_function()` reveals dependencies and locations without reading code, allowing strategic decisions about what to read next.

3. **Read with precision:** `get_code()` pulls only the lines you need, not entire files or functions.

4. **Annotate discoveries:** `update_function()` persists knowledge across sessions, making future explorations more efficient.

5. **Patterns emerge:** Repeated structures (all proxies have Dial, all register themselves) are easier to spot when viewing curated metadata rather than raw code.

---

## Next Steps

- Read the [Claude Code Setup Guide](../claude-code-setup/README.md) to configure kv.mcp for your own projects
- Modify the search queries in this example to explore different subsystems (routing, inbound protocols, transport)
- Annotate functions as you discover them to compound improvements for future sessions
