# Research Project Context

**Runs silently as Step 1 instruction 3. No narration to the developer.
Output feeds Step 1 instruction 4 (area presentation) and Step 5 (established decisions
written into spec).**

Research what the project has already established for each candidate area before
presenting scope to the developer. The goal is to inherit established decisions
rather than leave them blank, and to surface genuinely novel decisions as targeted
questions rather than assumptions.

This step reads documentation only. It does not explore source code.

---

## Pass 1 — Orientate (at most 2–3 file operations)

Locate the project's entry point document using the following priority order.
Stop at the first match — do not continue down the chain if a result is found.

```
1. AGENTS.md at repo root          — preferred; purpose-built for AI assistants
2. memory-bank/systemPatterns.md   — if present
3. docs/ root index or README      — last resort
```

**If none found:** mark all candidate areas as convention-free and return to
Step 4 instruction 4. Do not search further. Do not look at source code.

**If found:** read the located file and extract all declared pointers — these may be
links to convention files, rule files, workflow documents, memory bank files, or
inline decisions. The entry point may use any format (tables, lists, prose, frontmatter)
to declare these; read it as a human would.

**Relevance filtering — mandatory before loading anything:**

For each extracted pointer, ask: does the declared scope of this pointer overlap with
the candidate feature areas? Retain only those that do. Discard the rest entirely —
do not load them, even if they are present and readable.

A pointer's scope may be stated explicitly (tied to specific file types or surfaces)
or implied by its name or description. When scope is ambiguous, err toward including
it only if the area overlap is plausible.

This filtering is what keeps context focused. Loading a convention file for an area
the feature does not touch dilutes the research and wastes context. Only files that
cleared this filter are loaded.

**Loading:** read each file in the filtered candidate list. If a file cannot be found
at the path indicated by the entry point, mark that area as convention-free and move
on. Do not search for alternative paths. Do not load any file not on the filtered list.

Output of Pass 1: a candidate file list — one or more files per candidate area,
derived only from what the entry point document declares and filtered for relevance.
If the entry point makes no mention of a given area, that area is already convention-free.

---

## Output

Produce two lists before returning to Step 4 instruction 4:

**Established decisions** — for each covered area, the decision and its source:
```
Area: Data model changes
Decision: Schema changes are append-only; existing records must not be altered by migrations
Source: docs/conventions/data-model.md
```

**Convention-free areas** — areas with no established pattern found:
```
Area: Notification delivery — no convention found
Area: File storage naming — no convention found
```

Convention-free areas become targeted questions batched at the start of Step 5.
Established decisions are written into the spec as sourced facts and annotated
inline in the Step 4 area confirmation presented to the developer.

**Project flags** — record each of the following for use in Step 5:

```
CHANGELOG present: true | false   # result of: ls CHANGELOG.md 2>/dev/null
```

---

## Stale convention handling

If the developer challenges an established decision during Step 6 — "that's out of
date, we do it differently now" — do not silently update the spec with the new
approach. Instead:

1. Update the spec to reflect the agreed current approach.
2. Add a note in the spec's Constraints or References section:
   > Convention update required: `<source file>` describes `<old approach>` but
   > the agreed approach for this feature is `<new approach>`. The convention
   > should be updated before this feature is implemented.

The spec records the debt. It does not own the fix.