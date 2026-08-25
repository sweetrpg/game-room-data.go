# Contributing

Thanks for considering a contribution to `catalog-data.go`.

## Branching

This repo follows the sweetrpg platform's git-flow convention:

* `develop` is the integration branch. All feature and fix branches merge here.
* `master` reflects the latest released state. Nothing is committed here directly.
* Branch names: `feature/<description>` for new functionality, `fix/<description>` for bug
  fixes, `hotfix/<description>` for urgent fixes to a released version.

```bash
git checkout develop
git pull
git checkout -b feature/my-change
# ... work, commit ...
git push -u origin feature/my-change
# open a PR: feature/my-change -> develop
```

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>
```

## Running checks locally

Tests need a live MongoDB instance:

```bash
docker run --rm -d -p 27017:27017 --name mongodb-test mongo:7.0

export TEST_DB_URI="mongodb://localhost:27017/unit-tests"
export TEST_COLLECTION=unit-tests
export LOG_LEVEL=DEBUG

go build -v ./...
go vet ./...
go test -v -coverprofile coverage.out ./...
go tool cover -func coverage.out

docker stop mongodb-test
```

## Pull requests

CI runs automatically on PRs targeting `develop`, provisioning a MongoDB 7.0 instance for the
test suite. Once checks pass and the PR is reviewed, it can be merged (auto-merge is enabled
once required checks pass).

## Releases

Versions are tagged automatically from `develop` on merge (patch bump by default). Use the
"Bump version" workflow (`workflow_dispatch`) to cut a minor or major version instead.
