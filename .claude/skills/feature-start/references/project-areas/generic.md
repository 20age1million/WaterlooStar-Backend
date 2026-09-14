# Project Areas — Generic / Unlisted

Use this file when the project type does not match any of the specific files,
or when the project spans multiple types without a clear primary.

Derive the layers table from these four first-principle categories,
then add project-specific rows as needed.

| Category | Area | Examples |
|---|---|---|
| Entry point | How does work arrive? | HTTP request, queue message, scheduled trigger, CLI invocation, event |
| Processing | What transforms or acts on the input? | Business logic, validation, orchestration, computation |
| Persistence / output | Where does the result go? | Database, queue, file system, external API, cache |
| Cross-cutting | What applies throughout? | Auth, configuration, observability, tests, deployment |

Expand each category into specific rows that reflect the actual project structure.
When in doubt, ask the developer which layers their change will touch.