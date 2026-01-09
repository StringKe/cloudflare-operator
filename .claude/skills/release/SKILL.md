---
name: release
description: Release new version of cloudflare-operator. Handles version bump, changelog, git tag, and GitHub release. Use when releasing a new version, creating tags, or publishing releases.
allowed-tools: Read, Edit, Bash, Grep
user-invocable: true
---

# Release Process

## Overview

This skill guides the release process for cloudflare-operator, including version bump, git tag creation, and GitHub release.

## Pre-Release Checklist

```bash
# 1. Ensure all tests pass
make fmt vet test lint build

# 2. Check current version
grep "VERSION ?=" Makefile

# 3. Get latest tag
git tag --sort=-v:refname | head -5

# 4. Check for uncommitted changes
git status
```

## Release Steps

### Step 1: Determine Version

Follow semantic versioning:
- **MAJOR** (x.0.0): Breaking API changes
- **MINOR** (0.x.0): New features, backward compatible
- **PATCH** (0.0.x): Bug fixes, backward compatible

Current version format: `v0.17.X`

### Step 2: Update Version in Makefile

```bash
# Edit Makefile
sed -i '' 's/VERSION ?= .*/VERSION ?= 0.17.NEW/' Makefile
```

Or manually edit:
```makefile
VERSION ?= 0.17.NEW
```

### Step 3: Commit Version Bump

```bash
git add Makefile
git commit -m "chore: bump version to 0.17.NEW

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>"
```

### Step 4: Create Annotated Tag

```bash
git tag -a v0.17.NEW -m "v0.17.NEW: Brief description

## Changes

### Features
- Feature description

### Bug Fixes
- Fix description

### Documentation
- Doc changes

## Breaking Changes
None (or list breaking changes)"
```

### Step 5: Push to GitHub

```bash
git push origin main
git push origin v0.17.NEW
```

### Step 6: Verify Release

```bash
# Check workflow status
gh run list --limit 5

# Check release created
gh release list --limit 3

# View release details
gh release view v0.17.NEW
```

## Release Workflow

The CI automatically:
1. Builds Docker images (amd64, arm64)
2. Pushes to `ghcr.io/stringke/cloudflare-operator:VERSION`
3. Generates installer manifests
4. Creates GitHub Release with assets:
   - `cloudflare-operator.yaml`
   - `cloudflare-operator.crds.yaml`

## Rollback

If release fails:

```bash
# Delete local tag
git tag -d v0.17.NEW

# Delete remote tag (if pushed)
git push origin :refs/tags/v0.17.NEW

# Revert version commit
git revert HEAD
```

## Release Notes Template

```markdown
## v0.17.X

### Features
- feat(component): Description (#PR)

### Bug Fixes
- fix(component): Description (#PR)

### Documentation
- docs: Description

### Breaking Changes
- **BREAKING**: Description of breaking change

### Migration Guide
Steps to migrate from previous version (if applicable)

### Full Changelog
https://github.com/StringKe/cloudflare-operator/compare/v0.17.PREV...v0.17.X
```

## Hotfix Release

For urgent fixes:

```bash
# 1. Create hotfix branch (optional)
git checkout -b hotfix/v0.17.X

# 2. Apply fix
# ... make changes ...

# 3. Fast-track release
make fmt vet test
git add -A
git commit -m "fix(component): urgent fix description"
git checkout main
git merge hotfix/v0.17.X

# 4. Release immediately
# Follow standard release steps
```

## Version History

Check recent versions:
```bash
git tag --sort=-v:refname | head -20
```

View changes between versions:
```bash
git log v0.17.PREV..v0.17.NEW --oneline
```
