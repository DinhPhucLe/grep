# Cortisol CLI

## Developer setup

The development setup has three parts: a MongoDB database, the Go HTTP server,
and the Go TUI. Configure the database first, then run the server and TUI in
separate terminals. The TUI currently talks directly to `codex app-server`;
it does not connect to the Go HTTP server.

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
go run ./tui
```

The TUI starts `codex app-server` and uses its JSON-RPC stdio interface. Make
sure the Codex CLI is installed, authenticated, and available on `PATH`.
Type a message and press Enter; use `/quit` or Ctrl+C to exit. For TUI logging
and protocol details, see [`tui/README.md`](tui/README.md).
