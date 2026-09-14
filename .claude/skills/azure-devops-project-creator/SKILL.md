---
name: azure-devops-project-creator
description: >
  Analyse a documentation bundle directory and create the corresponding Azure DevOps project,
  repositories, and committed documentation via the ADO REST API — following the provided
  documentation exactly with no divergence. Use this skill whenever the user wants to scaffold
  an Azure DevOps project from a document bundle, points Claude at a directory of project
  documentation, asks to "set up ADO from my docs", "create the repos from my bundle",
  "provision Azure DevOps", or uploads or references a folder of architecture and SRS documents.
  The skill begins with deep directory and content analysis before taking any action.
---

# Azure DevOps Project Creator

Creates an Azure DevOps project and repositories from a documentation bundle, committing
documentation exactly as the provided documents direct. Entirely driven by document content —
no naming conventions, structural defaults, or assumptions are imposed.

> **Guiding principle**: The documentation is the specification. Read it, understand it,
> execute it. If something is ambiguous, stop and ask — never decide unilaterally.

Before any other action, set up the isolated Python environment and install all dependencies.
See `references/environment.md` for `setup_environment()`, `activate_venv()`, and
`teardown_environment()`. Wrap the entire workflow in a `try/finally` to guarantee cleanup.

---

## Phase 1 — Directory Discovery

Establish the working directory: use the path provided by the user, or ask them to confirm
one if not specified. Call `scan_directory(root)` (see `references/bundle-analysis.md`) to
recursively list all files. Report a brief summary — file counts by extension, total
directories — then proceed to content analysis.

---

## Phase 2 — Content Analysis

Read every file using `read_file(path)` (see `references/bundle-analysis.md`), building the
`project_model` (schema in `references/bundle-analysis.md`) incrementally as signals are
found. Images are noted by filename only. Files over 10 MB are flagged but attempted.
This phase is never rushed — full comprehension of the bundle comes first.

**Project identity** — explicit project name, organisation/programme name, process template
(Agile/Scrum/CMMI), visibility (public/private).

**Repository definitions** — any table, list, or section explicitly naming repositories;
repo names (often in backticks or code blocks); descriptions and primary responsibilities;
creation/implementation order; statements distinguishing a repo from a mere component.

**Document roles** — which document is the authoritative ADO project architecture (defines
the repo list); which documents are per-repo SRS/implementation plans; which are project-level
architecture docs (system design, deployment diagrams, graphs); which are governance docs
(runbooks, ADRs); which image files are architecture diagrams.

**Folder/structure signals** — any "Suggested Repository Structure", folder trees, storage
policies, or explicit paths stating where documents should live.



## Phase 3 — Synthesis and Mapping

### Identify the authoritative repo-definition document

Look for: explicit `Azure DevOps Project:` statements; a table or ordered list of repo names
with descriptions; language like "repository separation", "ADO layout", "project structure".
If multiple documents contain repo lists, identify which is most authoritative (typically an
index, pack, or project architecture document). If genuinely ambiguous, present candidates
and ask the user.

### Match SRS files to repos

For each repo, find its corresponding SRS or implementation plan. Apply heuristics in order,
stop at first confident match:

1. File content contains the repo name prominently (title, heading, or opening statement)
2. Normalised filename matches repo name — strip numeric prefixes, convert underscores to
   hyphens, remove common suffixes (`_implementation_plan`, `_srs`, `_spec`)
3. File resides in a subdirectory named for the repo or its subject area
4. File explicitly states it is the spec/plan for that repo

If no confident match, mark as unresolved.

### Identify the docs/governance repo

This repo receives all project-level documentation. Identify it by its description
mentioning: documentation, ADRs, runbooks, traceability, architecture, SRS preservation,
governance. It is typically last in implementation order.

Once identified, read its own SRS/implementation plan and extract its **"Suggested Repository
Structure"** (or equivalent) — this defines the exact folder paths for all project-level
documents committed to it. Follow this structure precisely; do not invent paths.

### Map project-level documents to docs repo paths

Using the docs repo's folder structure, assign each project-level file (`.md` and `.png`
only) to its target path. If a document's destination is unclear from both the folder
structure and the document's content, mark it as unresolved.

Silently skip `.docx` and `.pdf` — warn once per file type at the end of the summary:
```
⚠️  Skipped N .docx and N .pdf files — only .md and .png source files are committed.
```

### Generate the project landing page README

ADO automatically creates a default repository named after the project whenever a new project
is provisioned. This repo receives a single `README.md` that acts as the developer entry
point for the entire ADO project.

Compose this README entirely from the analysed bundle documents — do not invent content.
It must contain:

- **Project title and purpose** — one paragraph drawn from the system design document.
- **Architecture framing** — one or two sentences capturing the deployment model (e.g. the
  Azure public layer / on-premise operational core split) taken from the documents.
- **Repository map** — the complete repo table from the authoritative repo-definition
  document: repo name, primary responsibility, implementation order.
- **Where to go next** — a pointer to the docs/governance repo for architecture documents,
  SRS pack, ADRs, runbooks, and traceability.
- **Key documents** — the names of the most important files committed to the docs repo
  (system design, applied graph, SRS pack index, implementation plan index), with their
  committed paths so they are easy to locate.

Store the generated content string in `project_model["landing_page_readme"]`.

---

## Phase 4 — User Confirmation

Present the complete plan before any API call. Every proposed path must trace to a source.
All unresolved items must be listed. Do not proceed until the user explicitly confirms.

```
══════════════════════════════════════════════
  ANALYSIS COMPLETE — PROPOSED ADO SETUP
══════════════════════════════════════════════

Project:   [name]  ·  [template]  ·  [visibility]
Source:    [architecture doc filename]

DEFAULT REPO: [project-name]  (auto-created by ADO)
  /README.md  ←  generated from bundle documents
  Preview:
    # [Project Title]
    [purpose paragraph]
    ## Repositories
    | Repo | Responsibility | Order |
    ...
    ## Documentation
    All architecture, SRS, ADRs, and runbooks are in `[docs-repo-name]`.

DOCS REPO: [repo-name]
  Structure from: [impl plan filename]
  [repo-path]  ←  [local file path]
  [repo-path]  ←  [local file path]
  ...

APPLICATION REPOS (implementation order):
  N. [repo-name]
     SRS: [filename] → /docs/[target filename]
  ...

⚠️  UNRESOLVED (must resolve before proceeding):
  - [description of ambiguity]

Proceed? (yes / edit / cancel)
```

Validate that all source files exist on disk before presenting this summary. Report any
missing files here rather than at execution time.

---

## Phase 5 — Execution

Call `load_credentials()` (see `references/environment.md`) to retrieve `org_url` and `pat`
from environment variables. If either is missing, the function raises a clear error with
setup instructions — do not prompt for values in chat.
All API functions are defined in `references/api-patterns.md`.

### Create the project
Call `create_project(org_url, name, description, template, visibility, pat)`.
If the project already exists, confirm with the user before continuing.

### Commit the landing page README
ADO creates a default repository named after the project immediately on project creation.
Retrieve it with `get_repo(org_url, project, project_name, pat)` — it exists as soon as
`create_project` returns. Encode `project_model["landing_page_readme"]` as UTF-8 bytes,
base64-encode the result, and push it as `/README.md`:

```python
content = base64.b64encode(project_model["landing_page_readme"].encode("utf-8")).decode()
change = {
    "changeType": "add",
    "item": {"path": "/README.md"},
    "newContent": {"content": content, "contentType": "base64Encoded"}
}
push(org_url, project, default_repo["id"], [change], "Initial commit: project landing page", pat)
```

### Create repositories
Call `create_repo(org_url, project, name, pat)` for each repo in the exact order
defined by the architecture document. Collect each returned `repo["id"]` for use in commits.

### Commit documents
For each file in the commit plan, call `build_change(repo_path, local_path)` to construct
the change entry, then `push(org_url, project, repo_id, changes, message, pat)` to commit.

**Docs repo**: pass all project-level file changes in a single `push` call.
**Each application repo**: pass its matched SRS file change in a single `push` call.

### Windows print encoding
All `print()` calls in the execution script must use **ASCII-only** status strings — never
emoji (`✅`, `❌`, `⚠️`). On Windows the Python process inherits the console's `cp1252`
encoding, which cannot encode those characters; the resulting `UnicodeEncodeError` will abort
execution mid-run, leaving subsequent steps unexecuted. Use `[OK]`, `[FAIL]`, `[WARN]`
instead. The Phase 6 report (rendered in chat) may use emoji freely — only `print()` output
written to the Windows terminal must stay ASCII.

---

## Phase 6 — Results Report

```
══════════════════════════════════════════════
  EXECUTION COMPLETE
══════════════════════════════════════════════

✅ Project: [name]  [created / already existed]

✅ [project-name]  (default repo)  →  /README.md committed

[docs-repo-name]  [created / existed]
  ✅ [repo-path committed]
  ✅ ...

Repos (order per architecture doc):
  ✅ N. [repo-name]  [created / existed]  →  /docs/srs.md committed
  ❌ N. [repo-name]  FAILED: [reason]

⚠️  Skipped N .docx and N .pdf files.
```

---

## Error Handling

| Situation | Action |
|---|---|
| No clear authoritative repo-definition document | Present candidates, ask user to identify it |
| Multiple documents claim to define repo structure | Ask user which takes precedence |
| SRS cannot be matched to a repo with confidence | Mark unresolved, ask user before proceeding |
| Docs repo cannot be identified | Ask user to name it explicitly |
| Docs repo has no folder structure section | Ask user to specify target paths |
| Repo name invalid for ADO | Flag, propose correction, wait for user approval |
| ADO auto-creates a default repo named after the project | Expected — retrieve its ID with `get_repo(org_url, project, project_name, pat)` immediately after `create_project` and commit `/README.md` to it |
| Project already exists | Skip creation, confirm with user before continuing |
| Repo already exists | Ask: skip or overwrite the commit? |
| Any source file missing | Report all missing at once, block execution |
| 401 Unauthorized | Verify `ADO_PAT` env var is set and the token has not expired |
| 403 Forbidden | Verify `ADO_PAT` has the required scopes (see api-patterns.md) |
| Async project creation times out | Retry poll up to 10× at 3s intervals |
| Push fails on stale SHA | Re-fetch SHA and retry once |
| Any unexpected API error | Report full response, offer retry or skip |
| `UnicodeEncodeError` in print output (Windows) | Caused by emoji in `print()` calls on a `cp1252` console — use ASCII status strings (`[OK]`, `[FAIL]`) in all script output; see Phase 5 note |
| `PermissionError` / `WinError 5` during venv teardown | `.pyd` files locked by the running Python process — use `cmd /c rmdir /s /q` with `ignore_errors=True` fallback; see `teardown_environment()` in `references/environment.md` |

---

## Reference Files

- `references/environment.md` — Virtual environment setup, dependency installation, and teardown
- `references/bundle-analysis.md` — Directory scanning, file reading, and project model schema
- `references/api-patterns.md` — ADO credentials, all API functions, PAT scopes, HTTP status codes