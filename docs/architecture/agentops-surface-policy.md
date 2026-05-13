# AgentOps Surface Policy

## Status

Accepted planning decision for `ob-qbu`, `ob-wgw`, and `ob-48t`.

This decision follows `docs/architecture/openclerk-pattern-adoption-audit.md`.
It does not authorize runner API, skill, schema, storage, release, or CI
changes.

## Skill-Size Budget

`skills/openbrief/SKILL.md` should stay an activation, routing, safety, and
minimal request-shape document. The current skill is about 144 lines and 6.4 KB,
which is acceptable but close enough to need an explicit budget.

Budget:

- target: keep the core skill at or below 175 lines and 8 KB
- review threshold: any change that would push the skill above either target
  must explain why runner JSON, compact help, eval docs, or maintainer docs are
  insufficient
- migration threshold: any change that would push the skill above 220 lines or
  10 KB must include a follow-up Bead to move durable workflow detail out of
  the skill

Routing policy:

- Runner JSON results should carry data needed to answer routine brief and
  configuration tasks.
- Runner rejections should carry production-safe explanations for unsupported
  or invalid requests.
- Compact help can document stable command/action shapes when repeated
  exact-JSON ceremony appears but a new runner action is not justified.
- Eval docs should hold prompt choreography, scenario evidence, taste flags,
  and promotion or deferral rationale.
- Maintainer docs should hold release, review, security, Beads, and repository
  administration workflows.
- The skill should not become a long-lived workaround for missing runner-owned
  workflow surfaces.

## Discovery Surface Comparison

| Candidate | Fit | Risk | Decision |
| --- | --- | --- | --- |
| Current primitives plus the existing skill | Good for proven `config` and `brief` actions. | Exact JSON and answer-shape ceremony can grow in the skill. | Keep as baseline. |
| Compact runner help | Good when users need stable action shapes but not new behavior. | Help text can drift into a second skill if it becomes too broad. | Candidate for future eval/design rows. |
| Read-only `capabilities` command | Good if agents repeatedly need machine-readable supported actions. | Adds runner API surface before evidence proves status quo or help is insufficient. | Do not promote yet. |
| Narrow workflow guide action | Good if a small set of routine flows repeatedly need candidate assembly guidance. | Can become a soft implementation surface that bypasses clearer runner actions. | Do not promote yet. |

Outcome: no new discovery API is selected yet. Existing specific follow-up
Beads should compare candidate surfaces where evidence is concrete:

- source capture and field-policy candidates: `ob-rab`, `ob-8nt`, `ob-k92`,
  `ob-ivy`, `ob-776`, `ob-7an`, and `ob-zxn`
- high-touch handoff candidates: `ob-1qj`, `ob-z19`, and `ob-brh`

Those follow-ups should compare status quo JSON, compact help, and runner-owned
surfaces before any implementation promotion.

## Committed-Artifact Validation

Existing coverage:

- `scripts/validate-release-docs.sh` requires release-note shape, matching
  `CHANGELOG.md` links, and no hard-wrapped release prose.
- `scripts/validate-agent-skill.sh` validates the single-file skill contract,
  frontmatter, skill-local links, required runner guidance, and known forbidden
  legacy/private guidance.
- `scripts/validate-all-agent-skills.sh` keeps future skill directories from
  bypassing validation.
- `.github/workflows/pull-request.yml` and `.github/workflows/release.yml` run
  formatting, lint, tests, and all-skill validation.
- `.github/PULL_REQUEST_TEMPLATE.md`, `AGENTS.md`, and `docs/maintainers.md`
  require repo-relative paths or neutral placeholders and call out private-state
  exclusions.

Assessment:

- Public release artifacts and the shipped skill have enough committed
  validation for the current surface.
- A repo-wide private-state or absolute-path validator is not adopted yet. The
  current docs intentionally mention examples such as local database filenames,
  private-state categories, and neutral placeholders; a broad string denylist
  would likely create false positives without evidence of repeated misses.
- If review misses recur in committed docs or eval reports, prefer a narrow
  validator for that artifact class over a broad repository denylist.

This assessment preserves the existing review requirement: committed docs,
reports, and artifact references must use repo-relative paths or neutral
placeholders, and private source inventories, outlet policy evidence, delivery
logs, run history, workspace backups, and local SQLite databases must stay out
of the repository.
