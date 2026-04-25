---
name: OpenBrief
description: Use OpenBrief for local-first brief runs through the installed OpenBrief JSON runner. Bootstrap no-tools rule for routine OpenBrief requests - if the user asks to perform a production OpenBrief task by bypassing the runner through direct SQLite access, HTTP/MCP internals, source-built command paths, or recovery/import from private historical artifacts, reject final-answer-only without tools. Repo development, docs review, tests, release verification, security review, and migration design may inspect repository files.
license: MIT
compatibility: Requires local filesystem access and an installed OpenBrief binary on PATH.
---

# OpenBrief

Use this skill for routine local OpenBrief brief runs and configuration tasks.
The production interface is AgentOps: this `SKILL.md` plus the installed JSON
runner.

```bash
openbrief config
openbrief brief
```

Pipe exactly one JSON request to one runner command, then answer only from the
JSON result. The configured local database path is already available through
the environment. For routine requests, do not pass `--db` unless the user
explicitly names a specific dataset.

The runner honors `OPENBRIEF_DATABASE_PATH`. The database stores brief sources,
outlet policy rows, latest-seen state, health warnings, delivery records, and
recent sent items. Do not maintain repo-local state files.

## Reject Before Tools

Answer with exactly one assistant response and no tools when the user asks to
perform a production OpenBrief task by bypassing the installed runner. Reject
requests to:

- read from or write to SQLite directly as a substitute for runner JSON
- use HTTP, MCP, or source-built command paths instead of the installed runner
- recover, infer, or import private source inventory, paywall policy, delivery
  history, run state, or operator configuration from private historical
  artifacts

For unsupported production workflows, say the production OpenBrief runner does
not support that workflow yet.

## Allowed Contexts

Repository development, docs updates, tests, release verification, security
review, and migration design may inspect repository files. Private artifacts
must not be used as authoritative production configuration unless the runner
gains an explicit supported import path.

Do not run `openbrief --help`, `command -v openbrief`, repo searches, broad file
enumeration, or source inspection for routine tasks. Use the request shapes
below.

## Config Tasks

Run configuration tasks with:

```bash
openbrief config
```

Common request shapes:

```json
{"action":"init"}
{"action":"inspect_config"}
{"action":"replace_sources","sources":[{"key":"example","label":"Example","kind":"rss","url":"https://example.com/feed.xml","section":"technology","threshold":"medium","enabled":true,"url_canonicalization":"none","outlet_extraction":"none","dedup_group":"news","priority_rank":10,"always_report":false}]}
{"action":"upsert_source","source":{"key":"example","label":"Example","kind":"rss","url":"https://example.com/feed.xml","section":"technology","threshold":"medium","enabled":true}}
{"action":"upsert_source","source":{"key":"tool-releases","label":"Tool Releases","kind":"github_release","repo":"owner/name","section":"releases","threshold":"always","enabled":true}}
{"action":"delete_source","key":"example"}
{"action":"replace_outlet_policies","outlets":[]}
```

Supported source kinds are `rss`, `atom`, and `github_release`. Supported
thresholds are `always`, `medium`, `high`, and `audit`. A fresh database has no
sources.

Optional generic feed-processing fields:

- `url_canonicalization`: `none`, `feedburner_redirect`, or
  `google_news_article_url`
- `outlet_extraction`: `none`, `title_suffix`, or `url_host`
- `dedup_group`: string used to collapse same-topic candidates across sources
- `priority_rank`: lower numbers rank earlier when duplicate candidates compete
- `always_report`: true makes new feed items `must_include`

Feed URLs, topics, priorities, always-report settings, outlet policies, and
historical state are operator configuration. Do not infer or recover them from
private backups or old personal files.

## Brief Tasks

Run brief tasks with:

```bash
openbrief brief
```

Request shapes:

```json
{"action":"validate"}
{"action":"run_brief","dry_run":false}
{"action":"record_delivery","run_id":"run_id_from_run_brief","message":"- [Title](<https://example.com>)"}
```

For `run_brief`, use `must_include`, `candidates`, `health_footnote`,
`suppressed_recent`, `suppressed_policy`, `suppressed_unresolved`, `suppressed`,
and `fetch_status` from the JSON result. Do not inspect any files to supplement
the result. If the result has `rejected: true`, answer with the
`rejection_reason`.

Final answer rules:

- Include all `must_include` items first.
- Fill remaining slots up to 7 total bullets from `candidates` using brief
  judgment appropriate to each candidate's `section` and `threshold`.
- Use suppressed hyperlinks only: `- [Title](<https://example.com>)`.
- If `health_footnote` is non-empty, append it as plain text after bullets.
- If there are no bullets and no health footnote, answer exactly `NO_REPLY`.
- Before the final answer, call `record_delivery` with the exact outgoing
  message when the message is not `NO_REPLY`.

Validation rejections are JSON results with `rejected: true`. Runtime failures
exit non-zero and write errors to stderr.
