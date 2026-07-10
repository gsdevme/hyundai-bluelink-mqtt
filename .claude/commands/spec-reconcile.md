---
description: Reconcile docs/specs (REQUIREMENTS.md REQ-* IDs) against the codebase and report drift. Read-only — never edits code.
---

# /spec-reconcile

Reconcile the spec-driven requirements against the actual implementation and produce a
**drift report**. This command is **read-only**: it inspects and reports; it must not
modify code, specs, or tests.

## Inputs

- `docs/specs/REQUIREMENTS.md` — the authoritative list of `REQ-*` IDs, each with a
  one-line requirement and a `→ file` implementation pointer.
- The other `docs/specs/*.md` for detail behind each requirement.
- The Go source tree (`internal/`, `cmd/`, `main.go`), `features/`, `Dockerfile`,
  `go.mod`, `.claude/`.

## Procedure

1. **Parse requirements.** Extract every `REQ-*` ID, its text, and its `→` target(s)
   from `REQUIREMENTS.md`.
2. **Locate implementations.** For each requirement, check the pointed-to file exists
   and contains code plausibly satisfying it (function/type/const, endpoint path, env
   var, topic, header — grep for the concrete tokens named in the requirement). Note
   whether a corresponding test exists.
3. **Scan for untraceable code.** Walk the source tree for significant units
   (exported types/functions, HTTP handlers, endpoints, env-var reads, MQTT topics) and
   check each maps to at least one `REQ-*`. Flag anything with no traceable requirement.
4. **Detect mismatches.** Where a requirement names a specific constant, path, header,
   default, or behaviour, verify the code matches (e.g. `POLL_INTERVAL` default `30m`,
   `redirect_uri` value, `ccs2/carstatus` is a **GET**, odometer `state_class: total`).
   Flag divergences.

## Output — reconciliation report

Print a Markdown report with these sections (omit empty ones):

- **Summary** — counts: total requirements, implemented, missing, partial; untraceable
  code units; mismatches.
- **Missing** — `REQ-*` with no plausible implementation. Include the pointer and what
  was searched for.
- **Partial / untested** — implemented but no test, or only partially satisfied.
- **Untraceable code** — code units with no `REQ-*`; suggest either a new requirement
  or that the code is dead/out-of-scope.
- **Mismatches** — requirement says X, code does Y (with `file:line`).
- **Verified** — a compact checklist of `REQ-*` confirmed implemented (+ test).

## Rules

- Do **not** edit code, specs, tests, or config. Reporting only.
- Cite `file:line` for every finding so items are actionable.
- Prefer concrete grep evidence over assumptions; if uncertain, mark **partial** and say
  what to check manually rather than guessing.
- If `REQUIREMENTS.md` and code disagree on an intentional design change, recommend
  updating the spec — but leave the change to the human.
