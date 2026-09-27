<p align="center">
  <img src="docs/assets/grep-cover.png" alt="Grep — The intelligent layer for organizations." width="100%">
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-00ADD8?style=flat&amp;logo=go&amp;logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/React-20232A?style=flat&amp;logo=react&amp;logoColor=61DAFB" alt="React">
  <img src="https://img.shields.io/badge/MongoDB-47A248?style=flat&amp;logo=mongodb&amp;logoColor=white" alt="MongoDB">
  <img src="https://img.shields.io/badge/AWS-232F3E?style=flat&amp;logo=amazonwebservices&amp;logoColor=white" alt="AWS">
  <img src="https://img.shields.io/badge/Snowflake_Cortex-29B5E8?style=flat&amp;logo=snowflake&amp;logoColor=white" alt="Snowflake Cortex">
</p>

---

**Your team’s knowledge. Where you code.**

Grep is a shared knowledge base for engineering teams, bringing organizational
context directly to developers and AI coding agents.

Built for **ShellHacks 2026**.

## The problem

### Your team already solved it. Finding the answer is the hard part.

- **Knowledge is scattered.** Answers are buried in chat threads, outdated docs,
  and a few people’s heads.
- **Experts become bottlenecks.** The same engineers answer the same questions
  while everyone else waits.
- **Context gets lost.** Developers and agents repeat investigations because
  past discoveries never reach the next task.

## 💡 The solution

### One shared memory for your engineering organization.

Grep puts searchable teammate knowledge inside the coding workflow. Retrieve an
answer with its author and code context attached. Contribute a finding while
it is still fresh. Give the next developer, or agent, a head start.

## Three core capabilities

### 1. Capture team knowledge

**Solve it once. Share it with the team.**

Store durable engineering findings as knowledge cards with authors, topics, and
optional repository, module, or file references. Keep the reasoning behind a fix
accessible alongside the context needed to use it.

### 2. Find attributed answers

**Find who knows, and what they know.**

Search organizational knowledge by meaning, beyond exact keyword matches.
Retrieve relevant teammate insights directly in the terminal, with names and
code references attached.

### 3. Give agents shared context

**Your agents should know what your team knows.**

MCP tools let coding agents search and contribute to the same organizational
knowledge base. Make team decisions and discoveries available within the work,
across developers and agent sessions.

## How it works

**Capture → Search → Reuse → Contribute**

An engineer resolves a tricky issue and posts the finding. A teammate encounters
a related problem. Grep retrieves the relevant knowledge, including who
contributed it and where it applies. The teammate uses it and can add new context
for whoever comes next.

Developers search and share in the terminal. Agents use `knowledge_search` and
`knowledge_post` through MCP. Authenticated contributions preserve authorship,
and knowledge is scoped to the organization.

## Who it helps

- **Engineers:** Get unstuck without repeating an investigation.
- **New teammates:** Find the decisions and context behind an unfamiliar system.
- **Technical leads:** Make critical knowledge accessible beyond a few experts.

The goal: faster onboarding, fewer interruptions, and engineering knowledge that
stays with the team as people and projects change.

## Built with

| Technology | What it powers |
| --- | --- |
| **MongoDB Atlas + Voyage embeddings** | Organization-scoped knowledge storage, semantic retrieval, and dashboard aggregation. |
| **MCP** | Shared knowledge search and contribution for coding agents. |
| **AWS** | Hosting for the API, MCP server, and dashboard. |
| **Codex app-server** | AI coding actions inside the terminal workflow. |
| **Snowflake Cortex** | Supporting prompt evaluation and practice features. |
| **Go + React** | The API, terminal interface, and team dashboard. |

## 🚀 What’s next

### Team knowledge that stays useful as the code changes.

- **Richer context across agents:** Extend knowledge reuse across tools and
  sessions, preserving attribution and relevant project context.
- **Connected workplace knowledge:** Bring relevant insights from tools like
  Slack into the shared knowledge loop.
- **Self-maintaining documentation:** Build toward capturing verified findings
  during work and keeping them current as the code evolves.

## 🤝 Build with us

See [contributing.md](contributing.md) for setup and verification, including the
current demo’s authentication and organization behavior.

Explore the [API reference](API.md), [agent connection guide](docs/codex-mcp.md),
and [terminal documentation](server/cmd/tui/README.md).
