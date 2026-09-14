---
name: implementation-breakdown
description: >
  Produces a factual, consumer-agnostic breakdown of a completed implementation for any
  project type. Use this skill whenever a developer says "produce an implementation
  breakdown", "write a handoff for this feature", "summarise what was built", "document
  this implementation for handoff", or any phrase implying they want to communicate a
  completed implementation to another team or person. Covers new and changed public
  interfaces, retired interfaces, data model changes, business rules, validation
  constraints, error codes, cascade behaviours, known gaps, and deferred items.
---
 
# Implementation Breakdown Skill
 
Produces a factual, consumer-agnostic breakdown of a completed implementation. The output
describes what was built accurately enough that any downstream consumer can understand the
change and act on it without needing to read the source.
 
---
 
## Principles
 
- Every claim in the output must be traceable to the implementation. Do not infer intent
  or speculate beyond what is directly evidenced by the spec, code, or commit history.
- Retired public interfaces must be called out explicitly. A consumer cannot infer
  removal from absence.
- Known gaps and deferred items must be stated plainly — this includes anything the
  implementation does not do that a consumer might reasonably expect, not only items
  formally marked as deferred in the spec.
- When a behaviour's properties are only partially documented — for example, a cascade is
  described but its reversibility is not — state only what is confirmed and mark the rest
  as unconfirmed. Do not infer the unstated half from the stated half.
---
 
## Step 1 — Establish the Implementation Scope
 
If the developer has not identified the implementation at invocation, ask:
 
*"What should I use as the source for this breakdown?"*
 
If nothing traceable is provided, state that the breakdown cannot be produced and ask
the developer to provide one. If any inputs conflict, flag the conflict rather than
silently resolving it.
 
---
 
## Step 2 — Read the Implementation
 
Start from whatever source was provided. Treat it as the authoritative record of what
was built — use it to establish scope, completion state, divergences from any planned
design, and deferred items.
 
Follow where the source points. References to specific files, packages, components, or
changes indicate what is worth reading to verify and supplement. Read those — not to
discover scope from scratch, but to confirm what the source describes.
 
Where the source is ambiguous or incomplete, note what could not be confirmed rather
than inferring it.
 
Do not perform broad scans. Do not read anything the source does not reference. Stop
reading once the information needed for each output section is sufficient.
 
---
 
## Step 3 — Identify the Output Sections
 
Before writing, identify which sections apply to this implementation. Omit any section
the implementation did not touch — do not include empty sections.
 
| Section | Include when... |
|---------|----------------|
| Summary | Always |
| New or changed public interfaces | Any public surface was added or updated — endpoints, exported functions, message schemas, job contracts, or equivalent |
| Retired public interfaces | Any existing public surface was removed or replaced |
| Data model changes | Schema, entities, fields, or constraints changed |
| Business rules and constraints | Any non-trivial validation or rule was implemented |
| Error codes | Machine-readable codes are emitted, handled, or returned by this implementation |
| Cascade behaviours | An action on one entity affects related entities |
| Deferred items and known gaps | Anything planned but not yet implemented, or not done that a consumer might reasonably expect |
 
---
 
## Step 4 — Produce the Breakdown
 
Read `references/output-template.md` and produce the breakdown in chat using that
structure. Omit any section not identified as applicable in Step 3.
 
## Step 5 — Review Before Responding
 
Before producing the output, verify:
 
- [ ] Every public interface listed exists in the implementation — not in the planned spec only
- [ ] Every error code listed is actually emitted, handled, or returned — not just mentioned in the spec
- [ ] Deferred items are accurately distinguished from completed items
- [ ] Nothing in the output prescribes a UX decision or delivery sequence
- [ ] Retired public interfaces are explicitly listed, not just absent from the new interfaces list
- [ ] Divergences from the original design are reflected accurately — implementation notes take precedence over planned spec
- [ ] Every claim about cascade reversibility is sourced from the implementation or spec —
      if reversibility is not explicitly documented, it is marked as unconfirmed
If any of these checks cannot be satisfied because a required file is unavailable or
ambiguous, state what is missing at the top of the output rather than omitting it silently.
 
---
 
## What This Skill Does NOT Do
 
- Does not make UX or interaction decisions
- Does not recommend delivery order or phasing for the consuming team
- Does not produce acceptance criteria for the consuming team's work
- Does not generate code, migrations, or configuration
- Does not create files unless the developer explicitly asks for one