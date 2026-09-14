# Research Phase Context

**Silent — no narration to the developer. Output feeds Step 2 execution.**

The spec produced by feature-start already contains sourced conventions and decisions.
Extract those first. Only fall back to project entry point research for areas not covered.

---

## Pass 1 — Extract from the spec

From the phase doc and `index.md` already read in Step 0, extract:

- All decisions recorded as sourced facts (marked `per docs/conventions/...` or similar)
- All stated constraints and assumptions that bear on implementation

These are established decisions — already filtered and accepted. Do not re-research them.

Note any phase areas with no sourced convention. These are candidates for Pass 2.

---

## Pass 2 — Fill gaps (only for uncovered areas)

For each area not covered by the spec, locate the project entry point. Stop at first match:

```
1. AGENTS.md at repo root
2. memory-bank/systemPatterns.md
3. docs/ root index or README
```

If none found: mark the area convention-free and proceed.

If found: extract and load only pointers relevant to the uncovered area. Do not load
files for areas already covered by the spec. Do not load files not reachable at the
indicated path.

---

## Stale convention handling

If a convention from the spec conflicts with what the codebase actually does, do not
diverge silently. Surface the conflict, agree the correct approach with the developer,
update the spec, then continue. Record the discrepancy in Implementation notes.