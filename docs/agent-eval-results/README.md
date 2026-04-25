# Agent Eval Results

Current recommendation:

- Use the production OpenBrief runner/skill for routine local brief tasks covered by the eval suite.
- Keep production agents on the installed `openbrief` JSON runner and shipped `skills/openbrief/SKILL.md`; do not use repo inspection, direct SQLite access, legacy scripts, or source-built command paths as routine fallbacks.
- Release gate: the production runner/skill must pass selected v0.1.0 scenarios with no direct SQLite access, broad repo search, repo/source inspection, environment inspection, or hidden evaluator-only instructions.

Top-level reports:

- `docs/agent-eval-results/openbrief-v0.1.0-final.md`

Raw Codex logs, copied repositories, temporary SQLite databases, caches, and isolated Codex homes are not committed. Reduced reports use neutral `<run-root>` placeholders.
