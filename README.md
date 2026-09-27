# Cortisol CLI

## Developer setup

The development setup has these parts: a MongoDB **Atlas** database, the Go
HTTP API, the HTTP MCP server (agent tools), and optionally the Go TUI /
dashboard. Configure Atlas first, then run the API and MCP in separate
terminals (or `docker compose up`).

For team Snowflake access, follow the [Snowflake local development setup](#snowflake-local-development-setup) below.

### Prerequisites

- Go 1.27 or newer
- Node 20+ (for the MCP server)
- A MongoDB Atlas database (not run locally)
- The Codex CLI on `PATH` to run the TUI
- Docker (optional) for packaging the two HTTP services

### 1. Configure the database

Create or select a MongoDB Atlas cluster and database, then copy
`server/.env.example` to `server/.env`. Edit `server/.env` with the connection
URI and credentials for your database:

```dotenv
MONGODB_URI=mongodb+srv://<db_username>:<db_password>@<your-cluster>/?appName=cortisol-dev
MONGODB_DATABASE=cortisol
DB_USERNAME=<your-database-username>
DB_PASSWORD=<your-database-password>
```

Replace all example values, including the Atlas URI placeholders, with your
own. Keep `server/.env` private; it is gitignored. The server and database
commands below load this file automatically when run from the `server/`
directory.

### 2. Apply database migrations

From the repository root:

```bash
cd server
go run ./cmd/migrate
```

This validates and applies pending migrations to the configured database. To
validate migration files without connecting to MongoDB, run
`go run ./cmd/migrate -check`.

### 3. Run the HTTP API

Before starting, add the Snowflake account URL, PAT, and model to `server/.env`
using the [Cortex setup guide](server/internal/evaluation/docs/snowflake.md). The server requires
these settings to evaluate prompts.

In a terminal from the `server/` directory:

```bash
go run ./cmd/server
```

Default listen address: `127.0.0.1:8080` (`HTTP_ADDR`).

### 4. Run the MCP server (agent tools)

In another terminal:

```bash
cd mcp
npm install
npm run dev
```

- Health: `http://127.0.0.1:3100/health`
- MCP endpoint: `http://127.0.0.1:3100/mcp`

Tools (`knowledge_search`, `knowledge_post`) use **mock** knowledge
backends today. For Codex, open `~/.codex/config.toml` and paste

```
[mcp_servers.cortisol]
url = "http://127.0.0.1:3100/mcp"
startup_timeout_sec = 20
tool_timeout_sec = 60
enabled = true
```

### 5. Docker (API + MCP only)

With Docker running and `server/.env` filled:

```bash
docker compose up --build
```

Publishes API `:8080` and MCP `:3100`. Does not start Mongo, dashboard, or TUI.

Check the API:

```bash
curl http://localhost:8080/health
```

Check MCP:

```bash
curl http://localhost:3100/health
```

To load the linked development/demo records, run `go run ./cmd/seed` from
`server/` after applying migrations. Seeding is optional.

### 6. Run the TUI

Connect Codex to our MCP first — open `~/.codex/config.toml` and paste:

```toml
[mcp_servers.cortisol]
url = "http://127.0.0.1:3100/mcp"
startup_timeout_sec = 20
tool_timeout_sec = 60
enabled = true
```

Start MCP (`cd mcp && npm run dev`), then:

```bash
cd server
go run ./cmd/tui
```

Full steps: [`docs/codex-mcp.md`](docs/codex-mcp.md). The TUI does not load MCP
itself — `codex app-server` reads that file.

The TUI starts `codex app-server` and uses its JSON-RPC stdio interface. Make
sure the Codex CLI is installed, authenticated, and available on `PATH`.
Type a message and press Enter; use `/quit` or Ctrl+C to exit. For TUI logging
and protocol details, see [`server/cmd/tui/README.md`](server/cmd/tui/README.md).

## Snowflake Local Development Setup

The server uses the Snowflake Cortex REST API to evaluate prompts. This
integration does not require a Snowflake database, schema, or warehouse.

### Configure the local environment

Use the same `server/.env` file configured for MongoDB above. If you have not
created it yet, copy `server/.env.example` to `server/.env`.

Set the following values, preserving your MongoDB settings:

```dotenv
# Account configuration (may be shared by the team)
SNOWFLAKE_ACCOUNT_URL=https://<organization>-<account>.snowflakecomputing.com
SNOWFLAKE_MODEL=<model-available-to-your-account>

# Personal credential
SNOWFLAKE_PAT=<your-programmatic-access-token>
```

Replace every placeholder with your own account settings. Use the full HTTPS
account URL, not the Snowsight browser URL. The model in `.env.example` is an
example; choose a model available to your account that supports JSON schema
output. Each developer should use their own PAT and keep `server/.env` private.

These three Snowflake variables are required by the current server. The older
`SNOWFLAKE_ACCOUNT`, `SNOWFLAKE_USER`, `SNOWFLAKE_TOKEN`, `SNOWFLAKE_DATABASE`,
`SNOWFLAKE_SCHEMA`, and `SNOWFLAKE_WAREHOUSE` settings are not used by this
integration.

### Set up access and verify

Follow the [Cortex setup guide](server/internal/evaluation/docs/snowflake.md)
for role permissions, PAT creation, network policy requirements, and a sample
evaluation request.

The server validates configuration at startup. A successful evaluation request
verifies credentials and model access; `/health` only checks that the HTTP
server is running.
