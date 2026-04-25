# Contributing

Use the production runner and tests when changing OpenBrief behavior:

```bash
mise exec -- gofmt -w .
mise exec -- golangci-lint run
mise exec -- go test ./...
mise exec -- ./scripts/validate-agent-skill.sh skills/openbrief
```

Do not commit personal source inventories, `.openclaw` content, workspace
backups, delivery logs, run history, or local SQLite databases.

When a change affects the public runner, skill, release, or security contract,
update the matching docs and release notes. Before tagging a release, run:

```bash
mise exec -- ./scripts/validate-release-docs.sh <tag>
```

Maintainers use Beads for local task tracking, but outside contributors can work
entirely through GitHub issues and pull requests.
