# Setup MCP in Codex

## How hard is this

1. Start our MCP server.
2. Open the Codex config file.
3. Paste the block below.
4. Restart the TUI.

---

### Step 1 — start our MCP

```bash
cd mcp
npm run dev
```

Keep that terminal open.

---

### Step 2 — open this file

```text
~/.codex/config.toml
```

Create it if it does not exist:

```bash
mkdir -p ~/.codex
touch ~/.codex/config.toml
```

Open it in any editor (example):

```bash
open -e ~/.codex/config.toml
# or: code ~/.codex/config.toml
# or: nano ~/.codex/config.toml
```

---

### Step 3 — paste this into that file

```toml
[mcp_servers.cortisol]
url = "http://127.0.0.1:3100/mcp"
startup_timeout_sec = 20
tool_timeout_sec = 60
enabled = true
```

Save the file.

That is the whole Codex config for Cortisol MCP. Not YAML. Not `mcpServers`.
This exact TOML table.

Optional Slack (only if you want it too) — paste under the block above:

```toml
[mcp_servers.slack]
url = "https://mcp.slack.com/mcp"
startup_timeout_sec = 30
tool_timeout_sec = 120
enabled = true
```

---

### Step 4 — confirm

```bash
codex mcp list
```

You should see `cortisol`.

---

### Step 5 — use it from our TUI

```bash
cd server
go run ./cmd/tui
```

In the TUI ask: call `quack`

You should get: `this mcp tool work quack quack quack`

---

If `codex mcp list` is empty: you edited the wrong file, or forgot to save.
Path again: `~/.codex/config.toml`
