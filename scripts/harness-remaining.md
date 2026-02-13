# Harness Remaining Diagnostics

Snapshot source: `./scripts/verify-suite.sh` with `/tmp/wippy-local`.

## Current (`/tmp/wippy-local`)

Green targets:
- `~/wippy/wippy/tests/app`
- `~/wippy/framework/src/test`
- `~/wippy/framework/src/actor/test`
- `~/wippy/framework/src/bootloader`
- `~/wippy/framework/src/llm/src`
- `~/wippy/framework/src/llm/test`
- `~/wippy/framework/src/migration`
- `~/wippy/framework/src/relay/test`

Non-zero targets:
- `~/wippy/session` -> `errors=14`
- `~/wippy/framework/src/agent/src` -> `errors=3-4` (deterministic `claude:mapper` x3, intermittent `openai:embed` x1)
- `~/wippy/docker-demo/src` -> `errors=32`
- `~/wippy/framework/src/views` -> `errors=12`

## Baseline Comparison (`/tmp/wippy-fresh`)

- `session`: `19 errors + 1 warning` -> `14 errors + 0 warnings`
- `agent/src`: `13 errors` -> `3-4 errors`
- `docker-demo`: `42 errors` -> `32 errors`

`docker-demo` current diagnostics are a strict subset of baseline diagnostics (10 removed, 0 new).

`views` diagnostics are currently concentrated in:
- `wippy.views.api:list_components` (typed DTO assignment from unknown fields)
- `wippy.views.api:list_pages` (typed DTO assignment from unknown fields)
- `wippy.views:page_registry` and `wippy.views:renderer` (field-vs-method string concatenation diagnostics)
- `wippy.views:page_registry_test` (array index nil diagnostics on sorted loop assertions)

## LLM package verification note

- Canonical "current package" checks are:
  - `cd ~/wippy/framework/src/llm/src && ~/wippy/wippy-bin lint --cache-reset --json --level hint --ns 'wippy.llm.*'`
  - `cd ~/wippy/framework/src/llm/test && ~/wippy/wippy-bin lint --cache-reset --json --level hint --lock-file wippy.lock`
- Both are currently `0/0/0`.
- Running from repo root without package lock:
  - `cd ~/wippy && ~/wippy/wippy-bin lint --cache-reset --json --level hint --ns 'wippy.llm.*'`
  - can include test-entry diagnostics because it resolves a different lint closure.
- Root run with explicit llm lock resolves to clean package set:
  - `cd ~/wippy && ~/wippy/wippy-bin lint --cache-reset --json --level hint --ns 'wippy.llm.*' --lock-file framework/src/llm/src/wippy.lock`
  - currently `0/0/0`.

## Root-cause classes still open

1. App typing issues (not checker regressions):
- Optional ID returns used as required `string` without nil guard (`docker-demo` tests and service paths).
- `db:query` values returned as `any` then returned as typed records without explicit narrowing/cast.

2. Dependency/module contract mismatches in lint closure:
- `session` pulls `wippy.views`/`wippy.agent`/`wippy.llm` diagnostics via dependency analysis.
- Remaining `session`/`agent` issues are mostly in dependency entries, not local source files.
- `agent/src` lock file pins `wippy/llm@0.4.2`; those 8 diagnostics are emitted from locked dependency entries (`wippy.llm.*`), while `framework/src/llm/src` itself is `0/0/0` under current checker.

3. Checker behavior fixed in this branch:
- False positive class around field-write + truthy narrowing (`field_write_preserves_truthy_narrowing`).
- Return-summary reconciliation now prefers structured summaries over open-top placeholders in canonical merge.
- Assignment pre-state for self-referential RHS is now handled canonically in assignment checks.
- Mutable-record widening now accepts literal-union field values when every branch widens
  to the primitive supertype (`0|8000 <: integer`, `""|"alpha" <: string`).

## Current residual clusters

`session` (14):
- `wippy.agent.discovery:selector` x3
- `wippy.agent.tools:caller` x2
- `wippy.agent.tools:delay_tool` x1
- `wippy.agent:context` x1
- `wippy.session.process:message_handlers` x2
- `wippy.session.process:session` x1
- `wippy.views.api:list_components` x1
- `wippy.views.api:list_pages` x1
- `wippy.views:page_registry` x1
- `wippy.views:renderer` x1

`agent/src` (3-4):
- `wippy.llm.claude:mapper` x3 (stable)
- `wippy.llm.openai:embed` x1 (intermittent across repeated `--cache-reset` runs)

`docker-demo` (32):
- `app.docker.persist:images_test` x26
- `app.docker.persist:images` x3
- `app.docker.persist:containers` x1
- `app.docker.service:worker` x1
- `app:images_pull` x1
