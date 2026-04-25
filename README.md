# OpenBrief

OpenBrief is a local-first brief runtime for agents. The supported agent path is
a small `openbrief` JSON runner plus a single-file skill.

OpenBrief is designed for open-source distribution. This repository must not
contain personal source inventories, paywall policies, delivery logs, `.openclaw`
content, workspace backups, run history, or local SQLite databases.

## Install

Tell your agent:

```text
Install OpenBrief from https://github.com/yazanabuashour/openbrief.
Complete both required steps before reporting success:
1. Install and verify the OpenBrief runner binary with `openbrief --version`.
2. Register the OpenBrief skill from skills/openbrief/SKILL.md using your native skill system.
```

For the latest release:

```bash
sh -c "$(curl -fsSL https://github.com/yazanabuashour/openbrief/releases/latest/download/install.sh)"
```

For a pinned release:

```bash
OPENBRIEF_VERSION=v0.1.0 sh -c "$(curl -fsSL https://github.com/yazanabuashour/openbrief/releases/download/v0.1.0/install.sh)"
```

A complete install has two parts:

- `openbrief --version` succeeds
- the matching skill is registered from `skills/openbrief/SKILL.md`,
  `https://github.com/yazanabuashour/openbrief/tree/<tag>/skills/openbrief`,
  or `openbrief_<version>_skill.tar.gz`

Use the agent's native skill manager. OpenBrief does not require a specific
skill path or agent implementation.

## Upgrade

Tell your agent:

```text
Upgrade OpenBrief from https://github.com/yazanabuashour/openbrief.
Complete both required steps before reporting success:
1. Upgrade and verify the OpenBrief runner binary with `openbrief --version`.
2. Re-register the OpenBrief skill from skills/openbrief/SKILL.md using your native skill system.
```

Or upgrade the runner manually:

```bash
sh -c "$(curl -fsSL https://github.com/yazanabuashour/openbrief/releases/latest/download/install.sh)"
```

Then verify the runner and re-register the matching skill:

```bash
command -v openbrief
openbrief --version
```

## AgentOps Architecture

OpenBrief's agent-facing path is the AgentOps pattern: the skill gives the agent
task policy, and the local runner performs stateful brief operations through
structured JSON. Runtime configuration, latest-seen state, health warnings, and
delivery records live in the host SQLite database, not in this repository.

## Runner Interface

The skill sends structured JSON on stdin and reads structured JSON from stdout:

```bash
openbrief config
openbrief brief
```

Configuration actions manage sources and outlet policies. Sources are generic:
RSS/Atom feeds, GitHub releases, and optional feed-processing rules such as URL
canonicalization, outlet extraction, priority rank, dedup groups, and
always-report behavior. Brief actions run the brief, validate the runtime, and
record delivered messages for deduplication.

## Local Storage

The default SQLite path is
`${XDG_DATA_HOME:-~/.local/share}/openbrief/openbrief.sqlite`. Override it with:

- `OPENBRIEF_DATABASE_PATH`
- `--db` for explicit datasets and tests

`OPENBRIEF_DATABASE_PATH` is the only app-specific environment variable.
OpenBrief does not support a data-dir environment variable, workspace paths,
config-file environment variables, or repo-local state files.

## Development

Use the repo-pinned local toolchain for repository development:

```bash
mise install
mise exec -- gofmt -w .
mise exec -- golangci-lint run
mise exec -- go test ./...
mise exec -- ./scripts/validate-agent-skill.sh skills/openbrief
mise exec -- ./scripts/validate-release-docs.sh v0.1.0
```

The production runner supports RSS/Atom feeds, GitHub releases, generic URL
canonicalization strategies, outlet policy suppression, topic deduplication,
delivery-history suppression, and feed health warnings. End users configure
their own feed URLs, outlet policies, priorities, and always-report choices in
the host SQLite database; this repository does not seed personal sources or
policies.

## Eval Evidence

The production runner/skill is gated by agent evals documented in
[`docs/evals/agent-production.md`](docs/evals/agent-production.md). Current
release evidence lives in
[`docs/agent-eval-results/openbrief-v0.1.0-final.md`](docs/agent-eval-results/openbrief-v0.1.0-final.md).

## Releases

Tagged `v0.y.z` releases publish platform binary archives, the skill archive,
the installer, source archive, SHA256 checksums, an SPDX SBOM, and GitHub
attestations. Published release assets are intended to be immutable going
forward. See
[`docs/release-verification.md`](docs/release-verification.md) for verification
steps.

## Contributing

Outside contributors can work entirely through GitHub issues and pull requests.
Beads is maintainer-only workflow tooling and is not required for community
contributions.

See `CONTRIBUTING.md` for contribution expectations, `CODE_OF_CONDUCT.md` for
community standards, `SECURITY.md` for vulnerability reporting, and
`docs/maintainers.md` for maintainer-only workflow details.
