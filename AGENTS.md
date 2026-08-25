# AGENTS.md

This file provides guidance to Claude Code, Codex, GitHub Copilot, and other AI coding agents
working in this repository.

## About This Project

`game-room-data.go` is the database access layer for the Game Room microservice: repository functions
for library, wishlist, and table, plus visibility resolution, translating between
`game-room-objects.go` persistence models and their API value objects.

## Dependencies

Depends on `api-core.go` (query param parsing/tracing), `game-room-objects.go` (models/VOs),
`common.go` (logging), `model-core.go` (Property/Tag conversion), and `mongodb.go` (database
access). Depended on by `game-room-api`.

## Committing Code

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>
```

## Branches and Workflow

* `develop` - integration branch, default branch, target for all PRs.
* `master` - latest released state, nothing committed directly.
* `feature/*`, `fix/*` branched from `develop`; `hotfix/*` branched from `master`.

See `CONTRIBUTING.md` for the full workflow, including running the database-backed test suite
locally.

## Running Checks Locally

```bash
docker run --rm -d -p 27017:27017 --name mongodb-test mongo:7.0
export TEST_DB_URI="mongodb://localhost:27017/unit-tests"
export TEST_COLLECTION=unit-tests
go build -v ./...
go vet ./...
go test -v -coverprofile coverage.out ./...
docker stop mongodb-test
```

## Releases

See `RELEASE.md`. Summary: trigger `prepare-release.yaml` (`workflow_dispatch` against
`develop`), which computes the next version from conventional commits via git-cliff and opens
a `release/<version>` PR into `master`. Merging that PR tags the release
(`tag-release.yaml`), which triggers `release.yaml` - re-runs tests, creates a GitHub
Release, and merges `master` back into `develop`.
