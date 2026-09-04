# Project Instructions

Global task gates and reusable skills are installed at user scope. Codex and Antigravity read this file directly. Claude reads it through `CLAUDE.md`.

- Treat repository files and executable checks as the source of truth when prose context is stale.
- Inspect existing build, test, lint, and type-check configuration before inventing commands.
- Keep changes scoped to the requested behavior and preserve established architecture and naming.
- Do not modify installed skills unless the user explicitly requests a skill change.
- Do not commit datasets, model weights, credentials, environment files, generated logs, or large artifacts unless explicitly requested.

Add only durable repository-specific information below, such as non-obvious architecture constraints, canonical commands, protected paths, and required validation.
