# GitHub Actions CI/CD Design

**Date:** 2025-01-06
**Status:** Approved

## Overview

GitHub Actions CI/CD pipeline for obsidian-notion-sync with static code analysis, security scanning, and automated releases.

## Design Decisions

### Triggers
- **All pushes + PRs**: Catches issues early in feature branches
- **Tag-based releases**: Push `v*` tags to trigger release builds

### Security Scanning Strategy
- **Feature branches**: golangci-lint (includes gosec) + govulncheck
- **Main branch**: Full stack (golangci-lint + govulncheck + CodeQL)
- **Rationale**: Fast feedback on branches, thorough protection on main

### E2E Testing
- Runs on PRs and main when secrets are available
- Gracefully skips for external contributors (no secret access)
- Requires `NOTION_TOKEN` and `NOTION_TEST_PAGE_ID` GitHub Secrets

### Release Strategy
- Matrix builds on native runners (macOS + Linux)
- No CGO cross-compilation—builds natively on each platform
- Produces binaries for: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64
- Automatic GitHub Release creation with checksums

### Go Version
- Single version (latest stable)
- Matches development environment

## Workflow Files

### ci.yml
Runs on all pushes and PRs.

**Jobs (parallel except where noted):**
1. `lint` - golangci-lint with existing .golangci.yml
2. `test` - Unit tests with race detection and coverage
3. `govulncheck` - Dependency vulnerability scanning
4. `e2e` - Integration tests (conditional on secrets)
5. `build` - Verification build (waits for lint+test)

### security.yml
Runs on main pushes + weekly schedule.

**Jobs:**
1. `codeql` - GitHub CodeQL SAST analysis

### release.yml
Runs on `v*` tags.

**Jobs:**
1. `build-matrix` - Native builds on macOS and Linux runners
2. `release` - Creates GitHub Release with all artifacts

### dependabot.yml
Weekly updates for Go modules and GitHub Actions.

## CI Matrix

| Event | Lint | Test | govulncheck | E2E | Build | CodeQL |
|-------|------|------|-------------|-----|-------|--------|
| Feature branch push | ✓ | ✓ | ✓ | ✓* | ✓ | ✗ |
| PR to main | ✓ | ✓ | ✓ | ✓* | ✓ | ✗ |
| Merge to main | ✓ | ✓ | ✓ | ✓* | ✓ | ✓ |
| Tag v* | — | — | — | — | Release | — |

*E2E runs only when secrets are available

## Setup Requirements

### GitHub Secrets
- `NOTION_TOKEN` - Notion integration token
- `NOTION_TEST_PAGE_ID` - Page ID for E2E tests

### Repository Settings
- Actions → General → Workflow permissions: "Read and write permissions"
- Allow GitHub Actions to create releases

## Estimated CI Times
- Feature branch: ~3-4 min (parallel jobs)
- Main merge: ~5-6 min (adds CodeQL)
- Release: ~5 min (parallel matrix builds)
