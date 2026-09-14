---
name: feature-start
description: >
  Guides developers through the spec-driven feature-start workflow for any project type
  (backend API, frontend, background worker, service, library, etc.). Use this skill whenever
  someone says "I want to start a new feature", "kick off feature work", "create a feature
  spec", "begin a feature branch", "spec out this feature", "start work on X", or any phrase
  implying the beginning of new planned work before implementation. Covers confirming feature
  intent, branch hygiene (never on the primary branch), Git commands, spec folder creation
  under docs/specs/active/FEATURE-SLUG/, drafting index.md and phase docs, refining scope,
  and committing the accepted specification as the first meaningful commit on the feature
  branch. Stops at the committed spec. Does not implement the feature. Does not own the PR
  process. Works across all project types and repository structures.
---

# Feature Start Skill

Guides a developer from a feature idea to an accepted specification committed on a feature
branch. Works across all project and repository types.

**Stops once the accepted spec is committed. Does not implement. Does not own the PR.**

---

## Principles

- All new feature work begins from a feature branch created from the latest primary branch.
  The primary branch is typically `main` or `master` — confirm which is used in this
  repository before running branch commands. If neither is obvious, ask the developer.
- A formal ticket is optional. A documented feature intent is **mandatory**.
- The first committed deliverable on the branch is the accepted feature specification.
- The committed specification is the acceptance point for the feature-start stage.
- Work is **never** performed directly on the primary branch.
- Specs use the `index.md + phase-N-*.md` structure universally across all project types.
- Ask rather than assume. If the answer to a question would affect scope, constraints, phase
  boundaries, or checklist content, ask the developer directly. Do not infer or fill in from
  context unless the answer is unambiguously implied by what the developer has already stated.

---

## Step 0 — Confirm Feature Intent

Gather the following before proceeding. If any field is not clearly stated by the developer,
ask directly — do not infer.

| Field | Required | Notes |
|---|---|---|
| Feature name | Yes | Short, hyphen-separated slug: `user-notifications`, `payment-flow` |
| Feature intent | Yes | One or two sentences: what it does and why |
| Project / repo type | Yes | Backend API, frontend, worker, library, etc. |
| Primary branch name | Yes | Typically `main` or `master` — confirm before Step 2 |
| Ticket / work item | No | Record if provided; omit if not |

If the developer has not described the feature yet, ask:
*"What feature are you starting, and what is it intended to do?"*

Once the initial description is given, ask about anything that is not already clearly stated
before proceeding. Do not fill in gaps from inference. Areas that commonly need direct answers:

- What problem does this solve, and why is it being done now?
- Who are the users or consumers of this feature?
- Are there any known constraints — technical, regulatory, or time-based?
- Are there dependencies or prerequisites that need to be confirmed first?
- Are there any open decisions — technical or product — that could affect scope?
- Is there a ticket or work item to reference?

Ask these as natural questions in conversation, not as a form. Cover only what has not already
been answered. Record anything that cannot be resolved now — these become candidates for Open
Questions in the spec if they remain unresolved at commit time.

---

## Step 1 — Determine Scope and Phase Structure

1. Read `references/project-areas/INDEX.md` to select the correct areas file for this
   project type. If the project spans multiple types, read the primary type first, then
   supplement with the secondary type's file.

2. Using the areas file as a completeness reference, reason about which areas the feature
   is likely to touch based on its intent and project type. Record this candidate list
   internally.

3. Follow `references/context-research.md` exactly. This step is silent — no narration
   to the developer. Its output (established decisions and convention-free areas) feeds
   into instruction 4 and Step 4.

4. Present the candidate areas to the developer. For each, give a one-line reason why it
   is likely in scope and note any convention identified in instruction 3. Ask the
   developer to confirm, add, or remove. Do not present the full areas list.

5. Use the confirmed areas to populate the `index.md` layers table and decide the phase
   breakdown.

**Phase sizing guide:**

| Affected areas | Suggested structure |
|---|---|
| 1–3 | `index.md` only — single phase or very small feature |
| 4–6 | `index.md` + `phase-1-*.md` |
| 7+ | `index.md` + `phase-1-*.md` + `phase-2-*.md` + ... |

Each phase should represent a coherent, independently committable unit of work.
Ask the developer to confirm the phase breakdown before drafting.

---

## Step 2 — Branch Hygiene

Run the following commands, substituting `<primary-branch>` with the confirmed primary branch
name from Step 0. Work never starts on the primary branch.

```bash
# Check current branch — confirm you are not on the primary branch
git status

# Switch to the primary branch and pull latest
git checkout <primary-branch>
git pull origin <primary-branch>
```

> Never start feature work directly on the primary branch. Always branch from its latest state.

---

## Step 3 — Create the Feature Branch

**Naming convention:** `feature/<short-description>`

The `<short-description>` must match the feature slug from Step 0.

Run:

```bash
git checkout -b feature/<short-description>
git push -u origin feature/<short-description>
```

---

## Step 4 — Create the Specification Folder

All feature specifications live under:

```
docs/specs/active/<feature-slug>/
```

```bash
mkdir -p docs/specs/active/<feature-slug>
```

---

## Step 5 — Draft the Specification

Follow `references/spec-templates.md` for file structure and content requirements.

Before writing anything:
- Batch all convention-free areas from Step 1 into a single question block and wait
  for answers. These become stated decisions in the spec, not inherited ones.
- If any detail needed to write a section accurately is unknown, ask before writing.
  Do not fill in plausible-sounding content to refine later.

When writing established decisions from Step 1, record them as sourced facts:
> *(per `docs/conventions/<file>.md`)*

When generating phase docs, apply the following rules unconditionally:

- **Every phase doc** — include a checklist item in Final Verification to move the phase
  spec doc from `docs/specs/active/<feature-slug>/` to `docs/specs/implemented/<feature-slug>/`
  and a corresponding Completion Criteria item.
- **Final phase doc only** — include a checklist item to move `index.md` from
  `docs/specs/active/<feature-slug>/` to `docs/specs/implemented/<feature-slug>/`.
- **Final phase doc only, if `CHANGELOG present: true`** — include a checklist item to
  update `CHANGELOG.md` under `Unreleased` with an entry for the feature.
- **If `CHANGELOG present: false`** — omit all CHANGELOG items entirely. Do not add a
  note or placeholder.

After generating, present the files and ask:
*"Does this capture your intent accurately? What needs to change?"*

Open questions are a last resort — only record one if resolution genuinely requires
an external decision or information not yet available.

---

## Step 6 — Refine the Specification

Refinement is a loop, not a single pass. Repeat until the developer explicitly accepts the scope.

**On each iteration:**

1. Present or re-present the current spec state.
2. Work through any open questions in `index.md` — for each one, attempt to resolve it with
   the developer and update the spec. Push to resolve rather than accept the question as
   permanently open. Only leave a question recorded as open if resolution requires something
   genuinely outside the current conversation: an external decision, an unconfirmed dependency,
   or information the developer does not yet have.
3. Probe the scope with these checks:

   - *"Are there any constraints or hard limits I should add?"*
   - *"Are there any assumptions here you'd push back on?"*
   - *"Is the scope tight enough, or should anything move to a separate feature?"*
   - *"Are the phase boundaries clean — could each phase be committed independently?"*
   - *"Are the acceptance criteria testable and unambiguous?"*

4. Apply any changes to the spec files immediately. Do not accumulate edits in conversation.
5. If new unknowns surface during discussion, attempt to resolve them inline before adding
   them to Open Questions.
6. If the developer challenges a decision sourced from a project convention, follow the
   stale convention handling process defined in `references/context-research.md` — update
   the spec to the agreed approach and record the convention debt. Do not silently diverge.

**Loop exit condition — all of the following must be true:**

- [ ] The developer has explicitly said the scope is accepted (words to that effect —
      do not infer acceptance from silence or a lack of objections)
- [ ] All open questions are either resolved and removed from the section, or recorded
      with a named owner and a clear resolution path
- [ ] No checklist item in any phase doc depends on an unresolved question that would
      change its scope

**Do not proceed to Step 7 until all three conditions are met.**

---

## Step 7 — Commit the Accepted Specification

This step is only reached once the Step 6 exit conditions are all met.

If the repository has a `docs/specs/INDEX.md`, add the new feature to the `Active`
table with status `Ready` and location `docs/specs/active/<feature-slug>/` before
committing.

**Commit message format:**

```
docs(<feature-slug>): add accepted specification for <feature-name>
```

Do not add `Co-Authored-By` or any other trailer lines to the commit message.

Run:

```bash
git add docs/specs/active/<feature-slug>/
git add docs/specs/INDEX.md   # if present
git commit -m "docs(<feature-slug>): add accepted specification for <feature-name>"
git push origin feature/<short-description>
```

> This commit is the acceptance point for the feature-start stage.
> It records the agreed implementation scope before any code is written.

---

## Step 8 — Handoff Note

Once the spec is committed, present the handoff note.
Read `references/handoff-note.md` for the template.
Populate with: feature name, branch, spec location, phase summary, affected areas,
and any open questions recorded in the spec.

---

## Deliverables Summary

| Deliverable | Step |
|---|---|
| Confirmed scope and phase structure | 1 |
| Ready-to-run Git commands | 2, 3, 7 |
| `docs/specs/active/<feature-slug>/` folder | 4 |
| `index.md` with all required sections | 5 |
| `phase-N-*.md` files with checklists and completion criteria | 5 |
| Scope refinement and phase boundary review | 6 |
| First-commit message and commands | 7 |
| Handoff note | 8 |

---

## What This Skill Does NOT Do

- Does not implement the feature
- Does not create pull requests
- Does not own code review
- Does not move specs from `active/` to `implemented/`
- Does not run builds, tests, or verification commands
- Does not write code, migrations, or configuration