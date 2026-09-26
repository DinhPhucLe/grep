# Apply MongoDB migrations

From `server/`:

```sh
go run ./cmd/migrate -check
go run ./cmd/migrate
```

The first command checks file structure and JSON syntax without connecting to
Atlas. It does not validate MongoDB command semantics. The second applies pending
up migrations using golang-migrate. The HTTP server does not run them itself.

The command loads `server/.env` when launched from `server/`, and uses the existing
`MONGODB_URI`, `MONGODB_DATABASE`, and optional `DB_USERNAME`/`DB_PASSWORD` placeholder
values. Existing process environment variables take precedence. Credentials are
not passed as command-line arguments. Allow your current IP in Atlas and ensure
the database user can create collections/indexes and read/write migration records.

From the repository root, an equivalent command is:

```sh
cd server
go run ./cmd/migrate -env .env -path db/migrations
```

The target database is selected by `MONGODB_DATABASE`, independently of the URI's
database path. Inspect `users`, `projects`, and `sessions` in Atlas Data Explorer
afterward. golang-migrate also maintains `schema_migrations` and
`migrate_advisory_lock`. Running again with no pending migrations is a success.

Migrations execute commands separately, so a failed migration may have partially
applied changes. If the version is dirty, inspect the error and database before
repairing it; this command never automatically forces a version or drops data.
Do not edit already applied migrations. Add a new numbered up/down pair instead.

The down files drop collections and delete their data. This command intentionally
exposes only up migrations. Seeding is separate.

golang-migrate v4.20.1 uses MongoDB's v1 Go driver internally. The migration command
uses that compatible client; the application's existing connection remains on v2.
