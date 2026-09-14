---
name: az-pr-create
description: >
  Creates Azure DevOps pull requests from the command line using the Azure CLI (az repos).
  Use this skill when the user wants to raise a PR on Azure DevOps or Azure Repos, whether
  via the az CLI directly or as the next step after completing a release or feature branch.
  Also triggers when the user asks how to write a PR description, link work items to a PR,
  or sync a branch before raising a PR — in an Azure DevOps context.
---

# Azure DevOps PR Creation Skill

> **Platform check:** This skill is for **Azure DevOps (Azure Repos)** using the `az` CLI. If the user hasn't specified their platform and it's ambiguous, ask: *"Are you using Azure DevOps, or a different platform like GitHub or GitLab?"* — the commands differ significantly.

> **Execution mandate:** Invoking this skill is an explicit instruction to create the PR — do not present a plan and wait for confirmation. Run each step immediately. The only valid pause points are: (a) a required value is missing and cannot be inferred, or (b) a command fails and needs diagnosis. If the CLI is not set up, see `references/prerequisites.md`.

---

## Step 0 — Discover Context

```bash
git branch --show-current
```

Parse the branch name as `<prefix>/<slug>`. Use the prefix and slug to infer PR context:

### `release/<version>`

- **Suggested title:** `chore(release): <version>`
- **Description source:** find the changelog (`find . -maxdepth 1 -type f | xargs -r grep -l "\[Unreleased\]" 2>/dev/null`) and read the `## [<version>]` entry.

### `<prefix>/<slug>` — any other prefixed branch

- **Suggested title:** `<prefix>(<slug>): <short description>` — read the intent from `docs/specs/implemented/<slug>/index.md`, falling back to `docs/specs/active/<slug>/index.md`.
- **Description source:** same `index.md` — use the intent, affected areas, and phase summary. If no spec exists, ask the developer.

### No slug

Ask the developer for title and description content.

---

## Step 1 — Sync the branch

```bash
git fetch origin
git pull origin <branch>
```

**Do not merge or rebase locally before checking branch policy.** Azure DevOps repo policies often mandate a specific merge strategy and may forbid force-pushes. If unsure, skip local integration — the PR merge policy will handle it. Only integrate locally if the developer confirms it is permitted.

If confirmed:

| Strategy | Command |
|---|---|
| Merge | `git merge origin/<target>` then `git push origin <branch>` |
| Rebase | `git rebase origin/<target>` then `git push --force-with-lease origin <branch>` |

---

## Step 2 — Write the PR description file

Create `pr-description.md` in the repo root. Populate from the source identified in Step 0. See `references/templates.md` for templates.

> **Windows encoding — sanitise before writing.** On Windows hosts with cp1252 encoding, non-ASCII characters are silently corrupted when the `az` CLI reads the file. Applies to all sourced content (specs, changelogs). Replace before writing and tell the developer what was substituted:
>
> | Replace | With |
> |---|---|
> | Em dash `—` | `--` |
> | En dash `–` | `-` |
> | Smart quotes `"` `"` | `"` |
> | Smart apostrophes `'` `'` | `'` |
> | Ellipsis `…` | `...` |

---

## Step 3 — Create the PR

```
az repos pr create --source-branch <branch> --target-branch <target> --title "<title>" --description "@pr-description.md" --open
```

> **If the description shows only the first line:** `az repos pr create` truncates multi-line descriptions at the first newline. Patch immediately after creation:
> ```
> az repos pr update --id <pr-id> --description "@pr-description.md"
> ```
> Do not use `(Get-Content pr-description.md -Raw)` — PowerShell passes it as a shell string and hits the same truncation.

---

## Step 4 — Link work items (optional)

```
az repos pr work-item add --id <pr-id> --work-items <work-item-id>
```

Multiple work items: space-separated — `--work-items 123 456`.