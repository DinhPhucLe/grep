# Cortisol CLI — Project context

This document records the current product direction and what exists in the
repo today. Confirmed behavior is described below; planned work and open
choices are marked **WIP**.

## 1. Purpose

Cortisol is becoming a **CLI-native knowledge-sharing medium for software
engineers** in the same organization.

The core job: while someone is building, they can retrieve **attributed**
technical knowledge created by teammates (name + insight + optional code
locus), and they can leave verified knowledge behind for others. Wikis and
chat sit outside the coding loop; Cortisol aims to put org memory where
engineers already interrupt themselves to get unstuck (terminal / TUI).

That direction supersedes positioning Cortisol only as:

- a passive observability / “cortisol signal” dashboard, or
- a product whose *headline* is codebase quizzing.

Quiz / practice (`lead_and_reveal`) remains part of the system as a
**structured engagement and potential write path** into shared knowledge,
not as the primary customer-facing story.

### Product wedge (confirmed direction)

1. **IC first** — unblock mid-task (“who already solved this?”) with author
   attribution.
2. **Org second** — dashboards and coverage / health views for managers once
   usage exists.
3. **Not day-one** — Slack ingest, 3D knowledge maps, and IDE autocomplete
   with name chips are later; they need a working capture → search loop.

## 2. What exists today

### 2.1 Go HTTP server (`server/`)

- MongoDB-backed API (migrations, seed demo cast: NovaPay / AtlasHealth).
- Prompt evaluation via Snowflake Cortex (clarity / ambiguity tooling used by
  the intervention path).
- **Practice events** ingest and aggregation:
  - `POST /api/v1/practice-events`
  - Employee and org practice dashboard GETs under `/api/v1/dashboard/...`
  - Practice currently supported: `lead_and_reveal`
- Directory search (people / org recommendations).
- Practice views can surface **organization and user display names** when
  looked up from Mongo (not raw IDs only in the UI).

### 2.2 Go TUI (`server/cmd/tui`)

- Client of `codex app-server` over JSON-RPC via stdio.
- Owns the intervention UX around Codex: prompt evaluation hooks, quiz /
  reveal sequence, points feedback, approval surfacing.
- Historically does **not** talk to the Go HTTP server for its Codex loop;
  wiring TUI ↔ knowledge API is **WIP**.

### 2.3 Dashboard (`dashboard/`)

- Presentation-only React app. Loads aggregates over HTTP; does not stream
  from the CLI.
- Session / heatmap views plus **employee and organization practice** pages
  (calendar, outcomes, codebase treemaps, outcomes-over-time charts).
- Practice viz polish: readable dates, truncated IDs, named subjects, dynamic
  intensity tiers, denser mocks.

Authoritative API shapes: `API.md`, `dashboard/src/contracts/`.

## 3. Knowledge-sharing pivot (target)

### 3.1 Meaningful CLI loop

A credible eng-knowledge product needs:

1. **Write** — store a knowledge unit with author, org, topic, body, optional
   repo/module/file.
2. **Read** — query with filters + top‑k **vector search**, return results
   with **person attribution**.
3. **Schema** — one `knowledge_cards` (name TBD) model that both API and CLI
   share.

Without write + read, Slack, autocomplete, and graphs are empty.

### 3.2 Near-term build budget

| Size | Scope | Status |
|------|--------|--------|
| **Medium** | HTTP MCP server with real tool schemas (`quack`, `knowledge_search`, `knowledge_post`) + mock knowledge backends; Docker packaging for Go API + MCP; Slack via official Slack MCP (docs/config only) | **Done** — see [`mcp/`](mcp/) and root [`docker-compose.yml`](docker-compose.yml) |
| **Small** | Fixture knowledge corpus for demos | **Done** — [`mcp/fixtures/`](mcp/fixtures/) |
| **Next (other / follow-on)** | Go `GET/POST /api/v1/knowledge` with Atlas vector search; flip MCP from mock → HTTP client | Planned |

**Deferred:** Slack Events bot, full code-embedding index as a second system,
cross-org autocomplete with name chips, 3D knowledge maps.

Practice → auto-publish knowledge card is a natural follow-on once real ingest
and search exist on the Go API.

### 3.3 Schema direction (**WIP** on Go; frozen for MCP)

MCP contract: [`mcp/contracts/knowledge.ts`](mcp/contracts/knowledge.ts).

- `id`, `content`, `topics[]`, `properties{}`, `authors[]`, timestamps,
  `organizationId`
- optional properties for `repo` / `module` / `file_path` / `language`
- Go side still needs collection validator + `embedding[]` + Atlas vector index

### 3.4 MCP surface (agent discovery)

Agents attach to the Cortisol MCP **HTTP** endpoint (`POST /mcp`, default
`:3100`). Tools: `quack` (smoke), `knowledge_search`, `knowledge_post`
(knowledge backends mocked). **Slack tools are not part of Cortisol MCP** —
agents use Slack’s official MCP (`https://mcp.slack.com/mcp`); see
[`docs/slack-mcp.md`](docs/slack-mcp.md).

Expose Cortisol (+ Slack) to the Codex app-server behind our TUI via
[`.codex/config.toml.example`](.codex/config.toml.example) and
[`docs/codex-mcp.md`](docs/codex-mcp.md).

Deploy packaging (two containers only; Atlas external):

```bash
docker compose up --build
```
## 4. Intervention / practice (still in repo)

The TUI intervention flow remains relevant as **how engineers engage with
AI-generated work** and as a generator of structured practice events:

1. User prompts in the TUI; clarity is evaluated in context.
2. Clear prompts → points and normal Codex execution.
3. Consequential ambiguity → gaps identified (Cortex); Codex runs on the
   user’s original instructions.
4. Selected logic is withheld; user answers up to **two** attempts about a
   consequential choice.
5. Points update; **code is revealed either way**.
6. Conversation continues; later prompts re-enter the same flow.

Clarity rubric details, reveal/staging safety when code is already on disk,
and point economics remain partly **WIP** (see historical notes in git /
TUI docs). A correct quiz answer is **not** proof of full understanding.

Research inspiration (not a validation of our point rules): Kazemitabaar et
al., *Exploring the Design Space of Cognitive Engagement Techniques with
AI-Generated Code for Enhanced Learning* (IUI 2025; arXiv:2410.08922).

## 5. Architecture (current)

```text
┌─────────────┐     JSON-RPC/stdio      ┌──────────────────┐
│  Go TUI     │ ◄──────────────────────► │  codex app-server │
└──────┬──────┘                          └──────────────────┘
       │ (practice ingest — knowledge API WIP)
       ▼
┌─────────────┐     HTTP JSON           ┌──────────────────┐
│  Dashboard  │ ◄──────────────────────► │  Go HTTP API     │
└─────────────┘                          │  :8080 + Cortex  │
                                         └────────┬─────────┘
                                                  │
                                         ┌────────▼─────────┐
                                         │  MongoDB Atlas   │
                                         └──────────────────┘

┌─────────────┐   MCP Streamable HTTP   ┌──────────────────┐
│ Agent host  │ ◄──────────────────────► │  cortisol MCP    │
│ Codex/TUI   │                          │  :3100 (mocks)   │
│ Cursor/etc  │ ───────────────────────► │  Slack MCP       │
└─────────────┘                          │  mcp.slack.com   │
                                         └──────────────────┘
```

- **Go API** (`server/cmd/server`): practice, evaluations, dashboard; Atlas via
  `MONGODB_URI`.
- **MCP** (`mcp/`): agent tools for org knowledge (+ `quack` smoke); mock
  knowledge backends today. Slack is the official Slack MCP, configured next
  to Cortisol in Codex/Cursor — not reimplemented here.
- **Compose** packages only those two HTTP services; Atlas stays cloud-hosted.
- **Codex** generates code in the TUI loop; **Cortex** evaluates prompts.

## 6. Positioning guide (for pitches and agents)

**Lead with:** engineering knowledge sharing in the coding loop; attributed
teammate answers.

**Support with:** onboarding ramp, bus factor, senior interrupt load;
startups (few heads hold everything) vs large orgs (scattered knowledge,
same experts overloaded).

**Do not lead with:** general HR attrition theater, “we quiz your codebase,”
or a 3D knowledge graph as the next milestone.

**Category peers (not feature parity claims):** Stack Overflow for Teams /
Stack Internal (eng Q&A), Unblocked (eng context + CLI/MCP), Sourcegraph
(code intel), Copilot/Cursor (generation without org attribution).

## 7. Superseded directions

Do not treat these as the current product center:

- Whole-repo ASCII “future diagram” / pre-execution prompt rewrite as the
  main experience.
- Passive dashboard cortisol / observability metric contract as the north star.
- General-enterprise knowledge management (Confluence replacement) for all
  employees.
- Shipping Slack ingest or 3D cluster viz before `ask` + `add` work.

Older formulas and contracts in the tree may still exist for compatibility;
this file defines **target product intent**.
