# Cortisol CLI

## Developer setup

The development setup has three parts: a MongoDB database, the Go HTTP server,
and the Go TUI. Configure the database first, then run the server and TUI in
separate terminals. The TUI evaluates each prompt through the Go HTTP server
before sending the original text to `codex app-server`.

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
using the [Cortex setup guide](server/docs/snowflake.md). The server requires
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

## Snowflake Local Development Setup

The server also exposes a standalone [`POST /quizzes` endpoint](server/internal/quiz/README.md)
for generating questions from ambiguous prompts and generated code. The TUI
receives up to four questions from `/quizzes` through one Snowflake request,
presents them one at a time with local, ungraded answers, and
opens each question's referenced file and line in the VS Code editor (`code` on `PATH`).
Code remains visible throughout; Ctrl+O cycles through additional references.
Only explicit implementation requests receive an ambiguity score. Confirmations
and other conversation return a null score and go directly to Codex.
Null-score results are not saved; no database migration is needed for this flow.

These instructions cover team Snowflake access for local development. This branch does not yet include the Snowflake connection checker referenced below; the account and user setup can be completed independently.

### Connection model

```text
Go application
  -> individual PAT
  -> individual Snowflake user
  -> APP_DEV role
  -> SHELLHACK_API.PUBLIC
  -> COMPUTE_WH
```

Each developer must use their own Snowflake user and programmatic access token (PAT). The team shares the account identifier, database, schema, warehouse, role, and authentication policy.

Never share a PAT or commit a `.env` file.

### Shared and personal settings

| Setting | Shared by the team? |
| --- | --- |
| Snowflake account identifier | Yes |
| Database and schema | Yes |
| Warehouse | Yes |
| `APP_DEV` role | Yes |
| Authentication policy | Yes |
| Username | No |
| PAT secret | No |
| `.env` file | No |

### One-time administrator setup

Run the following statements from a Snowsight SQL worksheet using the `ACCOUNTADMIN` role. Run statements individually so errors are easy to identify.

#### Create the shared role and grant access

```sql
USE ROLE ACCOUNTADMIN;

CREATE DATABASE IF NOT EXISTS SHELLHACK_API;
CREATE ROLE IF NOT EXISTS APP_DEV;

GRANT ROLE APP_DEV TO ROLE SYSADMIN;

GRANT USAGE ON WAREHOUSE COMPUTE_WH TO ROLE APP_DEV;
GRANT USAGE ON DATABASE SHELLHACK_API TO ROLE APP_DEV;
GRANT USAGE ON SCHEMA SHELLHACK_API.PUBLIC TO ROLE APP_DEV;

GRANT SELECT, INSERT, UPDATE, DELETE
  ON ALL TABLES IN SCHEMA SHELLHACK_API.PUBLIC
  TO ROLE APP_DEV;

GRANT SELECT, INSERT, UPDATE, DELETE
  ON FUTURE TABLES IN SCHEMA SHELLHACK_API.PUBLIC
  TO ROLE APP_DEV;

GRANT SELECT
  ON ALL VIEWS IN SCHEMA SHELLHACK_API.PUBLIC
  TO ROLE APP_DEV;

GRANT SELECT
  ON FUTURE VIEWS IN SCHEMA SHELLHACK_API.PUBLIC
  TO ROLE APP_DEV;
```

If developers need to create tables or views, also run:

```sql
GRANT CREATE TABLE, CREATE VIEW
  ON SCHEMA SHELLHACK_API.PUBLIC
  TO ROLE APP_DEV;
```

#### Create the local-development authentication policy

This policy permits PAT authentication without requiring every developer to maintain a Snowflake network policy. If a developer already has a network policy, Snowflake still enforces it.

```sql
CREATE DATABASE IF NOT EXISTS SECURITY_CONFIG;
CREATE SCHEMA IF NOT EXISTS SECURITY_CONFIG.POLICIES;

CREATE AUTHENTICATION POLICY IF NOT EXISTS
  SECURITY_CONFIG.POLICIES.LOCAL_DEV_PAT_POLICY
PAT_POLICY = (
  NETWORK_POLICY_EVALUATION = ENFORCED_NOT_REQUIRED
);
```

Apply this policy only to local development users, not to the entire Snowflake account.

### Add a developer

Create a separate `TYPE=PERSON` Snowflake user for each developer. Do not share an administrator account.

Run the following statements as an administrator, replacing `TEAMMATE_USERNAME` with the actual Snowflake username:

```sql
GRANT ROLE APP_DEV TO USER TEAMMATE_USERNAME;

ALTER USER TEAMMATE_USERNAME SET DEFAULT_ROLE = APP_DEV;
ALTER USER TEAMMATE_USERNAME SET DEFAULT_WAREHOUSE = COMPUTE_WH;
ALTER USER TEAMMATE_USERNAME
  SET DEFAULT_NAMESPACE = 'SHELLHACK_API.PUBLIC';

ALTER USER TEAMMATE_USERNAME
  SET AUTHENTICATION POLICY
  SECURITY_CONFIG.POLICIES.LOCAL_DEV_PAT_POLICY;
```

Verify the role grants:

```sql
SHOW GRANTS TO ROLE APP_DEV;
```

### Generate a personal PAT

Each developer signs into their own Snowsight account and runs:

```sql
USE ROLE APP_DEV;

ALTER USER ADD PROGRAMMATIC ACCESS TOKEN GO_APP_DEV
  ROLE_RESTRICTION = 'APP_DEV'
  DAYS_TO_EXPIRY = 30;
```

Snowflake displays the token secret only when it is created. Copy it immediately into the developer's local `.env` file. Do not send it through chat or commit it to Git.

### Configure the local environment

Use the same `server/.env` file configured for MongoDB above. If you have not created it yet, run this from the `server` directory:

```powershell
Copy-Item .env.example .env
```

Add the following settings to `.env`, preserving your MongoDB settings. These Snowflake keys are not yet included in this branch's `.env.example`:

```dotenv
# Shared values
SNOWFLAKE_ACCOUNT=your-organization-your-account
SNOWFLAKE_DATABASE=SHELLHACK_API
SNOWFLAKE_SCHEMA=PUBLIC
SNOWFLAKE_WAREHOUSE=COMPUTE_WH

# Personal values
SNOWFLAKE_USER=your_username
SNOWFLAKE_TOKEN=your_private_pat
```

The account identifier should use the `organization-account` format. Do not include `https://` or `.snowflakecomputing.com`.

### Test the connection

The following command requires `server/cmd/snowflake-check`, which is not present in this branch. Once that command is available, run it from the `server` directory:

```powershell
go run .\cmd\snowflake-check\
```

Expected output:

```text
Connected to Snowflake!
```

The setup guide describes a checker that opens a database handle, calls `PingContext` to authenticate, and closes the handle when the command exits. Verify that behavior when the checker is added to this branch.

### Authentication-only test

To test PAT authentication without selecting application resources, temporarily leave these values blank:

```dotenv
SNOWFLAKE_DATABASE=
SNOWFLAKE_SCHEMA=
SNOWFLAKE_WAREHOUSE=
```

Restore the values after confirming authentication.

### Problems encountered during initial setup

#### HTTP 404 or error `261004`

Cause: an incorrect Snowflake account identifier produced the wrong Snowflake hostname.

Fix: use the value returned by:

```sql
SELECT CURRENT_ORGANIZATION_NAME() || '-' || CURRENT_ACCOUNT_NAME();
```

#### Error `390432`: Network policy is required

Cause: Snowflake's default PAT policy required the user to have a network policy.

Fix: assign `LOCAL_DEV_PAT_POLICY` to the development user, or temporarily configure a PAT bypass for a human user.

#### Error `390201`: Database does not exist or is not authorized

Cause: authentication succeeded, but the PAT session used the `PUBLIC` role, which lacked access to `SHELLHACK_API`.

Fix: grant `APP_DEV` to the user and create a new PAT with `ROLE_RESTRICTION = 'APP_DEV'`.

### Team security rules

- Give every developer a separate Snowflake user and PAT.
- Keep `.env` ignored by Git.
- Commit only `.env.example` with placeholder values.
- Restrict PATs to `APP_DEV`.
- Recreate or rotate PATs before they expire.
- Revoke a developer's PAT and role access when they leave the project.
- Do not use local developer PATs for deployment.

For deployment, use a dedicated service identity, a restricted production role, and a network policy for the deployment environment.

### References

- [Using programmatic access tokens](https://docs.snowflake.com/en/user-guide/programmatic-access-tokens)
- [Configure clients and drivers](https://docs.snowflake.com/en/user-guide/gen-conn-config)
- [Configure Snowflake access control](https://docs.snowflake.com/en/user-guide/security-access-control-configure)
- [Authentication policies](https://docs.snowflake.com/en/user-guide/authentication-policies)
