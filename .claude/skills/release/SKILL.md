---
name: release
description: >
  Guides developers through the full release workflow: verifying the changelog, determining
  the version bump, cutting a release branch, promoting the changelog, bumping version files,
  and committing. Use this skill whenever someone says "cut a release", "do a release",
  "release this", "bump the version", "start a release", "prepare a release", "release x.y.z",
  or any phrase implying they want to move [Unreleased] changelog content into a versioned
  release. Works across all project types. Stops at a pushed release branch. Does not own
  PR creation or post-merge tagging.
---

# Release Skill

Guides a developer from an `[Unreleased]` changelog to a pushed release branch ready for a PR.
Assumes [Keep a Changelog](https://keepachangelog.com) format — adapt if the project differs.
Never work on the primary branch directly. Ask before proceeding if anything is ambiguous.

---

## Step 1 — Verify

```bash
git status                # must be clean; if dirty, stop and ask developer to commit or stash
git branch --show-current # must be on primary branch; if not, confirm before continuing; record it
git fetch origin
git status                # if behind remote, pull before proceeding:
```
```bash
git pull origin <primary-branch>  # only if behind
```

Record the primary branch name — used in Steps 3 and 6.

Discover the changelog file:
```bash
find . -maxdepth 1 -type f | xargs -r grep -l "\[Unreleased\]" 2>/dev/null
```
Stop if not found. Confirm `[Unreleased]` has at least one entry. Record the filename.

Discover version file(s):
```bash
find . -maxdepth 2 -type f \( \
  -name "VERSION" -o -name "pyproject.toml" -o -name "package.json" -o \
  -name "setup.cfg" -o -name "Chart.yaml" \
\) 2>/dev/null
```
These are common examples — any file that clearly owns the version string counts. Inspect
each candidate to confirm it contains a version declaration. Stop and ask if none found.
Record all version files — used in Steps 2, 5, and 6.

---

## Step 2 — Determine Version

Infer the semver bump from `[Unreleased]`:

| Signal | Bump |
|---|---|
| Breaking change (`### Breaking`, `BREAKING CHANGE`, removed/renamed public API) | **major** |
| New feature or addition (`### Added`, `### Changed` with new capability) | **minor** |
| Fixes, patches, docs, chores only (`### Fixed`, `### Security`, `### Deprecated`) | **patch** |

A `### Security` entry that also removes or renames a public API is **major**, not patch.

Read the current version from the file(s) recorded in Step 1. If multiple version files
disagree on the current version, ask the developer which is the source of truth before proceeding.

Confirm with the developer before continuing:
> *"Based on [Unreleased], I'm inferring a **[major/minor/patch]** bump: `x.y.z` → `a.b.c`. Does that look right?"*

---

## Step 3 — Cut Release Branch

```bash
git checkout -b release/x.y.z
```

Do not push yet — the branch is pushed in Step 6 after the release commit exists.

---

## Step 4 — Promote Changelog

Edit the changelog file. Replace the `## [Unreleased]` heading with the following two headings — the first restores a blank `[Unreleased]` for future entries, the second opens the new versioned block:
```markdown
## [Unreleased]

## [x.y.z] — YYYY-MM-DD
```
Move all entries from `[Unreleased]` under the new versioned heading, preserving sub-groupings.
Do not alter previously versioned entries.

Show the diff to the developer before proceeding. Re-edit and re-show if corrections are needed.

---

## Step 5 — Bump Version Files

Update the version string in every file recorded in Step 1. Match the existing format and
location — do not change surrounding structure. If a file's format is unfamiliar, show the
relevant section and ask rather than guessing.

Show the developer which files changed and their new values.

---

## Step 6 — Commit and Push

Stage only the files changed in Steps 4–5:
```bash
git add <changelog-file>
git add <version-file-1>  # repeat for each version file
git commit -m "chore(release): x.y.z"
git push -u origin release/x.y.z
```

Confirm the commit is clean and the branch is pushed, then inform the developer:
> *"Release x.y.z is ready on `release/x.y.z` — ready for a PR into `<primary-branch>` (as confirmed in Step 1)."*

---

## Summary

```
1. git status / fetch / pull — confirm clean; discover changelog + version files
2. Read [Unreleased] → infer bump → confirm version with developer
3. git checkout -b release/x.y.z
4. Promote [Unreleased] → [x.y.z] — YYYY-MM-DD; show diff
5. Bump all version files; show changes
6. git add <files> && git commit -m "chore(release): x.y.z" && git push
```

---

## Edge Cases

**Multiple version files disagree:** Ask which is the source of truth before bumping any.

**Free-form `[Unreleased]` (no sub-groupings):** Promote as-is; do not impose structure.

**Breaking change is ambiguous:** Ask the developer; do not default to `minor`.

**Already on a `release/*` branch:** Confirm the version, assess what's done, resume from the first incomplete step.