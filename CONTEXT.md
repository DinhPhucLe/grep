# Vibe Coding Observatory — Decisions and Metric Contract (v1)

Status: first implementation context for the hackathon MVP.

This document records decisions already made. Treat them as constraints unless the user explicitly changes them. Do not expand the scope with additional behavioral scores, interventions, platforms, or metrics.

## 1. Project description

The project is a terminal-based observability application that sits on top of AI coding-agent CLIs. It records activity exposed by the wrapped session, derives a small set of metrics, and presents them in a dashboard.

The goal is to detect and visualize evidence associated with **vibe coding**.

For this project, vibe coding means:

> Overreliance on AI without active human control of the engineering process. The user lets AI perform planning, design, implementation, testing, or expected-behavior decisions without demonstrating meaningful direction, evaluation, or ownership.

This definition is motivated by Kazemitabaar et al., *Exploring the Design Space of Cognitive Engagement Techniques with AI-Generated Code for Enhanced Learning* (IUI 2025; arXiv:2410.08922). The paper frames the problem using two related concepts:

- **Automation bias:** a tendency to favor automated recommendations, including when contradictory evidence exists.
- **Cognitive offloading:** using an external aid to reduce one's own cognitive effort.

The project does not claim that telemetry can prove a person's internal understanding. It reports observable session behavior and a qualitative interpretation. Every metrics we add should try to meaningfully add an insight relevant to these categories

## 2. Philosophy and architecture guidelines

### 2.1 Non-invasive observation

- The application sits between the user and an AI-agent CLI.
- For now we only collect CLI / session sourced metrics such as observes available prompts, responses, suggestion decisions, plan events, token metadata, file changes, and timestamps. Metrics sourced from environment / filesystem such as file metadata etc are later consideration.
- It must not block, delay, rewrite, or otherwise hinder the developer's interaction with the coding agent.
- The MVP is observation and visualization only.
- A playful **cortisol level** may visualize the significant metrics, but it is a product metaphor, not a physiological measurement.
- Descriptive dashboard numbers and the code-change heatmap do not affect the cortisol signal.

### 2.2 Non-overlapping metrics

Each significant metric must answer a distinct question not already answered by another significant metric:

- Acceptance ratio: how frequently suggestions are accepted.
- Median time to approval: how quickly accepted suggestions are approved.
- Post-edit response time: how quickly the user re-engages after an agent edit.
- Qualitative session summary: what the session demonstrates in context.

Do not add complementary duplicates such as both acceptance rate and rejection rate as separate significant metrics. Raw acceptance and rejection counts may still appear as descriptive supporting numbers.

### 2.3 Hackathon MVP discipline

- Always choose the smallest feature that satisfies the explicit request.
- Do not design for multiple agent platforms before one adapter works.
- Do not introduce prediction models, long-term skill claims, intervention agents, blocking workflows, or unrequested scoring formulas.
- Prefer mock data and stable interfaces before production ingestion.
- Keep metric calculation independent from dashboard presentation because metric definitions and layout will change during the hackathon.

## 3. Abstract input vocabulary

Data collection are still in dev so raw events data model are yet unspecified. The formulas below use abstract observations rather than a concrete event schema. Implementation may rename fields, but it must preserve their meaning.

## 4. Dashboard metrics

### 4.1 Descriptive “small numbers”

These enrich the dashboard and are left to user interpretation. They must never directly update the cortisol signal.

#### Estimated active time

**Question:** How long did the user appear to be actively engaged with the wrapped session?

```text
active_time = Σ min(time(user_activity[i+1]) - time(user_activity[i]), G)
```

Recommended MVP default: `G = 1 minutes`. Label the result **Estimated active time**. Long agent execution or an idle open terminal must not automatically count as user activity.

#### Total files edited

```text
total_files_edited = count(distinct file paths changed from session baseline)
```

#### Final added lines

The number of added lines present in the current repository diff relative to the session-start baseline:

```text
final_added_lines = count(addition lines in diff(session_baseline, current_state))
```

This is a net/current-state statistic, not cumulative editing effort. Repeatedly adding and deleting the same line does not increase it if the line is absent at computation time.

#### Total model requests

```text
total_model_requests = count(model_request)
```

This is distinct from total user prompts because an agent may make multiple model requests during one user turn.

#### Total plans

```text
total_plans = count(distinct explicit plan_id values)
```

Plan updates to the same `plan_id` do not increase the count.

#### Total acceptances

```text
total_acceptances = count(suggestion_accepted)
```

#### Total rejections

```text
total_rejections = count(suggestion_rejected)
```

#### Total user prompts

```text
total_user_prompts = count(user_prompt)
```

#### Average prompt length

Use one unit consistently. The MVP should use Unicode character count unless the implementation already has a reliable tokenizer.

```text
average_prompt_length = sum(length(prompt_text)) / total_user_prompts
```

Return `null` when there are no user prompts.

#### Median prompt length

```text
median_prompt_length = median(length(prompt_text) for each user_prompt)
```

Return `null` when there are no user prompts.

#### Output tokens

```text
output_tokens = sum(provider-reported output tokens across agent responses)
```

Return `unknown`, not an estimate, when the provider does not expose token usage.

### 4.2 Code-line change heatmap

The heatmap explains what happened to lines touched during the session. It does not update the cortisol signal.

Minimum visual semantics:

- **Green:** a line introduced or changed during the session and still present at computation time.
- **Red:** a touched line that was later deleted or replaced.
- **Darker red:** a line or location with more repeated deletion/replacement churn.

Required data:

- Session-start file baseline.
- Current file state.
- Intermediate observed file/edit snapshots if churn intensity is displayed.

If intermediate history is unavailable, render only the final baseline-to-current diff. Do not invent churn intensity from a final diff.

The exact heatmap aggregation and code-rendering library remain implementation decisions for the next planning step.

## 5. Significant metrics

These are the only v1 metrics eligible to be sent to the client-side cortisol visualization.

### 5.1 Acceptance ratio

**Question:** What portion of shown actionable suggestions did the user explicitly approve?

```text
acceptance_ratio = total_acceptances / count(suggestion_shown)
```

Return `null` when no suggestions were shown. Also retain the shown, accepted, rejected, and unresolved counts so the UI can show sample size.

### 5.2 Median time to approval

For each accepted suggestion `s`:

```text
approval_latency(s) = accepted_at(s) - shown_at(s)
median_time_to_approval = median(approval_latency(s) for accepted suggestions)
```
Approval latency can be rendered as a histogram with reasonable scale

Return `null` when nothing was accepted. This measures approval speed, not whether the decision was correct.

### 5.3 Median active post-edit response time

**Question:** After the agent finishes an edit, how much active-session time passes before the user's next observable response?

For every `agent_edit_completed(e)` with a later user action:

```text
post_edit_response_time(e) = active elapsed time from edit_completed_at(e)
                             to the next user_activity

median_active_post_edit_response_time = median(post_edit_response_time(e))
```
Again, post_edit_response_time can be rendered as a histogram with reasonable scale

Use the same active-time/inactivity rule as Estimated active time. A next response may be a prompt, acceptance, or rejection. Exclude edits with no subsequent user action and report their count as pending/unresolved context.

This metric is distinct from approval latency: it covers re-engagement after an agent file edit, whether or not that edit is represented by an approval event.

### 5.4 Qualitative LLM session summary

Produce a short bullet-list summary of the current session. It should describe observable evidence, not diagnose the user.

Minimum criteria:

- Whether the user supplied goals, constraints, context, or expected behavior.
- Whether the user demonstrated planning or direction.
- Whether the user questioned, corrected, rejected, or redirected AI output.
- Whether the user requested explanations, evaluation, or verification.
- Important uncertainty or missing telemetry.

The summary must not infer comprehension solely from accepted code, polite wording, profanity, prompt length, or tool activity. It should allow `unknown` when evidence is insufficient.

Future-compatible output may retain evidence event IDs and improvement suggestions, but the v1 dashboard only needs the bullet summary.

The prompt will be updated in the future

## 6. Cortisol signal boundary

The client may receive some feedback from server to alert user live their "cortisol level"
No composite cortisol formula or thresholds have been approved. Do not invent weights or convert these values into a scientific probability. Until explicitly designed, the visualization should present the signals or use mock display states without claiming physiological meaning.

The descriptive small numbers and code heatmap are dashboard-only context and must not influence the cortisol visualization.
