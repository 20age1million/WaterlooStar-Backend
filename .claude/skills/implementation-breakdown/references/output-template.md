# Output Template
 
Respond in chat with the following structure. Omit sections not identified as applicable
in Step 3. Write in plain, direct prose or concise tables — no narrative framing, no
speculation.
 
---
 
### Implementation Breakdown: `<feature-name>`
 
#### Summary
 
One to three sentences. What was built, what it replaces or extends, and the overall
outcome. State the bounded scope clearly — what is and is not covered.
 
---
 
#### New or Changed Public Interfaces
 
For each new or materially changed public interface — HTTP endpoint, exported function,
message schema, job contract, or equivalent for this project type:
 
- Identity — method and path, function signature, message type, or equivalent
- What it does
- Success response, return type, or output — including code if applicable
- Inputs — required vs optional, types, constraints
- Output shape — key fields and their meaning
- Auth or permission requirements if non-standard
Group by resource or domain if the implementation introduces multiple related interfaces.
 
If an existing interface changed its contract (field names, response shape, added or
removed parameters), state what changed and what the previous shape was if known.
 
---
 
#### Retired Public Interfaces
 
List every public interface that was removed or replaced. For each:
 
- Identity — method and path, function signature, message type, or equivalent
- What replaced it, if anything
- Any call sites or consumers that will need to be updated
---
 
 
#### Data Model Changes
 
For each entity or structure that changed:
 
- New entities — purpose, key fields, constraints, relationships
- Dropped entities — what replaced them
- Field-level changes — added, removed, or renamed fields with types and nullability
- Constraint or index changes — new or dropped uniqueness rules, relationships, indexes or equivalent
State the intent behind non-obvious changes (e.g. why a field was split into two).
 
---
 
#### Business Rules and Constraints
 
List every rule or constraint the implementation enforces that is not obvious from the
field types alone. Include:
 
- Validation rules (e.g. a numeric field must be positive)
- Uniqueness rules (e.g. one record per combination of two fields)
- Immutability rules (e.g. fields that cannot be changed after creation)
- Scope or hierarchy rules (e.g. most-specific match wins)
- Anything a consumer must respect to interact with this surface correctly
---
 
#### Error Codes
 
List every machine-readable error code this implementation emits, handles, or returns.
Frame each entry from the perspective of this implementation — a backend emits codes,
a frontend handles them, a library returns typed results.
 
| Code | Description |
|------|-------------|
| `EXAMPLE_CODE` | Description of when and why this code occurs |
 
Group by component if the list is long. Note which codes map to which HTTP status codes
if that mapping is non-obvious.
 
---
 
#### Cascade Behaviours
 
Describe any action that has side effects on related entities. For each:
 
- The triggering action
- The cascaded effect
- Whether the cascade is recoverable — only state this if the source explicitly addresses
  it. If reversibility is not documented, write
  "Reversibility not confirmed in implementation — verify before assuming."
---
 
#### Deferred Items and Known Gaps
 
List everything that was planned as part of this feature but not implemented, plus
anything the implementation explicitly does not do that a consumer might reasonably
expect. For each item:
 
- What was deferred or omitted
- Why (if stated in the source)
- What the downstream impact is for a consumer acting on this breakdown now
Do not omit this section if there are deferred items or known gaps.