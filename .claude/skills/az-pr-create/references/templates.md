# PR Description Templates

Save the appropriate template as `pr-description.md` in the repo root.

---

## Feature / Fix / Chore (spec-driven branch)

Populate Summary and Changes from `docs/specs/implemented/<slug>/index.md` (intent,
affected areas, phase summary). Populate Validation from the spec's completion criteria.

```markdown
## Summary

<Feature intent — what this does and why.>

## Changes

- <Change 1>
- <Change 2>
- <Change 3>

## Validation

- <Completion criterion or verification step>

## Deployment notes

<Ordering dependencies or environment requirements. Leave blank if none.>
```

---

## Release (release/<version> branch)

Populate Changes directly from the `## [<version>]` changelog entry, preserving
sub-groupings (Added, Changed, Fixed, etc.).

```markdown
## Summary

Release <version>.

## Changes

### Added
- <entry>

### Changed
- <entry>

### Fixed
- <entry>

## Validation

- Pipeline passes on release branch
- Version files updated to <version>
- Changelog entry present and complete

## Deployment notes

<Post-merge steps, environment ordering, or migration requirements. Leave blank if none.>
```