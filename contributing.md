# Contributing to Grep

For the project overview, see [README.md](README.md). This guide covers local
setup, authentication, services, and verification.

## Legacy naming: Cortisol → Grep

**Grep is the current project name. Cortisol is the legacy name.** You will still
see it in environment variables, paths, and internal identifiers throughout the
setup guides and codebase:

- Environment variables such as `CORTISOL_SERVER_URL`, `CORTISOL_SESSION_TOKEN`,
  and `CORTISOL_DEFAULT_ORG_ID`.
- The credentials path `~/.cortisol/credentials`.
- Database names and test prefixes such as `cortisol` and `cortisol_test_`.
- The MCP configuration section `[mcp_servers.cortisol]` and Go module name
  `cortisol-server`.

Keep these legacy identifiers exactly as shown in the setup examples. The product
rename has not renamed the corresponding configuration or code; replacing them
with `GREP_*` variables or `~/.grep` paths will not configure the current app.

## Developer setup

The development setup has these parts: a MongoDB **Atlas** database, the Go
HTTP API, the HTTP MCP server (agent tools), and optionally the Go TUI /
dashboard. Configure Atlas first, then run the API and MCP in separate
terminals (or `docker compose up`).

For team Snowflake access, follow the [Snowflake local development setup](#snowflake-local-development-setup) below.

### Prerequisites

- Go 1.27 or newer
- Node 20+ (for the MCP server and dashboard)
- `jq` (for the session-token export command below)
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

After migrate, seed once so the default demo org exists (GitHub logins auto-join
it):

```bash
go run ./cmd/seed
```

### 3. Configure GitHub OAuth (team login)

Login uses **GitHub device flow** on the dashboard. The TUI opens the dashboard
and waits for the user to connect the terminal to that session. Knowledge
search/post require a session from this flow.

#### Create the OAuth App (one shared app for the team is fine)

1. Open [GitHub → Settings → Developer settings → OAuth Apps → New OAuth App](https://github.com/settings/developers)
2. Fill in:
   - **Application name:** e.g. `Grep local`
   - **Homepage URL:** `http://127.0.0.1:8080`
   - **Authorization callback URL:** `http://127.0.0.1:8080`  
     (required by the form; device flow does **not** redirect here)
3. Create the app, then open it and **enable Device Flow**. Save.
4. Copy the **Client ID**. If GitHub shows a **Client secret**, copy that too.

If Device Flow is off, Sign in fails with `device_flow_disabled`.

#### Put credentials in `server/.env`

```dotenv
GITHUB_CLIENT_ID=<oauth-app-client-id>
# Optional — only if your OAuth App has a client secret
# GITHUB_CLIENT_SECRET=<oauth-app-client-secret>

# Optional overrides
# CORTISOL_DEFAULT_ORG_ID=<hex ObjectId of the org everyone joins>
# AUTH_SESSION_TTL=720h
```

`GITHUB_CLIENT_ID` is required to start the HTTP API. New GitHub users are
upserted and **auto-joined** into the default org (seeded NovaPay unless
`CORTISOL_DEFAULT_ORG_ID` is set). Org invitations are not implemented yet.

#### How each person logs in

1. Start the API (`go run ./cmd/server` from `server/`).
2. **Dashboard:** `cd dashboard && npm install && npm run dev` → top-right
   **Sign in** → open the GitHub link → enter the code → avatar appears.
   Click the avatar → **Log out**.
3. **TUI:** with `CORTISOL_SERVER_URL=http://127.0.0.1:8080`, run
   `go run ./cmd/tui` from `server/`. When signed out, press **Enter** to open
   the dashboard. Sign in there, check that the connection code matches your
   terminal, then click **Connect terminal**. The TUI unlocks automatically.
   Set `CORTISOL_DASHBOARD_URL` if the dashboard is not at `http://localhost:5173`.
   Both clients must use the same API. The agent starts only after login succeeds.
   Type `/logout` to sign out in both the TUI and its connected dashboard session.
4. **MCP HTTP backend:** after login, pass the session token:

```bash
export CORTISOL_SESSION_TOKEN="$(jq -r .token ~/.cortisol/credentials)"
export CORTISOL_API_BASE="http://127.0.0.1:8080"
export MCP_BACKEND=http
```

Dashboard sessions are stored in the browser (`localStorage`). TUI sessions are
in `~/.cortisol/credentials` (mode `0600`). After connecting, they share a session
token. Both clients check for revocation every five seconds; the TUI also validates
saved credentials with the API on startup and locks if it cannot verify them.
The signed-in user owns quiz answers; manual `--user-id` selection is no longer used.

Terminal connection links expire after ten minutes and can be claimed once using
a separate secret held only by the TUI. Pending connections are kept in API memory;
after an API restart, press **R** in the terminal to create a new connection.

### 4. Run the HTTP API

Before starting, add the Snowflake account URL, PAT, and model to `server/.env`
using the [Cortex setup guide](server/internal/evaluation/docs/snowflake.md). The server requires
these settings to evaluate prompts. GitHub OAuth env from step 3 is also
required.

In a terminal from the `server/` directory:

```bash
go run ./cmd/server
```

Default listen address: `127.0.0.1:8080` (`HTTP_ADDR`).

### 5. Run the MCP server (agent tools)

In another terminal:

```bash
cd mcp
npm install
npm run dev
```

- Health: `http://127.0.0.1:3100/health`
- MCP endpoint: `http://127.0.0.1:3100/mcp`

Tools (`knowledge_search`, `knowledge_post`) use **mock** knowledge
backends by default. Set `MCP_BACKEND=http` and `CORTISOL_SESSION_TOKEN` (see
step 3) to hit the live knowledge API. For Codex, open `~/.codex/config.toml` and paste

```
[mcp_servers.cortisol]
url = "http://127.0.0.1:3100/mcp"
startup_timeout_sec = 20
tool_timeout_sec = 60
enabled = true
```

### 6. Docker (API + MCP only)

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
`server/` after applying migrations. Seeding is optional for demo data volume;
step 2 already mentions a minimal seed for the default login org.

### 7. Run the TUI

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
export CORTISOL_SERVER_URL=http://127.0.0.1:8080
go run ./cmd/tui
```

When signed out, press Enter to sign in through the dashboard (see step 3). Full MCP steps:
[`docs/codex-mcp.md`](docs/codex-mcp.md). The TUI does not load MCP
itself — `codex app-server` reads that file.

The TUI starts `codex app-server` and uses its JSON-RPC stdio interface. Make
sure the Codex CLI is installed, authenticated, and available on `PATH`.
Type a message and press Enter; use `/quit` or Ctrl+C to exit. For TUI logging
and protocol details, see [`server/cmd/tui/README.md`](server/cmd/tui/README.md).

## Verify minimal knowledge storage

From `server/`, run the offline tests:

```powershell
go test ./internal/knowledge -count=1 -v
go test ./internal/db -run TestKnowledge -count=1 -v
go vet ./...
```

These check validation, exact text/vector serialization, invalid repository
arguments, and the new migration pair. The live repository test prints `SKIP`
unless explicitly configured. If Windows denies access to the default Go cache,
set `$env:GOCACHE = "$PWD\..\.superpowers\go-build"` before running tests.

To exercise actual persistence, ownership rejection, ordering, and MongoDB
validators against a **disposable MongoDB instance**:

```powershell
$env:KNOWLEDGE_TEST_MONGODB_URI = "mongodb://127.0.0.1:27017"
$env:KNOWLEDGE_TEST_DATABASE = "cortisol_test_knowledge"
go test ./internal/knowledge -run TestMongoRepositoryIntegration -count=1 -v
```

The integration test never loads `server/.env`. It creates a uniquely suffixed
database under the specified `cortisol_test_` prefix and drops only that database
afterwards. Its vectors are synthetic storage fixtures.

`go test ./...` and `go run ./cmd/migrate -check` should pass on this branch.
See [API.md](API.md) for the minimal record fields and repository calls.

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
