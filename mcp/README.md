# Cortisol MCP server (HTTP)

Streamable HTTP MCP for **org knowledge**. Tool schemas are real; set
`MCP_BACKEND=http` to call the Go knowledge API (Atlas vector search).
`MCP_BACKEND=mock` keeps the in-memory fixture store for offline use.

**Slack is not implemented here.** Agents use Slack’s official MCP
(`https://mcp.slack.com/mcp`) beside this server — see
[Slack MCP in this repo](#slack-mcp-official--not-cortisol) and
[`docs/slack-mcp.md`](../docs/slack-mcp.md).

## Tools

| Tool | Purpose |
|------|---------|
| `quack` | Smoke test — returns `this mcp tool work quack quack quack` |
| `knowledge_search` | Top-k knowledge docs (`k` default 5); query may be NL or code |
| `knowledge_post` | Create a knowledge document |

Contract: [`contracts/knowledge.ts`](contracts/knowledge.ts).

## Local run

```bash
cd mcp
npm install
npm run dev
# or: npm run build && npm start
```

Listens on `MCP_HTTP_ADDR` (default `0.0.0.0:3100`).

- Health: `GET http://127.0.0.1:3100/health`
- MCP: `POST http://127.0.0.1:3100/mcp`

Env:

| Variable | Default | Notes |
|----------|---------|--------|
| `MCP_HTTP_ADDR` | `:3100` | `host:port` or `:port` |
| `CORTISOL_ORG_ID` | `demo-org` | Scoped onto knowledge search/post |
| `MCP_BACKEND` | `mock` | `mock` or `http` |
| `CORTISOL_API_BASE` | `http://127.0.0.1:8080` | Used when `MCP_BACKEND=http` |

With `MCP_BACKEND=http`, tools hit `GET/POST /api/v1/knowledge` on the Go API
(`knowledge_documents` + Atlas voyage-code-4 autoEmbed).


```bash
curl -sS -X POST http://127.0.0.1:3100/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"quack","arguments":{}}}'
```

## Slack MCP (official — not Cortisol)

For messages, files, canvases, and posting in Slack, connect **Slack’s** MCP:

- Docs: [Slack MCP server](https://docs.slack.dev/ai/slack-mcp-server/)
- Endpoint: `https://mcp.slack.com/mcp`
- Cursor is a listed partner client; Codex uses `config.toml` (see below)

Repo guide: [`docs/slack-mcp.md`](../docs/slack-mcp.md). Example Codex/Cursor
snippets: [`.codex/config.toml.example`](../.codex/config.toml.example).

## Expose Cortisol MCP to Codex (TUI / app-server)

Open `~/.codex/config.toml` and paste:

```toml
[mcp_servers.cortisol]
url = "http://127.0.0.1:3100/mcp"
startup_timeout_sec = 20
tool_timeout_sec = 60
enabled = true
```

Start this MCP (`npm run dev`), then restart the TUI. Full steps:
[`docs/codex-mcp.md`](../docs/codex-mcp.md).

### Cursor (HTTP)

```json
{
  "mcpServers": {
    "cortisol": {
      "url": "http://127.0.0.1:3100/mcp"
    }
  }
}
```

Connect Slack via Cursor’s Slack MCP / partner flow (see Slack docs). Do not
add fake Slack tools under Cortisol.

## Docker

From repo root (with Go API):

```bash
docker compose up --build
```

MCP publishes `3100`, API publishes `8080`. MongoDB is Atlas (pass `MONGODB_URI`
into the `api` service); it is not started by Compose.
