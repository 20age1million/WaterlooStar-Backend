---
name: feature-implement
description: >
  Guides developers through implementing a feature from an agreed spec committed in
  docs/specs/active/. Use this skill whenever someone says "implement this feature",
  "start implementing", "work through the spec", "implement phase N", "pick up from
  the spec", or any phrase implying the beginning of implementation work from an
  existing specification. Executes one phase at a time. Owns the full implementation
  loop: context research, spec-driven execution, ask-first for developer actions,
  spec lifecycle, and phase commit. Does not create specs — use feature-start for that.
  Does not own the PR process. Works across all project and repository types.
---

# Feature Implement Skill

Guides a developer through implementing one phase of an agreed feature specification.
Works across all project and repository types.

**Executes one phase at a time. Stops at the phase commit. Does not own the PR.**

---

## Principles

- One phase per execution. Do not batch phases.
- The spec is the contract. Execute the checklist as written. Where sequence is not
  explicit, work lowest dependency to highest — foundational changes before the code
  that depends on them.
- Ask before acting on any step the developer must perform outside this conversation.
  State the exact command, then stop and wait.
- Never diverge silently. If a planned approach proves unworkable, surface it, agree
  a revision with the developer, update the spec, then continue.

---

## Step 0 — Identify the Phase

Read `docs/specs/active/<feature-slug>/index.md` and the target phase doc. Confirm:

- This is the correct next phase
- Dependencies and prerequisites are met
- Any open questions in the phase doc are resolved before starting

If a blocker exists and cannot be resolved in this conversation, stop. Tell the
developer exactly what needs to happen externally before implementation can begin.
Do not proceed past Step 0 with an unresolved blocker.

---

## Step 1 — Load Phase Context

Follow `references/context-research.md` exactly. Silent — no narration to the developer.

---

## Step 2 — Execute

Work through the phase checklist in order. Where sequence is not stated, work
lowest dependency first. Mark each checklist item `- [x]` as it is completed.

Any checklist item requiring a developer action outside this conversation:
- State what is needed and the exact command
- Stop and wait for confirmation before continuing

If a planned approach proves unworkable, stop, agree a revision, update the spec, continue.

---

## Step 3 — Verify and Close

1. Confirm every completion criterion from the phase doc is met or explicitly deferred
   with a recorded reason.
2. Add `## Implementation notes` to the phase doc:
   - Key files changed
   - Any divergence from the planned design and why
   - Verification steps run and their outcomes

---

## Step 4 — Spec Lifecycle and Commit

In order, before committing:

1. Move the phase file: `active/<feature-slug>/phase-N-*.md` → `implemented/<feature-slug>/`
2. Update `index.md` — set phase status to `Complete`
3. Update `docs/specs/INDEX.md` if present
4. Update `memory-bank/` if present — `activeContext.md` and `progress.md` if a milestone
5. Commit everything in one commit, following the project's commit message convention.
   If no convention is established, use conventional commits format:
   ```
   <type>(<feature-slug>): implement <phase-title>
   ```

If this was the final phase, move `index.md` to `implemented/<feature-slug>/` in the same commit.

---

## Step 5 — Check for Remaining Phases

Read `index.md`. Further phases in `active/`?

- **Yes** — present the next phase. Run this skill again from Step 0.
- **No** — confirm all files are in `implemented/` and `INDEX.md` is current.