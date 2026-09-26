# Vibe Coding Observatory — Project context

Hackathon MVP decisions and metric contract. Treat these as constraints unless explicitly changed. Do not expand scope with extra behavioral scores, interventions, platforms, or metrics.

## 1. Project description

The product is a terminal-based observability layer for AI coding sessions. The developer works through **our TUI**, which is a client of **Codex app-server**. The TUI records live session traffic, derives a small set of metrics, and presents them in a dashboard.

The goal is to detect and visualize evidence associated with **vibe coding**:

> Overreliance on AI without active human control of the engineering process. The user lets AI perform planning, design, implementation, testing, or expected-behavior decisions without demonstrating meaningful direction, evaluation, or ownership.

Motivated by Kazemitabaar et al., *Exploring the Design Space of Cognitive Engagement Techniques with AI-Generated Code for Enhanced Learning* (IUI 2025; arXiv:2410.08922):

- **Automation bias:** favoring automated recommendations even against contradictory evidence.
- **Cognitive offloading:** using an external aid to reduce one's own cognitive effort.

Telemetry cannot prove internal understanding. We report observable session behavior and a qualitative interpretation. Every metric should add insight relevant to those categories.

## 2. Architecture direction

### 2.1 TUI as Codex app-server client

Codex app-server is the process that runs the Codex agent and performs coding actions. Codex CLI is only OpenAI’s native client of that server. We build **our own client** in Go (`tui/`) that:

- Spawns / connects to `codex app-server` (JSON-RPC over stdio)
- Renders chat history
- Sends user prompts as turns
- Streams agent responses back to the user
- Collects approvals and other user input when the server requests them

Agentics stay on app-server. The TUI does not reimplement the agent loop or patch application. Protocol detail (threads, turns, items, approvals) lives in `tui/README.md`. Regenerate API schemas locally with:

```bash
codex app-server generate-ts --out ./schemas
```

### 2.2 “Non-invasive” means UX, not invisibility

We are **in the user’s path**: they code through our TUI instead of stock Codex CLI. That is intentional so we can observe the live session accurately.

Non-invasive here means **user experience**:

- Do not block, delay, rewrite, or invent friction that hinders getting work done.
- Approvals and prompts the agent already requires still surface; we pass them through rather than silently deciding for the user in the product path.
- The MVP observes and visualizes; it does not intervene in engineering decisions or police the developer.
- A playful **cortisol** metaphor may visualize significant metrics; it is not a physiological claim.
- Descriptive dashboard numbers and the code-change heatmap do not drive the cortisol signal.

### 2.3 Data source

Primary live source: JSON-RPC traffic between our TUI and Codex app-server (prompts, items, approvals, completions, timestamps, token metadata when exposed). Filesystem/environment metrics are later consideration.

Raw events feed metric functions; aggregations feed the dashboard. Metric calculation stays independent of dashboard layout.

### 2.4 Non-overlapping significant metrics

Each significant metric answers a distinct question:

- Acceptance ratio — how often shown suggestions are accepted
- Median time to approval — how quickly accepted suggestions are approved
- Post-edit response time — how quickly the user re-engages after an agent edit
- Qualitative session summary — what the session shows in context

Do not add duplicates such as both acceptance rate and rejection rate as separate significant metrics. Raw accept/reject counts may still appear as descriptive numbers.

### 2.5 Hackathon MVP discipline

- Smallest feature that satisfies the explicit request
- One agent stack first (Codex app-server); no multi-platform design ahead of need
- No prediction models, long-term skill claims, intervention agents, or unrequested scoring formulas
- Prefer mock data and stable interfaces before production ingestion where helpful

## 3. Abstract input vocabulary

Raw event schema is still evolving. Formulas below use abstract observations. Implementation may rename fields but must preserve meaning. Concrete wire shapes come from app-server schemas and live RPC (e.g. approval requests → `suggestion_shown` / decisions; completed `fileChange` items → `agent_edit_completed`).

## 4. Dashboard metrics

### 4.1 Descriptive “small numbers”

Enrich the dashboard only. Never update the cortisol signal directly.

#### Estimated active time

**Question:** How long did the user appear actively engaged with the session?

```text
active_time = Σ min(time(user_activity[i+1]) - time(user_activity[i]), G)
```

MVP default: `G = 1 minute`. Label **Estimated active time**. Long agent execution or an idle terminal must not count as user activity by itself.

#### Total files edited

```text
total_files_edited = count(distinct file paths changed from session baseline)
```

#### Final added lines

Net additions in the current repo diff vs session-start baseline:

```text
final_added_lines = count(addition lines in diff(session_baseline, current_state))
```

#### Total model requests

```text
total_model_requests = count(model_request)
```

Distinct from user prompts: one turn may include multiple model requests.

#### Total plans

```text
total_plans = count(distinct explicit plan_id values)
```

Updates to the same `plan_id` do not increase the count.

#### Total acceptances / rejections

```text
total_acceptances = count(suggestion_accepted)
total_rejections = count(suggestion_rejected)
```

#### Total user prompts

```text
total_user_prompts = count(user_prompt)
```

#### Average / median prompt length

Unicode character count unless a reliable tokenizer already exists.

```text
average_prompt_length = sum(length(prompt_text)) / total_user_prompts
median_prompt_length = median(length(prompt_text) for each user_prompt)
```

Return `null` when there are no user prompts.

#### Output tokens

```text
output_tokens = sum(provider-reported output tokens across agent responses)
```

Return `unknown` when the provider does not expose usage — do not estimate.

### 4.2 Code-line change heatmap

Explains what happened to lines touched in the session. Does not update cortisol.

- **Green:** introduced or changed in-session and still present
- **Red:** touched then deleted or replaced
- **Darker red:** more repeated deletion/replacement churn

Needs session-start baseline, current state, and intermediate edit snapshots if churn intensity is shown. Without intermediates, render only baseline→current diff; do not invent churn from a final diff alone.

## 5. Significant metrics

Only these feed the client-side cortisol visualization.

### 5.1 Acceptance ratio

**Question:** What portion of shown actionable suggestions did the user explicitly approve?

```text
acceptance_ratio = total_acceptances / count(suggestion_shown)
```

Return `null` when nothing was shown. Keep shown / accepted / rejected / unresolved counts for sample size.

### 5.2 Median time to approval

For each accepted suggestion `s`:

```text
approval_latency(s) = accepted_at(s) - shown_at(s)
median_time_to_approval = median(approval_latency(s) for accepted suggestions)
```

May render as a histogram. Return `null` when nothing was accepted. Measures speed of approval, not correctness.

### 5.3 Median active post-edit response time

**Question:** After the agent finishes an edit, how much active-session time passes before the user’s next observable response?

```text
post_edit_response_time(e) = active elapsed time from edit_completed_at(e)
                             to the next user_activity

median_active_post_edit_response_time = median(post_edit_response_time(e))
```

Same active-time / inactivity rule as estimated active time. Next response may be a prompt, acceptance, or rejection. Exclude edits with no later user action; report them as pending. Distinct from approval latency: re-engagement after a completed file edit, with or without an approval event.

### 5.4 Qualitative LLM session summary

Short bullet list of observable evidence — not a diagnosis of the user.

Cover whether the user supplied goals/constraints, showed direction, questioned or redirected AI output, asked for explanation/verification, and note uncertainty or missing telemetry. Do not infer comprehension from accepted code, tone, prompt length, or tool volume alone. Allow `unknown` when evidence is thin. Dashboard needs the bullet summary; richer evidence IDs can come later.

## 6. Cortisol signal boundary

The client may show a live “cortisol level” style alert from the server. No composite formula or thresholds are approved. Do not invent weights or scientific probabilities. Until designed, present raw signals or mock display states without physiological claims.

Descriptive numbers and the heatmap stay dashboard-only and must not drive the cortisol visualization.
