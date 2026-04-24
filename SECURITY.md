# Security

Do not report private brief data through public issues. OpenBrief stores runtime
configuration and state in local SQLite databases controlled by the operator.

Security-sensitive areas include runner JSON validation, SQLite schema changes,
network fetch providers, and skill instructions that prevent bypassing the
runner.
