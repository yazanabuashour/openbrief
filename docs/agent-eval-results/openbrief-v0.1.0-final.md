# OpenBrief Agent Eval openbrief-v0.1.0-final

Harness: `codex exec --json --full-auto from throwaway run directories; single-turn scenarios use --ephemeral, multi-turn scenarios resume a persisted eval session with explicit writable eval roots`.

- Run root: `<run-root>`
- Isolated Codex home: `<run-root>/codex-home`
- Scenarios: `11`
- New session files: `2`
- Elapsed seconds: `410.62`

## Results

| Scenario | Passed | Assistant | Database | Tools | Commands | Hygiene |
| --- | --- | --- | --- | ---: | ---: | --- |
| `empty-config-rejects-run-brief` | `true` | `true` | `true` | `4` | `4` | `clean` |
| `rss-source-first-run-candidate` | `true` | `true` | `true` | `8` | `8` | `clean` |
| `github-release-source-must-include` | `true` | `true` | `true` | `8` | `8` | `clean` |
| `repeat-run-no-new-items` | `true` | `true` | `true` | `10` | `10` | `clean` |
| `record-delivery-suppresses-repeats` | `true` | `true` | `true` | `10` | `10` | `clean` |
| `rss-source-generic-processing-fields` | `true` | `true` | `true` | `8` | `8` | `clean` |
| `outlet-policy-watch-audit` | `true` | `true` | `true` | `16` | `16` | `clean` |
| `feed-failure-health-footnote` | `true` | `true` | `true` | `8` | `8` | `clean` |
| `feed-recovery-resolves-warning` | `true` | `true` | `true` | `14` | `14` | `clean` |
| `invalid-source-config-rejects` | `true` | `true` | `true` | `6` | `6` | `clean` |
| `routine-agent-hygiene` | `true` | `true` | `true` | `4` | `4` | `clean` |

## Gate

Recommendation: `ship_openbrief_runner_production`.

Raw Codex logs, copied repositories, local SQLite databases, caches, and isolated session stores are intentionally not committed. Reduced artifacts use `<run-root>` placeholders.
