# OpenBrief

OpenBrief is a local-first brief runtime for agents. The supported agent path is
a small `openbrief` JSON runner plus a single-file skill.

OpenBrief is designed for open-source distribution. This repository must not
contain personal source inventories, paywall policies, delivery logs, `.openclaw`
content, workspace backups, run history, or local SQLite databases.

## Install

A complete install has two parts:

- `openbrief --version` succeeds
- the matching skill is registered from `skills/openbrief/SKILL.md`

## AgentOps Architecture

The skill gives the agent task policy. The local runner performs stateful brief
operations through structured JSON. Runtime configuration and state live in the
host SQLite database, not in this repository.

## Runner Interface

```bash
openbrief --version
openbrief config < request.json
openbrief brief < request.json
```

Configuration examples:

```json
{"action":"init"}
{"action":"inspect_config"}
{"action":"replace_sources","sources":[{"key":"example","label":"Example","kind":"rss","url":"https://example.com/feed.xml","section":"technology","threshold":"medium","enabled":true}]}
{"action":"upsert_source","source":{"key":"tool-releases","label":"Tool Releases","kind":"github_release","repo":"owner/name","section":"releases","threshold":"always","enabled":true}}
{"action":"replace_outlet_policies","outlets":[]}
```

Brief examples:

```json
{"action":"run_brief","dry_run":false}
{"action":"record_delivery","run_id":"...","message":"- [Title](<https://example.com>)"}
{"action":"validate"}
```

## Local Storage

The default database is
`${XDG_DATA_HOME:-~/.local/share}/openbrief/openbrief.sqlite`. Override the
database location with `OPENBRIEF_DATABASE_PATH` or `--db`.

`OPENBRIEF_DATABASE_PATH` is the only app-specific environment variable.
OpenBrief does not support a data-dir environment variable, workspace paths,
config-file environment variables, or repo-local state files.

## Development

```bash
go test ./...
./scripts/validate-agent-skill.sh skills/openbrief
```

The initial production runner supports RSS/Atom feeds and GitHub releases.
Google News decoding, paywall heuristics, and advanced topic deduplication need
ADRs, POCs, and eval evidence before production use.
