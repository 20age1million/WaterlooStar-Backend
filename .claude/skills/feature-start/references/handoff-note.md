# Handoff Note Template

Use this template at Step 8 to signal the end of the feature-start stage.
Populate all placeholders before presenting to the developer.

---

## Rendered Output

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 FEATURE-START STAGE COMPLETE
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Feature:        <feature-name>
Branch:         feature/<short-description>
Specification:  docs/specs/active/<feature-slug>/

Phases:
  <List each phase: "Phase 1 — <title> (Ready)">
  <Phase 2 — <title> (Ready)>

Affected areas:
  <Comma-separated list of areas from the index.md layers table>

Acceptance point:
  Specification reviewed, accepted, and committed as the first
  meaningful commit on the feature branch.

  Commit: docs(<feature-slug>): add accepted specification for <feature-name>

Open questions:
  <If none: "None — all questions resolved before acceptance.">
  <If any remain: list each question, the phase it affects, and its named owner.
   These must be resolved before the affected phase begins — not during implementation.>

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 READY FOR IMPLEMENTATION
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

The feature branch is ready for implementation work.

Before starting:

  1. Review the committed specification and confirm alignment
     across the team.
  2. Resolve any open questions recorded in the spec.
  3. Break each phase into tasks if your workflow requires it.
  4. Implement on feature/<short-description> — never on the primary branch.

Conventions:
  - Work through phases in order.
  - Each completed phase is committed independently.
  - Update index.md phase status as work progresses.
  - On phase completion, add Implementation Notes to the
    phase doc before committing.
  - On phase completion, move the phase spec doc:
      docs/specs/active/<feature-slug>/phase-N-*.md
      → docs/specs/implemented/<feature-slug>/phase-N-*.md
  - On final phase completion, also move index.md:
      docs/specs/active/<feature-slug>/index.md
      → docs/specs/implemented/<feature-slug>/index.md
  - Any scope changes during implementation must be reflected
    by updating the spec and committing the change before
    the affected phase begins.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```