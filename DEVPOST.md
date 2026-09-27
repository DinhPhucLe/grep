# Cortisol

Org knowledge where engineers already work — the terminal.

Wikis and Slack sit outside the coding loop. Cortisol puts attributed teammate answers (who said it, what they learned, optionally where in the code) into the same place people get stuck: a CLI/TUI wired to coding agents.

## Why

Every team has a few people who carry the real mental model of the system. When they're busy, offline, or gone, everyone else re-derives the same answers from chat history and half-updated docs. That is bus factor as a daily tax, not a disaster scenario.

Cortisol is searchable org memory with names attached. Agents and humans can pull teammate knowledge mid-task and leave verified cards behind when they figure something out — so the next engineer does not start from zero.

## How it works

Sign in with GitHub (device flow) from the dashboard or TUI. Point Codex at the Cortisol MCP endpoint so the agent can call `knowledge_search` on its own when you're blocked, or hit `/search-learning` yourself in the TUI. When you learn something durable, post it with attribution via `knowledge_post` or `/send-learning` — topic, body, optional repo/module/file locus. Snowflake Cortex scores ambiguous prompts and can generate practice quizzes so the team stays sharp on material that already lives in the system. The React dashboard shows practice heatmaps and knowledge connects: who linked what to what, and which topics keep showing up together.

## Design and technologies

The stack is a Go HTTP API, a Go TUI client of Codex app-server, an HTTP MCP server for agent tools (`quack`, `knowledge_search`, `knowledge_post`), and a React dashboard for org views. Knowledge lives as org-scoped cards with authors, topics, and optional code locus. Codex owns coding actions; Cortisol owns evaluation, review, and the knowledge loop around them. Auth is GitHub OAuth device flow; sessions gate search and post so attribution stays real.

## Why Snowflake and MongoDB

**Snowflake.** We use Cortex for the judgment calls that sit next to coding — prompt clarity/ambiguity evaluation and quiz generation — not as a generic chatbot wrapper. That means one high-availability frontier LLM path for interventions that need structured output under a deadline, with networking and decode cost measurable separately from Mongo write latency. The product decision was deliberate: keep the coding agent on Codex, and put Snowflake where we need reliable, API-shaped language judgment.

**MongoDB.** Atlas is the system of record for sessions, practice events, and knowledge cards. Automated Embedding (voyage-code-4) builds vectors on content so `$vectorSearch` can return attributed hits without us running a separate embedding service. Aggregation pipelines power employee and org dashboard views — calendars, outcomes, topic co-occurrence — without shipping raw collections to the browser. Deploying on Atlas kept the hackathon path short: one cloud database, migrations and seed for demo orgs, no local Mongo to babysit.

## What's next

**Shared context between AI agents.** Today each agent session is mostly alone with whatever it scraped from chat. Cortisol's knowledge store is meant to become the shared layer: one agent posts a verified finding, another agent in a different session searches it with the author's name still attached. Org memory as coordination, not another siloed transcript.

**Self-maintaining documentation.** Wikis rot because writing them is a separate chore from shipping. The loop we want is: you debug something hard, you post the card while the context is still hot, the next person (or agent) finds it where they already work. Documentation that grows from verified work instead of from someone remembering to update Confluence.
