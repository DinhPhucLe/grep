# Cortisol CLI — Project context

This document records the current product direction after the intervention pivot. It replaces the earlier passive observability and dashboard metric contract. Confirmed behavior is described below; unresolved product and implementation choices are marked **WIP**.

## 1. Purpose

Cortisol CLI is a terminal interface for working with Codex that helps users notice the consequences of leaving coding instructions ambiguous. It intervenes in the workflow rather than merely monitoring it. The user works through our TUI, which remains a client of Codex app-server.

The central experience is a contrast between **what the user intended** and **what Codex chose to implement** when the prompt left room for interpretation. For an ambiguous request, Codex is allowed to make its own implementation decisions. The product then draws attention to those decisions and asks the user to reason about them before revealing the relevant code. It does not silently improve the prompt or preselect the “right” interpretation for Codex.

The goal is meaningful engagement with AI-generated code and with the instructions that led to it. A correct quiz answer or a high score is not proof that a user understands the entire implementation.

## 2. Core user flow

1. The user enters a prompt in our TUI. The system evaluates its clarity in the context of the task, repository, and relevant prior conversation. A short follow-up can be sufficiently clear when earlier turns already established the requirements.
2. **When the prompt is sufficiently clear:** the user earns points, and the TUI sends the prompt to Codex for normal execution.
3. **When the prompt leaves consequential ambiguity:** Snowflake Cortex identifies the gaps. The user's original instructions are sent to Codex so its choices expose the consequences of those gaps.
4. Codex generates an implementation. The TUI withholds selected, important implementation logic from the initial presentation, then asks questions about the decisions or behavior that the ambiguous prompt left open. The user gets up to **two attempts** to answer.
5. Correct answers earn points. After two failed attempts, points are deducted. **The code is revealed either way** so the user can inspect Codex's choice and compare it with their intent.
6. The user can continue the conversation, clarify the intended behavior, and ask Codex to revise the result. This subsequent prompt enters the same evaluation flow with relevant context.

For example, “fix login” might leave open whether the problem is an expired session, a bad password, or an error message. The learning moment is seeing which case Codex chose and whether it matches what the user meant. A question about an unrelated syntax detail would miss that purpose.

The user remains in control of Codex execution and review. Existing Codex requests for approvals or other user decisions must still reach the user.

## 3. What the intervention should test

The ambiguity check concerns missing decisions that could materially change the result, not prompt length, tone, or a preference for verbose instructions. It must consider requirements and plans already established in the conversation.

Questions should connect a flagged gap to a consequential choice in Codex's actual implementation. They can ask the user to predict behavior, identify an assumption Codex made, or explain why the result may differ from their intent. The reveal then shows the relevant code and the consequence, including when the user answered incorrectly.

The exact clarity rubric, question format, answer evaluation criteria, treatment of partially correct answers, and handling of multiple ambiguities are **WIP**. Do not substitute simple prompt-length rules or treat an LLM judgment as a validated measure of understanding.

## 4. Architecture direction

### 4.1 Codex wrapper

The Go TUI remains a client of `codex app-server` over JSON-RPC via stdio. Codex app-server runs the coding agent and performs coding actions. Our TUI owns the user-facing intervention: prompt evaluation, explanation of ambiguous gaps, the quiz and reveal sequence, and points feedback. It sends user prompts as Codex turns, streams agent responses, renders conversation and code review, and surfaces Codex approval requests.

The TUI does not reimplement Codex's coding agent loop. See the TUI protocol documentation for the current app-server integration. Existing code may still reflect the former product direction; this document defines the target scope rather than claiming the new flow is implemented.

### 4.2 Snowflake Cortex

Snowflake Cortex is the planned LLM service for checking prompt clarity, identifying consequential gaps, generating questions, and evaluating answers. Codex generates and changes code. Structured LLM responses may help connect each question to a specific ambiguity and implementation choice, but the model, request format, output contract, and validation strategy are **WIP**.

### 4.3 Withholding and revealing code

The desired user experience temporarily withholds selected important logic before the quiz, then reveals it regardless of the outcome. **How to do this safely and credibly is WIP.** If Codex has already written code into the user's working tree, hiding text in the TUI does not hide the code. A genuine reveal sequence may require staging or isolating the generated changes until the review step. The system must not claim that code is hidden when it is already accessible on disk.

The criteria for selecting important logic and deciding what remains visible are also **WIP**. The user must ultimately be able to inspect the full result, including after two wrong answers.

## 5. Points and longer-term rewards

The confirmed point events are:

- A sufficiently clear prompt earns points.
- A correct answer to an ambiguity-related question earns points.
- Two failed attempts on a question deduct points; the code is still revealed.

Point values, thresholds, score persistence, recovery, and safeguards against inconsistent LLM judgments are **WIP**. The points are feedback within the experience, not a scientific measure of effort, comprehension, or developer ability.

A persistent visual mascot or similar long-term reward is intended for a later stage. Groot or a growing and wilting tree has been discussed as a candidate, but the character, appearance, states, and progression are **WIP** under this pivot. The MVP should stand on the intervention itself.

## 6. MVP boundary and superseded direction

The MVP should demonstrate one complete loop: evaluate a prompt in context, let Codex implement an ambiguous request as written, ask about one consequential choice, reveal the result after at most two attempts, and show the point outcome. A clear prompt should take the direct execution path and earn points.

The earlier whole-repo ASCII future diagram and pre-execution prompt rewrite are no longer the central experience. The previous dashboard, observability metrics, heatmap, and cortisol signal are also outside this product direction. Their old formulas must not be reused as a proxy for prompt quality or comprehension.

The intervention is motivated by research on active engagement with AI-generated code, including Kazemitabaar et al., *Exploring the Design Space of Cognitive Engagement Techniques with AI-Generated Code for Enhanced Learning* (IUI 2025; arXiv:2410.08922). That work studied specific techniques with novice programmers; it does not validate this product's point rules, two-attempt limit, or use of Snowflake Cortex as a judge. Those choices require product testing.
