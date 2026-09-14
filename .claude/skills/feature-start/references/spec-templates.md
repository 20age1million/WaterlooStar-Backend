# Specification Templates

Use these templates in Step 5 to generate specification files.
Replace all placeholder values with content from the developer interview.
Adapt layer/area names to match the project type and repository conventions.

---

## index.md

```markdown
# <Feature Title>

**Status:** Ready
**Branch:** `feature/<short-description>`
**Ticket / Work Item:** <reference or N/A>
**Owner(s):** <team or role>
**Date:** <YYYY-MM-DD>

---

## Purpose / Goal

<1–2 sentences describing the objective and the value delivered.>

---

## Problem Statement / Motivation

<What problem is being solved? Why now? What is the cost of not doing this?>

---

## Proposed Solution / Design

<High-level approach, key components, and relationships.
No code snippets. Reference existing files and patterns by path.>

### Key Components

- **`<component-a>`**: <responsibility>
- **`<component-b>`**: <responsibility>

### Data / Control Flow

<Describe the flow in bullet form. Keep it conceptual and self-contained.>

---

## Layers / Areas Affected

Adapt rows to the project type. Remove rows that do not apply.

| Layer / Area | Change |
|---|---|
| <area-1> | <description or "none"> |
| <area-2> | <description or "none"> |
| <area-3> | <description or "none"> |
| Tests | <description or "none"> |
| Configuration | <description or "none"> |
| Deployment / Infra | <description or "none"> |
| Operational | <description or "none"> |

---

## Phase Tracker

| Phase | Title | Status | Location |
|---|---|---|---|
| [1 — <title>](./phase-1-<title>.md) | <brief description> | Ready | `docs/specs/active/<feature-slug>/` |
| [2 — <title>](./phase-2-<title>.md) | <brief description> | Ready | `docs/specs/active/<feature-slug>/` |

*Remove rows for phases that do not exist.*

---

## Trade-offs / Alternatives Considered

- **<option A>** — <why rejected or why chosen>
- **<option B>** — <why rejected or why chosen>

---

## Assumptions

Things believed to be true that have not been formally verified, and whose truth the
implementation depends on. Distinguish from constraints (hard limits that must be respected)
and open questions (things that are genuinely unknown and need a decision).

- <Assumed to be true but not formally verified>

---

## Constraints

- <Hard limit that must be respected: technical, regulatory, operational, or time-based>

---

## Open Questions

Questions that could not be resolved during spec drafting and require an external decision,
an unconfirmed dependency, or information not yet available. Each must name who is responsible
for resolving it and what unblocks resolution. Aim to keep this section empty — an open
question is a last resort, not a placeholder for things that can be worked out now.

| # | Question | Affects | Owner | Status |
|---|----------|---------|-------|--------|
| Q-1 | <question> | <phase or area> | <name or role> | Open |

*Remove this section entirely if there are no open questions at the time of acceptance.*

---

## Success Criteria / Definition of Done

- <Measurable outcome>
- <Verification evidence expected>
- All phase checklists completed and verified
- No regressions in related areas

---

## References (internal only)

- Related specs: `docs/specs/<other-feature>/index.md`
- Standards / conventions: `docs/conventions/<relevant-convention>.md`

*No external URLs. All essential context must be captured within the repository.*
```

---

## phase-N-*.md

```markdown
# Phase <n> — <Phase Title>

**Status:** Ready
**Feature:** [<feature-name>](./index.md)
**Objective:** <One sentence describing what this phase delivers.>

---

## Scope

### In scope

- <item>

### Out of scope

- <item>

---

## Dependencies / Prerequisites

- <Prerequisite within the repo or controlled environment. No external references.>

---

## Implementation Steps

- [ ] **Preparation**
  - [ ] Confirm prerequisites are available
  - [ ] Update `docs/specs/active/<feature-slug>/index.md` — set Phase <n> to In Progress

- [ ] **<Area group 1>**
  - [ ] <Concrete action — verb + file path + what changes>
  - [ ] <Concrete action>
  - [ ] Verify: <describe verification action and expected result>

- [ ] **<Area group 2>**
  - [ ] <Concrete action>
  - [ ] Verify: <describe verification action and expected result>

- [ ] **<Area group 3>**
  - [ ] <Concrete action>
  - [ ] Verify: <describe verification action and expected result>

- [ ] **Tests**
  - [ ] <Describe what tests cover and which files they live in>
  - [ ] Verify: test suite passes

- [ ] **Final verification**
  - [ ] Build or compile step passes with no errors (adapt to project type: compile, lint,
        bundle, type-check, etc.)
  - [ ] All relevant tests pass
  - [ ] Update `index.md` — set Phase <n> to Complete
  - [ ] Move `docs/specs/active/<feature-slug>/phase-<n>-<title>.md` →
        `docs/specs/implemented/<feature-slug>/phase-<n>-<title>.md`

*If this is the final phase, also include:*

- [ ] *(Final phase only)* Update `CHANGELOG.md` — add entry for `<feature-name>` under
      `Unreleased` describing what was delivered
      *(omit if CHANGELOG.md was not detected during spec authoring)*
- [ ] *(Final phase only)* Move `docs/specs/active/<feature-slug>/index.md` →
      `docs/specs/implemented/<feature-slug>/index.md`

---

## Completion Criteria

- [ ] All checklist items completed and verified
- [ ] No regressions in related areas
- [ ] `index.md` phase status set to Complete
- [ ] Phase spec doc moved to `docs/specs/implemented/<feature-slug>/`
- [ ] *(Final phase only)* `CHANGELOG.md` updated under `Unreleased`
      *(omit if CHANGELOG.md was not detected during spec authoring)*
- [ ] *(Final phase only)* `index.md` moved to `docs/specs/implemented/<feature-slug>/`

---

## Implementation Notes

> *Added after completion. Fill in before committing the phase.*
>
> **Key files changed:**
> - `path/to/file`: what changed
>
> **Divergences from plan:**
> - <none / description>
>
> **Verification run:**
> - <command or action and its result>
```