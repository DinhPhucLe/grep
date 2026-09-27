# Cortisol CLI

## Developer setup

The development setup has three parts: a MongoDB database, the Go HTTP server,
and the Go TUI. Configure the database first, then run the server and TUI in
separate terminals. The TUI currently talks directly to `codex app-server`;
it does not connect to the Go HTTP server.

For team Snowflake access, follow the [Snowflake local development setup](#snowflake-local-development-setup) below.

### Prerequisites

- Go 1.27 or newer
- A MongoDB database (the sample configuration is for MongoDB Atlas)
- The Codex CLI installed and available as `codex` on `PATH` to run the TUI

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

### 3. Run the HTTP server

Before starting, add the Snowflake account URL, PAT, and model to `server/.env`
using the [Cortex setup guide](server/internal/evaluation/docs/snowflake.md). The server requires
these settings to evaluate prompts.

In a terminal from the `server/` directory:

```bash
go run ./cmd/server
```

The server listens on port `8080`. Check that it is running with:

```bash
curl http://localhost:8080/health
```

To load the linked development/demo records, run `go run ./cmd/seed` from
`server/` after applying migrations. Seeding is optional.

### 4. Run the TUI

In another terminal from the repository root:

```bash
cd server
go run ./cmd/tui
```

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

`go test ./...` and `go run ./cmd/migrate -check` currently report the existing
duplicate version-5 migration error. The earlier repair was reverted on request;
the new knowledge migration is verified independently. Full migration deployment
needs a separate resolution of that baseline. See [API.md](API.md) for the
minimal record fields and repository calls.

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
