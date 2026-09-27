# Agent notes (Cortisol)

## Org knowledge (MCP `cortisol`)

This repo ships an org knowledge MCP. When it is connected, treat `knowledge_search` as the default way to retrieve **team/org memory that is not guaranteed to live in the local codebase**.

Call `knowledge_search` without waiting for the user to name the tool, say “MCP”, or say “skill”, when they:

- ask to search the knowledge base / knowledge database / org memory
- ask how **we / the team / the org** handle something
- want answers **outside** this repo, codebase, or checked-out files
- are stuck on an internal convention, service, or failure mode another engineer may already have written down

Prefer `knowledge_search` before guessing org-specific behavior or assuming the answer is only in local files.

Use `knowledge_post` only for durable, non-secret insights worth sharing. Use `quack` only as a connectivity smoke test.
