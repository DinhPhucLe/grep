# Slack MCP

Cortisol MCP does not implement Slack tools. Configure Slack’s hosted MCP for
workspace search (messages and files), send message, canvases, etc.

- Docs: https://docs.slack.dev/ai/slack-mcp-server/
- Endpoint: `https://mcp.slack.com/mcp`
- Transport: Streamable HTTP (JSON-RPC 2.0). No SSE.

| Task | Server |
|------|--------|
| Org knowledge (`knowledge_*`, `quack`) | Cortisol MCP |
| Slack messages, files, channels, post | Slack MCP (`mcp.slack.com`) |

## Cursor

Slack lists **Cursor** as a partner client. Prefer Cursor’s Slack MCP /
OAuth UI when available. Otherwise add a remote MCP URL
`https://mcp.slack.com/mcp` per Cursor’s current MCP settings format.

## Codex (powers our TUI)

Add to `~/.codex/config.toml` or project `.codex/config.toml` (trusted
project only). See [`.codex/config.toml.example`](../.codex/config.toml.example).

```toml
[mcp_servers.slack]
url = "https://mcp.slack.com/mcp"
startup_timeout_sec = 30
tool_timeout_sec = 120
```

Then complete Slack OAuth for that server (`codex mcp login slack` when your
Codex build supports it). App identity / admin approval rules are owned by
Slack — see their [App Identity](https://docs.slack.dev/ai/slack-mcp-server/#app-identity)
and OAuth sections.

Only directory-published or **internal** Slack apps may use MCP; unlisted apps
are blocked.

## Scopes (user token) — high-signal subset

| Need | Scopes (from Slack docs) |
|------|---------------------------|
| Search messages | `search:read.public`, `search:read.private`, … |
| Search files | `search:read.files` |
| Read file bytes | `files:read` |
| Send message | `chat:write` |

Full matrix: https://docs.slack.dev/ai/slack-mcp-server/#oauth-scopes

## Security

Connecting Slack MCP gives the agent access to workspace data. Prefer a
restricted channel allowlist; audit MCP usage in Slack admin logs. Do not put
Slack bot tokens into the Cortisol MCP process.
