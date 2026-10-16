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
- **Every pull request leaves the documentation current.** The next agent starts from
  the repository's files, not from this conversation. `CLAUDE.md` above all must let
  someone with no other context pick the work up seamlessly (Step 4, item 4).
- **Every pull request carries a description of what it changes.** A title alone is
  not a description. See "The pull request description" below.

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
4. **Update the handoff documentation — required, every phase, because every phase ends
   in a pull request.** Another agent must be able to pick the work up from the files
   alone:
   - `CLAUDE.md` (or the repository's agent entry point), above all:
     - what is built, including this phase;
     - what is in flight: the branch, the pull request, what remains;
     - what comes next;
     - decisions the developer made during the phase, with the date and why;
     - gotchas learned, with what they cost and how to avoid them.
   - `README.md` and its linked guides under `docs/`: the routes or endpoints,
     commands, configuration and environment variables the phase changed.
   - Any other document the phase made stale. Search the docs for the names, paths,
     counts and statements the phase changed.

   Correct or remove a stale statement; do not leave it beside the new one.
5. Update `memory-bank/` if present — `activeContext.md` and `progress.md` if a milestone
6. Commit everything in one commit, following the project's commit message convention.
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

**After a pull request merges,** the documents still describe it as in flight. The next
piece of work in the repository starts by bringing `CLAUDE.md` up to date: record the
merge and its pull request number, and clear or replace the "in flight" entry.

---

## The pull request description

This skill stops at the commit and does not open the pull request. Whoever opens one,
for a phase or for anything else, writes a description of what that pull request
changes. A title, an empty body or a line of attribution is not one. A reviewer who
reads only the description must know what they are about to review:

- **What changed and why**, grouped by area (code, contract, schema, configuration,
  documentation). Name the endpoints, routes, files or behaviours a reviewer should
  look at, and the spec or phase it implements, if any.
- **The commits it contains**, one line each, when there is more than one.
- **How it was verified:** the commands run and their outcome, or that the change is
  documentation-only and nothing was run.
- **What it leaves open:** known gaps, deferred items, follow-up work and anything the
  reviewer has to do, such as an environment variable to set on the host.

The description covers the whole pull request as it stands. When further commits land
on the branch, edit the description to match, not only the first commit.