# Contributing

Use the production runner and tests when changing OpenBrief behavior:

```bash
mise exec -- go test ./...
mise exec -- ./scripts/validate-agent-skill.sh skills/openbrief
```

Do not commit personal source inventories, `.openclaw` content, workspace
backups, delivery logs, run history, or local SQLite databases.
