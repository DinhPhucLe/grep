# Cortex evaluation backend

The Go server sends prompt/context JSON to Snowflake Cortex over HTTPS, validates
its structured answer, stores the request and evaluation in MongoDB, and returns
the saved evaluation. No Snowflake database, table, warehouse, or Go SQL driver
is needed for this REST integration.

## Backend layout

```text
cmd/server/main.go                    configuration and dependency wiring
internal/cortex/config.go            Snowflake environment configuration
internal/cortex/client.go            authenticated Cortex HTTP client
internal/evaluation/model.go         request, response, and validation rules
internal/evaluation/prompt.go        versioned clarity rubric and JSON schema
internal/evaluation/service.go       classify, validate, persist all evaluations
internal/evaluation/repository.go    MongoDB evaluations collection
internal/evaluation/handler.go       HTTP request parsing and error mapping
internal/db/migrations/000004_*      evaluations schema and timestamp index
internal/db/migrations/000007_*      nullable scores for non-implementation messages
internal/evaluation/examples/evaluation.json  runnable request payload
```

The evaluation contract implements the prompt-clarity step from `CONTEXT.md`.
It considers prior conversation and supplied repository context. It does not
assign points, generate quizzes, grade answers, or run Codex. Those product
contracts remain separate work. The model's judgment is not a validated measure
of user understanding. The TUI calls this endpoint before forwarding messages to Codex.

## 1. Configure Snowflake

In Snowsight, open the account menu, choose **View Account Details**, and copy
the account URL, such as `https://org-account.snowflakecomputing.com`. Do not
use the `app.snowflake.com` browser URL.

An administrator can run the following in a SQL worksheet. Replace `YOUR_USER`
with the user that will own the token (for example, `PHUCLE1309`):

```sql
USE ROLE ACCOUNTADMIN;
CREATE ROLE IF NOT EXISTS CORTISOL_API;
GRANT DATABASE ROLE SNOWFLAKE.CORTEX_REST_API_USER TO ROLE CORTISOL_API;
GRANT ROLE CORTISOL_API TO USER YOUR_USER;
ALTER USER YOUR_USER SET DEFAULT_ROLE = CORTISOL_API;
```

Create a programmatic access token (PAT) for that user, restricted to
`CORTISOL_API`. The SQL form is:

```sql
ALTER USER YOUR_USER ADD PROGRAMMATIC ACCESS TOKEN CORTISOL_BACKEND
  ROLE_RESTRICTION = 'CORTISOL_API'
  DAYS_TO_EXPIRY = 30;
```

Save the token returned by Snowflake in your private environment file. Under
the default PAT policy, a network policy must allow the caller's public outbound
IP. Have your administrator configure that for local development and the deployed
server. If an authentication policy restricts login methods, it must permit PATs.
Do not disable account network protection as a workaround.

Choose a model supporting JSON schema output that is available to your account.
The example uses `claude-sonnet-4-6`. Model availability depends on region and
account settings; an administrator may need to enable an appropriate cross-region
inference setting if your chosen model is not local.

Official references:

- [Cortex API, permissions, models, and structured output](https://docs.snowflake.com/en/user-guide/snowflake-cortex/cortex-rest-api)
- [PAT creation and network policy requirements](https://docs.snowflake.com/en/user-guide/programmatic-access-tokens)

## 2. Configure the Go server

Keep your existing MongoDB configuration in `server/.env`. Add these variables
(the `.env.example` file contains the full configuration template):

```dotenv
SNOWFLAKE_ACCOUNT_URL=https://org-account.snowflakecomputing.com
SNOWFLAKE_PAT=your-private-token
SNOWFLAKE_MODEL=claude-sonnet-4-6
EVALUATION_TIMEOUT=60s
HTTP_ADDR=127.0.0.1:8080
```

All three Snowflake variables are required. `EVALUATION_TIMEOUT` defaults to
60 seconds and accepts positive Go durations up to 5 minutes. It covers inference and storage. The model receives at most 4096 output tokens.
The server validates configuration at startup, but credentials and model access
are only verified by a real evaluation call.

The server binds to loopback by default. The local API requires no application
login or session token. Calls spend Snowflake credits and the payload may contain
source code; keep this development API on loopback.
Snowflake provider tokens stay in the backend and are never sent to a browser or logged. Input, context, and conversation ARE stored in MongoDB;
apply your application's retention/access rules to the evaluations collection.

## 3. Apply migrations and start

From `server/`, validate files without connecting to MongoDB:

```sh
go run ./cmd/migrate -check
```

The fourth migration creates `evaluations` with schema validation and a timestamp
index. Migration 7 allows `not_applicable` with a null score and must be applied
before storing those evaluations. Get explicit approval before executing migrations;
`go run ./cmd/migrate` applies every pending migration, including the separate
quiz-answer migration 8. The server does not apply migrations at startup.
Migration 4's down migration drops the collection and its data. Migration 7's down
migration restores the numeric-only validator without converting or deleting records;
rollback after null-score records exist needs a separate approved data-handling plan.

## 4. Send an evaluation

From `server/` in another terminal:

```sh
curl --fail-with-body http://127.0.0.1:8080/evaluations \
  -H 'Content-Type: application/json' \
  --data-binary @internal/evaluation/examples/evaluation.json
```

`POST /evaluations` accepts:

- `input`: required nonempty prompt; at most 32,000 bytes.
- `context`: optional repository/task context; at most 128,000 bytes.
- `conversation`: optional chronological array of up to 50 `{role, content}`
  entries. Roles are `user` or `assistant`, with nonempty content.

Combined text is capped at 256,000 bytes, and the HTTP body at 1 MiB. Unsupported
fields, trailing JSON values, and invalid conversation entries are rejected. The
backend does not fetch files or previous conversation automatically; send relevant
context explicitly. Submitted conversation is evaluation data, not system messages.

A successful call returns HTTP **201**. Example (the judgment varies by input/model):

```json
{
  "id": "66f600000000000000000001",
  "created_at": "2026-09-26T20:00:00Z",
  "model": "claude-sonnet-4-6",
  "rubric_version": "implementation-specificity-v3",
  "evaluation": {
    "verdict": "ambiguous",
    "summary": "The affected login behavior is unspecified.",
    "ambiguity_score": 0.78,
    "gaps": [
      {
        "description": "No failing scenario or expected outcome is provided.",
        "consequence": "Implementation could change password validation when the actual problem is session expiry."
      }
    ]
  }
}
```

The evaluator classifies intent before scoring. Non-implementation messages such
as confirmations, explanations, option selections, and status checks return:

```json
{"verdict":"not_applicable","summary":"Confirmation of an existing plan","ambiguity_score":null,"gaps":[]}
```

The score field is required even when null. Numeric scores apply only to explicit
implementation/build/change requests. `clear` requires a score at or below 0.30
and no gaps; `ambiguous` requires a score above 0.30 and 1–10 consequential gaps.
The specificity rubric considers concrete outcomes and scope, relevant implementation
approach/constraints, and applicable examples or reusable code references. It uses
details already supplied in conversation, without inventing repository facts or
requiring unnecessary detail for narrow edits. Missing information must affect
behavior, compatibility, or integration to count as a gap. Higher scores indicate
more ambiguity. Quiz questions remain about client-visible behavior.
A null score is accepted only with `not_applicable` and no gaps. The TUI sends
unrated messages directly to Codex without an evaluation card or quiz.
Both numeric and null-score evaluations are saved and return HTTP 201 with their
database ID. Null is stored explicitly, not replaced with zero. Migration 7 is
required for the nullable MongoDB validator.
For all evaluations, the original payload is persisted but omitted from the
HTTP response, which is sent only after MongoDB acknowledges the insert. There is no retrieval
endpoint yet; the returned ID identifies the MongoDB record.

Only `POST /evaluations` is supported; the legacy `/jobs` test alias is removed.

## Errors and operating behavior

Errors have the shape `{"error":{"code":"...","message":"..."}}`:

| HTTP | Meaning |
| --- | --- |
| 400 | Invalid payload or text limits exceeded |
| 405 | Use POST |
| 413 | HTTP body exceeds 1 MiB |
| 415 | Content-Type must be application/json |
| 502 | Cortex transport/permissions (`cortex_error`), invalid model output (`cortex_invalid_response`), or network-policy auth failure |
| 504 | Evaluation deadline exceeded |
| 500 | Evaluation could not be saved or another internal error |

Calls execute directly without a worker queue. Cancellation is propagated to Cortex and MongoDB.
No automatic retries are made, since a repeated inference can incur another charge.
Client retries may create duplicate records, and a disconnect after storage may
leave a saved evaluation the client did not receive. There is no idempotency key yet.
Provider response bodies and submitted prompts are not included in error responses.
`/health` is a liveness endpoint; it does not test Cortex availability.

For a 502, check PAT expiry, the user's role, allowed outbound IP, model availability,
and account Cortex settings. A direct request using the official Cortex quickstart
can reveal the provider's specific error without exposing it through this API.

## Verification

```sh
go test ./...
go test -race ./internal/cortex ./internal/evaluation
```

Tests use a local TLS Cortex stub and an in-memory repository; they do not spend
credits or connect to Atlas. A live test requires your configured Snowflake account,
MongoDB migrations, and the curl request above.

## Fix: Snowflake code 390432 (network policy required)

The backend reports `cortex_network_policy_required` when Snowflake refuses PAT
login because the token owner is not subject to a network policy. In a Snowsight
SQL worksheet, select ACCOUNTADMIN and run:

```sql
USE ROLE ACCOUNTADMIN;
SELECT CURRENT_IP_ADDRESS();
```

When your browser and backend run from the same network without different VPN/proxy
routes, this is the public IP to allow. Replace `YOUR_PUBLIC_IPV4` below with the
returned IPv4 address and `YOUR_USER` with the PAT owner, such as `PHUCLE1309`:

```sql
CREATE NETWORK POLICY CORTISOL_DEV_ACCESS
  ALLOWED_IP_LIST = ('YOUR_PUBLIC_IPV4/32')
  COMMENT = 'Cortisol development access';

ALTER USER YOUR_USER SET NETWORK_POLICY = CORTISOL_DEV_ACCESS;
```

This policy restricts all sign-ins for that user to the listed IPs. If the browser
and backend have different outbound IPs, include both in the allowed list. If the
policy already exists, inspect it and update its allowed list instead of replacing
it blindly. Changing networks or deploying the backend may require updating the
allowed IPs. Keep the PAT in `.env`, restart the Go server to load code changes,
and retry the sample curl request. The existing PAT need not be regenerated for
this policy change.

[Snowflake network policy documentation](https://docs.snowflake.com/en/user-guide/network-policies)
