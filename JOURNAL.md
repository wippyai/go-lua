# JOURNAL

## 2026-02-14T04:33:41Z
- Started active journal per request.
- Branch: `feature/unified-fixpoint-kernel`.
- Mode: run harness and investigation; no new edits beyond already-present go-lua refactor work.
- Latest full harness result (`./scripts/verify-suite.sh`):
  - `/home/wolfy-j/wippy/wippy/tests/app` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/session` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/test` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/actor/test` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/agent/src` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/bootloader` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/docker-demo` => `errors=44 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/llm/src` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/llm/test` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/migration` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/views` => `errors=27 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/relay/test` => `errors=0 warnings=0 hints=0`
- Confirmed direct repro for views with fresh rebuilt binary (`/home/wolfy-j/wippy/wippy-bin`): 27 errors in `wippy.views:page_registry_test` (`cannot index type nil` on `page.proxy...`).

## 2026-02-14T04:42:09Z
- Phase baseline rerun (`./scripts/verify-suite.sh`) completed.
- Results:
  - `/home/wolfy-j/wippy/wippy/tests/app` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/session` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/test` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/actor/test` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/agent/src` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/bootloader` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/docker-demo` => `errors=44 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/llm/src` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/llm/test` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/migration` => `errors=0 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/views` => `errors=27 warnings=0 hints=0`
  - `/home/wolfy-j/wippy/framework/src/relay/test` => `errors=0 warnings=0 hints=0`
- Confirmed failing views diagnostics remain `wippy.views:page_registry_test` (`page.proxy.*` after `test.not_nil(page.proxy)`).
- Confirmed `/home/wolfy-j/wippy/framework/src/views/.wippy` is absent in this environment; dependency-resolution/fallback behavior is under investigation.

## 2026-02-14T04:46:06Z
- Correlated `require("test")` usage vs harness outcomes.
- Counts across harness targets show broad usage outside views:
  - `session`: `require("test")=6`, `test.not_nil(...)=101`, lint 0/0/0
  - `framework/src/agent/src`: `12`, `377`, lint 0/0/0
  - `framework/src/llm/src`: `39`, `207`, lint 0/0/0
  - `framework/src/migration`: `2`, `5`, lint 0/0/0
  - `framework/src/views`: `2`, `24`, lint 27/0/0
  - `docker-demo`: `7`, `57`, lint 44/0/0
- Lock/config contrast:
  - Green dirs (`llm/src`, `migration`, `session`) have `wippy.lock` modules list including `wippy/test`.
  - Failing dirs (`views`, `docker-demo`) have `wippy.lock` with only `directories` and no `modules` list.
- Confirmed from lock implementation (`boot/deps/lock.GetLoadPaths`) that module load paths are derived from `lock.modules`; vendor contents are not auto-scanned when modules list is empty.
- Implication: in views/docker contexts, imports like `test: wippy.test:test` are declared in entry metadata but dependency manifests are not loaded through lock-based path expansion.

## 2026-02-14T04:48:06Z
- Controlled lock-completeness experiment (no harness edits):
  - Created temp lock in `framework/src/views` with explicit `modules` (`wippy/test`, `wippy/terminal`, `wippy/views`) + replacement.
  - Result: `framework/src/views` lint changed from `27` errors to `0` errors.
- Controlled lock-completeness experiment in `docker-demo`:
  - Created temp lock with explicit `modules` (`wippy/test`, `wippy/terminal`).
  - Result: `docker-demo` lint changed from `44` errors to `38` errors.
- Conclusion from controlled runs:
  - `views` failures are lock/dependency-resolution driven (not checker core behavior).
  - `docker-demo` has a mixed profile: part lock/dependency-resolution noise removed, plus remaining checker/code issues (38).

## 2026-02-14T04:50:28Z
- Full harness rerun after lock-path analysis phase (`./scripts/verify-suite.sh`).
- Results unchanged from baseline:
  - `/home/wolfy-j/wippy/wippy/tests/app` => `0/0/0`
  - `/home/wolfy-j/wippy/session` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/test` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/actor/test` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/agent/src` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/bootloader` => `0/0/0`
  - `/home/wolfy-j/wippy/docker-demo` => `44/0/0`
  - `/home/wolfy-j/wippy/framework/src/llm/src` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/llm/test` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/migration` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/views` => `27/0/0`
  - `/home/wolfy-j/wippy/framework/src/relay/test` => `0/0/0`

## 2026-02-14T04:55:31Z
- Implemented canonical subtype fix for optional-like super fields:
  - `types/subtype/subtype.go`: optional-flag compatibility now checks whether super field type admits nil (`unwrap.IsOptionalLike`) before rejecting optional sub fields.
- Added regression tests:
  - `types/subtype/subtype_test.go`: `TestRecordOptionalSubFieldAllowedWhenSuperTypeAdmitsNil`.
  - `compiler/check/tests/regression/optional_like_record_param_flow_test.go` (docker containers_list pattern).
- Validation:
  - `go test ./types/subtype -count=1` ✅
  - `go test ./compiler/check/tests/regression -count=1` ✅
  - full harness `./scripts/verify-suite.sh` rerun ✅ with updated counts:
    - `docker-demo`: `43` (improved from `44`)
    - `views`: `27` (unchanged; lock/dependency configuration-driven as established earlier)
    - all other harness targets remain `0/0/0`.
- Confirmed removed docker diagnostic class: `app:containers_list` record-compatibility error no longer appears in lint output.


## 2026-02-14T05:02:39Z
- Added focused diagnosis for `wippy.views:page_registry_test` `page.proxy` issue.
- Signals isolating the issue:
  - `framework/src/views` reports exactly 27 errors, all in `page_registry_test.lua` lines `476-522`.
  - All 27 are `cannot index type nil` on `page.proxy.*` immediately after `test.not_nil(page.proxy)`.
  - Other `test.not_nil(...)` sites in the same file (for `err`, `pages`, `components`, `context`) do not error.
  - With a temporary lock that explicitly declares modules (`wippy/test`, `wippy/terminal`, `wippy/views`) plus replacement, views lint goes `27 -> 0`.
- Source hypothesis (high confidence):
  - This is not a general `test` narrowing failure.
  - The failing class is tied to dependency/manifest availability in the lint context for views; when lock metadata includes modules explicitly, imported `test` summaries are available and `page.proxy` narrowing succeeds.

## 2026-02-14T05:04:21Z
- Clarified lock-mask question for views:
  - Original `framework/src/views/wippy.lock` has `src: .` and no `modules` list.
  - Validation lock also used `src: .` (same source directory), so no source-directory masking.
  - Validation lock added explicit `modules` and replacement (`wippy/views -> .`) only to provide dependency manifests while still linting current local views sources.
- Why other folders pass:
  - `framework/src/llm/src/wippy.lock` and `framework/src/migration/wippy.lock` include `modules` entries for `wippy/test`, so imported test summaries are available there.
- Why only some test calls fail in views:
  - Failure appears on `page.proxy` field-path narrowing after `test.not_nil(page.proxy)`.
  - Other `test.*` calls that do not require refinement propagation can still pass without this summary path.

## 2026-02-14T05:05:08Z
- Confirmed with explicit entry counts (views):
  - default lock (): , , , .
  - validation lock (): , , , .
- This confirms the validation lock does not reduce scope; it increases checked entries while clearing the  failures.

## 2026-02-14T05:05:15Z
- Confirmed with explicit entry counts (views):
  - default lock (`wippy.lock`): `total_entries=9`, `error_count=27`, `warning_count=0`, `hint_count=0`.
  - validation lock (`.tmp_views_with_test.lock`): `total_entries=15`, `error_count=0`, `warning_count=0`, `hint_count=0`.
- This confirms the validation lock does not reduce scope; it increases checked entries while clearing the `page.proxy` failures.

## 2026-02-14T05:06:56Z
- Added direct A/B proof in `framework/src/llm/src` for the same narrowing class:
  - Example pattern present: `claude/mapper_test.lua` has `test.not_nil(result.system)` then `result.system[1].text`.
  - Normal lock run: `total_entries=62`, `error_count=0`.
  - Stripped lock (same src, no modules list): `total_entries=56`, `error_count=3` with mapper/indexing errors.
- Interpretation:
  - Same code pattern succeeds when dependency manifests are loaded.
  - Removing modules metadata reproduces the false positives, matching the views behavior class.

## 2026-02-14T05:12:11Z
- Canonicalized default docker lock via `wippy update` in `/home/wolfy-j/wippy/docker-demo`.
- New default `wippy.lock` now includes explicit modules:
  - `wippy/terminal@0.4.2`
  - `wippy/test@0.4.10`
- Default docker lint after lock update (`/tmp/wippy-local lint --cache-reset --json`):
  - `total_entries=43`
  - `error_count=37`
  - `warning_count=0`
  - `hint_count=0`
- Improvement from previous default docker run: `43 -> 37` errors.

## 2026-02-14T05:14:42Z
- Full harness rerun after canonicalizing default docker lock.
- Results:
  - `/home/wolfy-j/wippy/wippy/tests/app` => `0/0/0`
  - `/home/wolfy-j/wippy/session` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/test` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/actor/test` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/agent/src` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/bootloader` => `0/0/0`
  - `/home/wolfy-j/wippy/docker-demo` => `37/0/0`
  - `/home/wolfy-j/wippy/framework/src/llm/src` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/llm/test` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/migration` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/views` => `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/relay/test` => `0/0/0`
- Net from previous baseline: `views` fixed to zero by canonical default lock; `docker-demo` reduced to 37 remaining errors.

## 2026-02-14 - Current baseline before next fix phase
- Ran `./scripts/verify-suite.sh` from go-lua root.
- Harness result: RED.
- go-lua checker tests: failing `compiler/check/tests/core TestMapJoin_WhileLoopServicePattern` with `cannot assign unknown? to number`.
- Lint target counts from harness:
  - `/home/wolfy-j/wippy/wippy/tests/app`: errors=2 warnings=0 hints=0
  - `/home/wolfy-j/wippy/session`: errors=2 warnings=3 hints=0
  - `/home/wolfy-j/wippy/framework/src/test`: 0/0/0
  - `/home/wolfy-j/wippy/framework/src/actor/test`: 0/0/0
  - `/home/wolfy-j/wippy/framework/src/agent/src`: errors=2 warnings=2 hints=0
  - `/home/wolfy-j/wippy/framework/src/bootloader`: 0/0/0
  - `/home/wolfy-j/wippy/docker-demo`: errors=33 warnings=0 hints=0
  - `/home/wolfy-j/wippy/framework/src/llm/src`: errors=2 warnings=1 hints=0
  - `/home/wolfy-j/wippy/framework/src/llm/test`: errors=2 warnings=1 hints=0
  - `/home/wolfy-j/wippy/framework/src/migration`: 0/0/0
  - `/home/wolfy-j/wippy/framework/src/views`: 0/0/0
  - `/home/wolfy-j/wippy/framework/src/relay/test`: 0/0/0
- Focus requested by user: docker first, no lua patches/hacks, foundational checker fixes + regression tests.

## 2026-02-14 - Canonicalization correction (no duplicate index inference)
- Found regression root: duplicate index-write typing path in `collectInferredTypes` (`TargetIndex` branch) overlapped canonical flow transfer.
- Action taken: removed `TargetIndex` inference branch from `compiler/check/flowbuild/assign/infer.go` to restore single canonical source (flow transfer).
- Validation:
  - `go test ./compiler/check/tests/core -run TestMapJoin_WhileLoopServicePattern -count=1` now passes.
  - `/home/wolfy-j/wippy/wippy/tests/app` lint now `0/0/0` on rebuilt `/tmp/wippy-local`.
- Remaining work: solve root map/index/shape merge gap causing `llm.google` options union artifact and docker false positives.

## 2026-02-14 - Foundational fixes applied (no key-default hacks)
- Implemented transparent annotated-wrapper dispatch at the type visitor layer:
  - `types/typ/visit.go` now unwraps `*typ.Annotated` before visitor dispatch.
  - Added tests in `types/typ/visit_test.go`.
- Removed scattered dynamic-key string-default policy from checker extraction paths:
  - Added canonical helper `canonicalDynamicKeyType` in `compiler/check/flowbuild/assign/keytype.go`.
  - Migrated `emit.go`, `collect.go`, `infer.go` to this helper.
  - Unknown/absent dynamic keys now stay `unknown` (truthy-normalized + widened), never forced to `string`.
- Solver-side key resolution made canonical:
  - In `types/flow/transfer.go`, when extracted key type is absent/unknown, re-resolve via key symbol flow before normalization.

## 2026-02-14 - Dynamic index read source wiring (architectural gap closure)
- Wired existing `MapElementSource` abstraction end-to-end:
  - Extraction: `compiler/check/flowbuild/assign/emit.go` now attaches `MapElementSource` for `local v = t[k]` when `k` is non-static and source path cannot be statically represented.
  - Solving: `types/flow/transfer.go` now derives assignment type via `mapElementTypeAt` from current map flow type.
- This removes the previous parallel/fallback behavior where dynamic-index reads collapsed to static unknown types.

## 2026-02-14 - Regression tests/status after fixes
- Added regression: `compiler/check/tests/regression/computed_index_unresolved_key_test.go`.
- Existing regressions now passing:
  - `IndexedMapPairsKeyInference_DirectIndex`
  - `IndexedMapPairsKeyInference_NestedIndex`
- Full checker tests passing:
  - `go test ./compiler/check/... -count=1` => pass

## 2026-02-14T07:42:11Z
- Canonical fix implemented for dynamic-index table literal field precision under builtin `type(...)` guards.
- Root issue reproduced in go-lua:
  - Pattern: `pending[k] = { respond_to = payload.respond_to }` after `if type(payload.respond_to) ~= "string" then return end`
  - Readback `local op = pending[k2]; send(op.respond_to)` previously inferred `any` for `respond_to`.
- Engine changes:
  - Added `CollectTypeGuards` in `compiler/check/flowbuild/guard/guard.go` to propagate positive builtin type facts from branch edges (including `~=` fallthrough).
  - Extended `NarrowTableFieldsByGuard` to apply both:
    - truthy guard narrowing (existing behavior),
    - builtin type-key narrowing via `narrow.ByTypeKey` (new behavior).
  - Wired new type-guard map into assignment extraction in `compiler/check/flowbuild/assign/emit.go`:
    - table-literal field narrowing during dynamic index writes now uses type guards,
    - attr synthesis path applies type-guard narrowing when available.
- Added regression coverage:
  - `compiler/check/tests/regression/dynamic_index_table_literal_type_guard_test.go`
  - New/updated guard unit tests in `compiler/check/flowbuild/guard/guard_test.go`.
- Verification:
  - `go test ./compiler/check/flowbuild/guard ./compiler/check/tests/regression -count=1` => pass.
  - Full harness `./scripts/verify-suite.sh`:
    - `/home/wolfy-j/wippy/wippy/tests/app`: `0/0/0` (fixed from prior `1/0/0`)
    - all non-docker framework/session targets remain `0/0/0`
    - `/home/wolfy-j/wippy/docker-demo`: `34/0/0` (remaining)
- Current blocker:
  - Docker remaining diagnostics are concentrated in:
    - `db:query` row typing as `any` at return sites,
    - unchecked `string?` from multi-return usage in tests,
    - unknown fields from untyped records/options in API code.

## 2026-02-14T07:42:11Z (docker classification evidence)
- Re-classified remaining docker `34` diagnostics by class:
  - `arg_string_from_optional`: 20
  - `record_param_optional_field`: 6
  - `return_any_vs_table`: 4
  - `arg_string_from_unknown`: 3
  - `assign_unknown_to_string`: 1
- Entry distribution:
  - `app.docker.persist:images_test`: 26
  - `app.docker.persist:images`: 3
  - `app.docker.persist:containers`: 1
  - `app:containers_delete`: 2
  - `app:images_pull`: 1
  - `app.docker:docker_client`: 1
- Minimal checker proof for `return_any_vs_table` class:
  - Unannotated receiver param:
    - `local function get(db): (table?, string?) ... local rows, err = db:query(...) ... return rows[1] end`
    - checker emits `cannot return any, expected table?`.
  - Annotated receiver param:
    - `local function get(db: DB): (table?, string?) ...`
    - checker is clean.
- Conclusion:
  - Remaining docker diagnostics are predominantly user-code typing gaps (missing param annotations / unchecked optional values), not the solved map-guard flow false-positive class.
  - `go test ./types/... -count=1` => pass

## 2026-02-14 - Harness status (fresh run)
- `./scripts/verify-suite.sh` results:
  - `/home/wolfy-j/wippy/wippy/tests/app`: `0/0/0`
  - `/home/wolfy-j/wippy/session`: `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/test`: `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/actor/test`: `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/agent/src`: `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/bootloader`: `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/llm/src`: `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/llm/test`: `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/migration`: `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/views`: `0/0/0`
  - `/home/wolfy-j/wippy/framework/src/relay/test`: `0/0/0`
  - `/home/wolfy-j/wippy/docker-demo`: `38/0/0` (remaining)
- Next focus: docker remaining 38 diagnostics only.

## 2026-02-14 - Recovery + full harness green
- Restored accidental docker-demo edits from canonical userspace copies (no harness script edits):
  - `/home/wolfy-j/wippy/docker-demo/src/docker/persist/images.lua`
  - `/home/wolfy-j/wippy/docker-demo/src/docker/persist/containers.lua`
  - `/home/wolfy-j/wippy/docker-demo/src/docker/persist/images_test.lua`
  - `/home/wolfy-j/wippy/docker-demo/src/docker/client.lua`
- Remaining docker lint issue after restore: 1 error in `docker/client.lua` (`cannot assign unknown to string` at raw_body assignment).
- Applied minimal type-correct fix in docker code:
  - `if opts.raw_body then` now coerces non-string values via `tostring(opts.raw_body)` before assigning to `encoded_body: string`.
- Re-ran full harness `./scripts/verify-suite.sh` (fresh build + cache reset each target):
  - `tests/app`: 0/0/0
  - `session`: 0/0/0
  - `framework/src/test`: 0/0/0
  - `framework/src/actor/test`: 0/0/0
  - `framework/src/agent/src`: 0/0/0
  - `framework/src/bootloader`: 0/0/0
  - `docker-demo`: 0/0/0
  - `framework/src/llm/src`: 0/0/0
  - `framework/src/llm/test`: 0/0/0
  - `framework/src/migration`: 0/0/0
  - `framework/src/views`: 0/0/0
  - `framework/src/relay/test`: 0/0/0
- Exit status: success.

## 2026-02-14 - Wider regression swoop
- Ran full harness: `./scripts/verify-suite.sh`.
- Ran full repo tests: `go test ./... -count=1`.
- Results:
  - go-lua tests: pass (all packages green).
  - Harness targets all green (`errors=0 warnings=0 hints=0`):
    - `/home/wolfy-j/wippy/wippy/tests/app`
    - `/home/wolfy-j/wippy/session`
    - `/home/wolfy-j/wippy/framework/src/test`
    - `/home/wolfy-j/wippy/framework/src/actor/test`
    - `/home/wolfy-j/wippy/framework/src/agent/src`
    - `/home/wolfy-j/wippy/framework/src/bootloader`
    - `/home/wolfy-j/wippy/docker-demo`
    - `/home/wolfy-j/wippy/framework/src/llm/src`
    - `/home/wolfy-j/wippy/framework/src/llm/test`
    - `/home/wolfy-j/wippy/framework/src/migration`
    - `/home/wolfy-j/wippy/framework/src/views`
    - `/home/wolfy-j/wippy/framework/src/relay/test`
- Regression status: no regressions detected in this sweep.

## 2026-02-14 - Callsite canonicalization sweep
- Refactor target: `compiler/check/callsite` duplicated symbol candidate logic.
- Added shared helper: `compiler/check/callsite/candidates.go`.
  - `symbolSet` for deterministic deduped ordering.
  - `selectPreferredSymbol` for shared preferred-first selection.
  - `addAliasExpansion` for shared alias expansion.
- Migrated:
  - `compiler/check/callsite/callee_symbols.go`
  - `compiler/check/callsite/canonical_symbol.go`
- Behavior intent: no policy change; remove parallel local implementations.
- Verification:
  - `go test ./compiler/check/callsite -count=1` => pass
  - `go test ./compiler/check/... -count=1` => pass
  - `go test ./... -count=1` => pass
  - `./scripts/verify-suite.sh` => all harness targets `0/0/0`

## 2026-02-14 - Canonical function-fact views (store cleanup)
- Refactor target: duplicated extraction of summary/narrow/func channels from `FunctionFacts` in `store`.
- Added canonical exported view helpers in `returns`:
  - `returns.SummaryViewFromFacts`
  - `returns.NarrowViewFromFacts`
  - `returns.FuncTypeViewFromFacts`
- Removed duplicate local extractor functions from `store`.
- Updated `SessionStore` snapshot readers to use canonical `returns` helpers.
- Simplified `initInterprocFacts` by not pre-initializing deprecated mirror maps (`ReturnSummaries`, `NarrowReturns`, `FuncTypes`); canonical writers own mirror materialization.
- Updated store tests to validate via canonical `returns` view helpers.
- Verification:
  - `go test ./compiler/check/returns ./compiler/check/store -count=1` => pass
  - `go test ./compiler/check/... -count=1` => pass
  - `go test ./... -count=1` => pass
  - `./scripts/verify-suite.sh` => all harness targets `0/0/0`

## 2026-02-14 - Function channel normalization boundary
- Added canonical boundary helper: `returns.NormalizeFunctionFactChannels(*api.Facts)`.
  - Promotes legacy-only channels into canonical `FunctionFacts` when canonical is absent.
  - Rewrites legacy mirrors from canonical writer path.
  - Preserves canonical precedence when canonical facts already exist.
- Updated canonical readers/writers:
  - `readFunctionFactFromFacts` now uses canonical entry first; falls back to legacy-only data when canonical missing.
- Updated merge/widen boundaries:
  - `returns.MergeFunctionFactIntoFacts` normalizes before merge.
  - `returns.MergeFunctionFactsIntoFacts` normalizes once per batch and uses internal normalized merge helper.
  - `returns.WidenFacts` normalizes `prev`/`next` and now collects symbols from canonical maps only.
- Updated store boundary:
  - `SessionStore.UpdateInterprocFactsNext` now normalizes function channels after update callback.
- Added regression coverage:
  - `TestNormalizeFunctionFactChannels_PromotesLegacyIntoCanonical` in `compiler/check/returns/kernel_test.go`.
- Validation:
  - `go test ./compiler/check/returns ./compiler/check/store -count=1` => pass
  - `go test ./compiler/check/... -count=1` => pass
  - `go test ./... -count=1` => pass
  - `./scripts/verify-suite.sh` => all harness targets `0/0/0`

## 2026-02-14 - Additional de-scatter cleanup
- `callsite/effect_resolution.go`:
  - extracted `resolveSynthedCalleeType(...)` helper.
  - removed duplicated method/receiver fallback logic from both `ResolveCalleeEffect` and `ResolveCalleeType`.
- `returns/kernel.go`:
  - introduced internal `mergeFunctionFactIntoNormalizedFacts(...)` to avoid per-symbol re-normalization in batch merges.
  - `MergeFunctionFactsIntoFacts` now normalizes once per batch.
- Verification:
  - `go test ./compiler/check/callsite -count=1` => pass
  - `go test ./compiler/check/returns ./compiler/check/store -count=1` => pass
  - `go test ./compiler/check/... -count=1` => pass
  - `go test ./... -count=1` => pass
  - `./scripts/verify-suite.sh` => all harness targets `0/0/0`

## 2026-02-14 - Resolve path de-dup
- `compiler/check/flowbuild/resolve/resolve.go`:
  - added `dedupeAppendSymbols` helper.
  - added `calleeResolverCandidates` helper.
  - `CalleeType` now uses canonical candidate builder instead of inlined dedupe logic.
- Behavior intent: no policy change; remove local parallel candidate assembly implementation.
- Verification:
  - `go test ./compiler/check/flowbuild/resolve -count=1` => pass
  - `go test ./compiler/check/... -count=1` => pass
  - `go test ./... -count=1` => pass
  - `./scripts/verify-suite.sh` => all harness targets `0/0/0`

## 2026-02-14 - function_facts maintainability pass
- `compiler/check/returns/function_facts.go`:
  - added `functionFactSymbols` helper for canonical symbol iteration.
  - extracted mirror map upsert/delete helpers:
    - `setOrDeleteReturnSummary`
    - `setOrDeleteNarrowSummary`
    - `setOrDeleteFuncType`
  - reused helpers across `writeFunctionFactToFacts`, summary/narrow/func view extraction, and canonical map building.
- Behavior intent: no policy change; remove repeated map mutation and repeated symbol iteration code.
- Verification:
  - `go test ./compiler/check/returns -count=1` => pass
  - `go test ./compiler/check/... -count=1` => pass
  - `go test ./... -count=1` => pass
  - `./scripts/verify-suite.sh` => all harness targets `0/0/0`

## 2026-02-14T15:30:42Z
- Refactor cleanup pass: removed split ownership in returns symbol-collection helpers.
- Moved function-fact symbol collection helpers to canonical module:
  - `collectFunctionFactChannelSymbols`
  - `collectCanonicalFunctionFactSymbols`
  - `markFunctionFactSymbols`
  in `compiler/check/returns/function_facts.go`.
- Updated callers:
  - `compiler/check/returns/kernel.go` now uses `collectFunctionFactChannelSymbols` and no longer defines local collector helpers.
  - `compiler/check/returns/widen.go` now uses `collectCanonicalFunctionFactSymbols`.
- Goal: eliminate remaining parallel helper path and keep function-fact channel normalization + symbol collection in one canonical layer.

Validation run set:
- `go test ./compiler/check/returns ./compiler/check/store -count=1` ✅
- `go test ./compiler/check/... -count=1` ✅
- `go test ./... -count=1` ✅
- `./scripts/verify-suite.sh` ✅

Harness counts from this run:
- `/home/wolfy-j/wippy/wippy/tests/app`: `0/0/0`
- `/home/wolfy-j/wippy/session`: `0/0/0`
- `/home/wolfy-j/wippy/framework/src/test`: `0/0/0`
- `/home/wolfy-j/wippy/framework/src/actor/test`: `0/0/0`
- `/home/wolfy-j/wippy/framework/src/agent/src`: `0/0/0`
- `/home/wolfy-j/wippy/framework/src/bootloader`: `0/0/0`
- `/home/wolfy-j/wippy/docker-demo`: `0/0/0`
- `/home/wolfy-j/wippy/framework/src/llm/src`: `0/0/0`
- `/home/wolfy-j/wippy/framework/src/llm/test`: `0/0/0`
- `/home/wolfy-j/wippy/framework/src/migration`: `0/0/0`
- `/home/wolfy-j/wippy/framework/src/views`: `0/0/0`
- `/home/wolfy-j/wippy/framework/src/relay/test`: `0/0/0`

## 2026-02-14T15:33:59Z
- Minor canonical cleanup in returns symbol collection:
  - `markFunctionFactSymbols` now iterates map keys directly and relies on a single terminal sort in collectors.
  - Removes repeated per-map sorting while preserving deterministic output from collector-level sort.
- Validation:
  - go test ./compiler/check/returns ./compiler/check/store -count=1 => pass
  - ./scripts/verify-suite.sh => all harness targets 0/0/0.

## 2026-02-14T15:37:02Z
- Post-format verification phase:
  - Ran gofmt on updated returns files (`compiler/check/returns/function_facts.go`, `compiler/check/returns/kernel.go`, `compiler/check/returns/widen.go`).
  - Re-ran full harness `./scripts/verify-suite.sh`.
- Result: all harness targets remain green (`0/0/0` each).

## 2026-02-14T15:41:54Z
- Canonical test cleanup in store package:
  - Updated `compiler/check/store/store_test.go` to seed interproc facts via canonical `FunctionFacts` in widen/fixpoint tests instead of legacy direct `ReturnSummaries` map literals.
  - This removes another legacy channel usage point from checker tests.
- Canonical callsite resolution cleanup:
  - Updated `compiler/check/callsite/effect_resolution.go`.
  - Added `resolveCalleeTypeBySymbolCandidates(...)` and reused it in both `ResolveCalleeEffect` and `ResolveCalleeType`.
  - Removed duplicated by-symbol loop logic while preserving existing resolution order.
- Formatted edited files with gofmt.

Validation:
- `go test ./compiler/check/callsite ./compiler/check/store -count=1` => pass
- `go test ./compiler/check/... -count=1` => pass
- `./scripts/verify-suite.sh` => all harness targets `0/0/0`.

## 2026-02-14T15:42:42Z
- Added full-suite confirmation after this cleanup phase:
  - `go test ./... -count=1` => pass.
  - Harness still green from the same phase (`./scripts/verify-suite.sh` all targets `0/0/0`).

## 2026-02-14T15:47:26Z
- Callsite candidate policy canonicalization pass:
  - Added `ResolverCalleeSymbolCandidates(...)` to `compiler/check/callsite/callee_symbols.go`.
  - Moved resolver candidate assembly policy (callee-path symbol + alias-expanded callsite candidates) out of `flowbuild/resolve` into callsite canonical layer.
  - Updated `compiler/check/flowbuild/resolve/resolve.go` to consume `callsite.ResolverCalleeSymbolCandidates(...)`.
  - Added regression coverage in `compiler/check/callsite/callee_symbols_test.go`:
    - `TestResolverCalleeSymbolCandidates_PrefersCalleePathSymbol`.
- Canonical scope impact:
  - Removes another local duplicate candidate-selection path from resolve and keeps call-candidate policy centralized in callsite package.

Validation:
- `go test ./compiler/check/callsite ./compiler/check/flowbuild/resolve -count=1` => pass
- `go test ./compiler/check/... -count=1` => pass
- `./scripts/verify-suite.sh` => all harness targets `0/0/0`
- `go test ./... -count=1` => pass

## 2026-02-14T15:51:13Z
- Extended callsite canonicalization to effect resolution:
  - `compiler/check/callsite/effect_resolution.go` now uses `ResolverCalleeSymbolCandidates(...)` for symbol-based fallback in both:
    - `ResolveCalleeEffect`
    - `ResolveCalleeType`
  - This keeps callee-path-symbol precedence consistent across resolver and effect paths.
- Added coverage:
  - `compiler/check/callsite/effect_resolution_test.go`
  - new test `TestResolveCalleeType_UsesCalleePathSymbolCandidate`.

Validation:
- `go test ./compiler/check/callsite ./compiler/check/flowbuild/resolve -count=1` => pass
- `go test ./compiler/check/... -count=1` => pass
- `./scripts/verify-suite.sh` => all harness targets `0/0/0`
- `go test ./... -count=1` => pass

## 2026-02-14T15:41:33Z
- Canonical test cleanup in store package:
  - Updated `compiler/check/store/store_test.go` to seed interproc facts via canonical `FunctionFacts` in widen/fixpoint tests instead of legacy direct `ReturnSummaries` map literals.
  - This removes another legacy channel usage point from checker tests.
- Canonical callsite resolution cleanup:
  - Updated `compiler/check/callsite/effect_resolution.go`.
  - Added `resolveCalleeTypeBySymbolCandidates(...)` and reused it in both `ResolveCalleeEffect` and `ResolveCalleeType`.
  - Removed duplicated by-symbol loop logic while preserving existing resolution order.
- Formatted edited files with gofmt.

Validation:
- `go test ./compiler/check/callsite ./compiler/check/store -count=1` => pass
- `go test ./compiler/check/... -count=1` => pass
- `go test ./... -count=1` => pass
- `./scripts/verify-suite.sh` => all harness targets `0/0/0`

## 2026-02-14T15:59:59Z
- Sweep result: found remaining parallel call-candidate usage in:
  - `compiler/check/flowbuild/keyscoll/keyscoll.go`
  - `compiler/check/flowbuild/assign/spec.go`
  - `compiler/check/flowbuild/assign/infer.go`
  - `compiler/check/flowbuild/assign/emit.go`
- Attempted canonical migration of those paths to `ResolverCalleeSymbolCandidates(...)`.
- Regression observed immediately in harness (broad non-zero errors across many wippy targets).
- Root cause:
  - `ResolverCalleeSymbolCandidates` intentionally prepends `CalleePath.Symbol` (often receiver identity for method calls).
  - The affected assign/keyscoll paths require callable-callee symbol candidates, not receiver-identity fallback symbols.
- Canonical correction:
  - Reverted those four paths back to `CalleeSymbolCandidatesWithAliases(...)`.
  - Kept resolver/effect paths on `ResolverCalleeSymbolCandidates(...)`.
  - Added API boundary note in `compiler/check/callsite/callee_symbols.go` documenting intended usage and the strict-callable alternative.
- Verification after correction:
  - `go test ./compiler/check/flowbuild/assign ./compiler/check/flowbuild/keyscoll ./compiler/check/callsite ./compiler/check/flowbuild/resolve -count=1` => pass
  - `go test ./compiler/check/... -count=1` => pass
  - `./scripts/verify-suite.sh` => all harness targets `0/0/0`
  - `go test ./... -count=1` => pass

## 2026-02-14T16:05:27Z
- Wider swoop verification requested:
  - Re-ran full harness: `./scripts/verify-suite.sh`.
  - Re-ran full repo tests: `go test ./... -count=1`.
- Harness result: all targets `errors=0 warnings=0 hints=0`:
  - `/home/wolfy-j/wippy/wippy/tests/app`
  - `/home/wolfy-j/wippy/session`
  - `/home/wolfy-j/wippy/framework/src/test`
  - `/home/wolfy-j/wippy/framework/src/actor/test`
  - `/home/wolfy-j/wippy/framework/src/agent/src`
  - `/home/wolfy-j/wippy/framework/src/bootloader`
  - `/home/wolfy-j/wippy/docker-demo`
  - `/home/wolfy-j/wippy/framework/src/llm/src`
  - `/home/wolfy-j/wippy/framework/src/llm/test`
  - `/home/wolfy-j/wippy/framework/src/migration`
  - `/home/wolfy-j/wippy/framework/src/views`
  - `/home/wolfy-j/wippy/framework/src/relay/test`
- Journal integrity cleanup:
  - Removed accidental raw command output block and restored the affected section to structured notes.
  - Confirmed no remaining raw `go test`/harness dump lines in `JOURNAL.md`.

## 2026-02-14T16:10:00Z
- Wider cross-tree sweep (not limited to touched files):
  - Scanned production `compiler/check` paths for candidate-resolution divergence and compatibility-mirror spread.
  - Verified remaining call-candidate usage is now semantically partitioned:
    - strict callable lookup paths use `CalleeSymbolCandidatesWithAliases`
    - resolver/effect fallback paths use `ResolverCalleeSymbolCandidates`
  - Confirmed remaining `ReturnSummaries` / `NarrowReturns` / `FuncTypes` references are expected API compatibility surfaces and canonical normalization boundaries, not newly scattered policy paths.
- Re-validation:
  - `./scripts/verify-suite.sh` => all harness targets `0/0/0`
  - `go test ./... -count=1` => pass

## 2026-02-14T16:11:57Z
- Cross-tree semantic naming cleanup (clarity + misuse prevention):
  - Renamed strict-callable alias-expanded API across checker tree:
    - `CalleeSymbolCandidatesWithAliases` -> `CallableCalleeSymbolCandidates`
    - `PreferredCalleeSymbolWithAliases` -> `PreferredCallableCalleeSymbol`
  - Updated all production and test call sites in `compiler/check/*`.
  - Kept `ResolverCalleeSymbolCandidates` as separate API for resolver/effect fallback semantics.
- Rationale:
  - Removes ambiguous naming that previously invited mixing strict-callable and resolver-fallback candidate sets.
  - Makes semantic boundary explicit in both API names and call sites.
- Coverage/maintenance:
  - Updated corresponding test names in `compiler/check/callsite/callee_symbols_test.go` to match canonical API language.

Validation:
- `go test ./compiler/check/callsite ./compiler/check/returns ./compiler/check/flowbuild/resolve ./compiler/check/infer/interproc ./compiler/check/synth/phase/extract ./compiler/check/flowbuild/assign ./compiler/check/flowbuild/keyscoll -count=1` => pass
- `go test ./compiler/check/... -count=1` => pass
- `./scripts/verify-suite.sh` => all harness targets `0/0/0`
- `go test ./... -count=1` => pass

## 2026-02-14T16:18:07Z
- Broader non-local cleanup pass in `compiler/check/api/env.go`:
  - Introduced shared `envCommon` implementation for BaseEnv behavior.
  - Consolidated duplicated shared env logic (Phase/Graph/Types/Consts/Effects/TypeNames/Bindings/ModuleAliases/ModuleAlias/GlobalType/GlobalTypes) behind `envCommon`.
  - Kept explicit nil-safe forwarding methods on `DeclaredEnvImpl` / `NarrowEnvImpl` to preserve existing nil-receiver behavior contract.
  - Updated env constructors to initialize `envCommon`.
- Regression caught and fixed in same phase:
  - Initial consolidation caused nil-safety panic in `TestDeclaredEnv_NilSafety` due promoted embedded methods on nil env receivers.
  - Fixed by adding explicit nil-safe forwarding methods on the concrete env types.
- Validation:
  - `go test ./compiler/check/api ./compiler/check -count=1` => pass
  - `go test ./compiler/check/... -count=1` => pass
  - `./scripts/verify-suite.sh` => all harness targets `0/0/0`
  - `go test ./... -count=1` => pass

## 2026-02-14T16:26:40Z
- Wider defensive sweep + canonical bug fix:
  - Found a nil-safety gap in method-call symbol resolution: `methodSymbolFromBaseWithAliases` assumed non-nil graph and could dereference `graph` on strict-callable paths where alias expansion is intentionally optional.
  - Canonical fix in `compiler/check/callsite/receiver.go`:
    - direct-base field symbol lookup is now first-class and independent of alias graph presence.
    - alias expansion is now optional enrichment only when `graph != nil`.
- Added regression coverage:
  - `compiler/check/callsite/callee_symbols_test.go::TestMethodCalleeSymbolFromCall_ResolvesDirectBaseWithoutGraph`
  - Verifies non-alias method symbol resolution works without graph aliases and stays consistent with graph-enabled resolution.
- Verification after fix:
  - `go test ./compiler/check/callsite -count=1` => pass
  - `go test ./compiler/check/... -count=1` => pass
  - `go test ./... -count=1` => pass
  - `./scripts/verify-suite.sh` => all configured targets `errors=0 warnings=0 hints=0`

## 2026-02-14T16:30:11Z
- Broader quality pass beyond touched files:
  - Ran heavy linter across checker tree: `golangci-lint run ./compiler/check/...`.
  - Removed staticcheck noise source in `compiler/check/api/facts.go` by replacing `Deprecated:` field tags on compatibility mirrors with explicit compatibility wording (keeps intent, removes misleading global deprecation diagnostics while migration is still active).
  - Fixed staticcheck quality finding in `compiler/check/api/env.go` (`QF1008`) by removing redundant embedded-field selector usage in overlay builders.
- Full validation after quality pass:
  - `golangci-lint run ./compiler/check/...` => `0 issues`
  - `go test ./compiler/check/... -count=1` => pass
  - `go test ./... -count=1` => pass
  - `./scripts/verify-suite.sh` => all configured targets `errors=0 warnings=0 hints=0`

## 2026-02-14T16:35:17Z
- Canonical function-facts consistency fix (found during wider sweep):
  - Root issue: canonical helper paths (`canonicalFunctionFacts`, `SummaryViewFromFacts`, `NarrowViewFromFacts`, `FuncTypeViewFromFacts`) enumerated symbols only from `Facts.FunctionFacts`.
  - Consequence: facts stored only in compatibility channels (`ReturnSummaries/NarrowReturns/FuncTypes`) could be silently ignored by canonical views/equality before normalization.
- Fix implemented in `compiler/check/returns/function_facts.go`:
  - `functionFactSymbols(...)` now enumerates across all function-fact channels.
  - View builders now derive from `canonicalFunctionFacts(...)` instead of direct `facts.FunctionFacts[...]` reads.
- Regression coverage added:
  - `compiler/check/returns/equal_test.go::TestFactsEqual_LegacyOnlyChannelsAreComparedCanonically`
  - `compiler/check/returns/kernel_test.go::TestFunctionFactViews_UseLegacyChannelsWhenCanonicalMissing`
- Validation:
  - `go test ./compiler/check/returns -count=1` => pass
  - `go test ./compiler/check/tests/regression -count=1` => pass
  - `golangci-lint run ./compiler/check/...` => `0 issues`
  - `go test ./compiler/check/... -count=1` => pass
  - `./scripts/verify-suite.sh` => all configured targets `errors=0 warnings=0 hints=0`

## 2026-02-14T16:38:56Z
- Documentation canonicalization pass:
  - Updated `compiler/check/FIXPOINT_CHANNEL_MAP.md` to match current architecture.
  - Channel table now declares `FunctionFacts` as canonical and marks `ReturnSummaries` / `NarrowReturns` / `FuncTypes` as compatibility mirrors rewritten from canonical state.
  - Replaced stale risk section (parallel-channel split) with current migration-risk statement (mirror drift prevention).
- Re-validation after doc update:
  - `./scripts/verify-suite.sh` => all configured targets `errors=0 warnings=0 hints=0`.

## 2026-02-14T16:45:24Z
- Parallel-path / redundancy audit pass (maintenance-focused):
  - Audited `compiler/check/returns/*` and `compiler/check/callsite/*` for duplicate policy implementations.
- Redundant paths removed:
  1. `callsite` duplicated expression candidate assembly:
     - Before: duplicated `raw + SymbolFromExpr(primary/fallback)` logic in both
       `canonical_symbol.go` and `callee_symbols.go`.
     - After: both use shared `exprSymbolCandidates(...)` in
       `compiler/check/callsite/candidates.go`.
  2. `returns` wrapper indirections around canonical function-type merge:
     - Removed `MergeFuncFactType(...)` wrapper.
     - Removed `mergeFuncTypes(...)` wrapper.
     - `WidenFuncTypes(...)` and tests now call `MergeFunctionFactType(...)` directly.
- Rationale:
  - Fewer alternate entrypoints to same policy => less drift risk and clearer canonical path.
- Validation:
  - `golangci-lint run ./compiler/check/...` => `0 issues`
  - `go test ./compiler/check/... -count=1` => pass
  - `go test ./... -count=1` => pass
  - `./scripts/verify-suite.sh` => all configured targets `errors=0 warnings=0 hints=0`

## 2026-02-14T16:49:11Z
- Additional redundancy elimination:
  - Removed dead, non-production function-type widening entrypoint `returns.WidenFuncTypes`.
  - Canonical function-type widening path remains `WidenFacts` -> `ReconcileFunctionFact` -> `MergeFunctionFactType`.
- Why this matters:
  - Eliminates an alternate exported merge entrypoint that was not used by production flow, reducing policy-surface area.
- Validation:
  - `golangci-lint run ./compiler/check/...` => `0 issues`
  - `go test ./compiler/check/... -count=1` => pass
  - `go test ./... -count=1` => pass
  - `./scripts/verify-suite.sh` => all configured targets `errors=0 warnings=0 hints=0`
- Remaining intentional parallel surfaces (tracked):
  1. `api.Facts` compatibility mirrors (`ReturnSummaries/NarrowReturns/FuncTypes`) vs canonical `FunctionFacts`.
     - Mitigated by normalization + canonical views.
  2. `DeclaredEnvImpl` vs `NarrowEnvImpl` forwarding methods.
     - Intentional for nil-safe behavior and phase-specific interfaces.

## 2026-02-14T16:54:36Z
- Broader parallel-path audit and cleanup:
  - Found duplicated symbol-name resolution logic in `flowbuild/resolve`:
    - `RootName(...)` and `RootFromSymbol(...)` each reimplemented the same `NameOf(sym)` fallback logic.
  - Canonicalized both through one helper:
    - `rootNameFromSymbolSource(...)` in `compiler/check/flowbuild/resolve/resolve.go`.
  - Added regression coverage:
    - `compiler/check/flowbuild/resolve/resolve_test.go::TestRootFromSymbol_UsesGraphName`.
- Validation:
  - `go test ./compiler/check/flowbuild/resolve -count=1` => pass
  - `golangci-lint run ./compiler/check/...` => `0 issues`
  - `go test ./compiler/check/... -count=1` => pass
  - `./scripts/verify-suite.sh` => all configured targets `errors=0 warnings=0 hints=0`

## 2026-02-14T17:11:17Z
- Phase objective: wider sweep for parallel implementations/redundant policy paths across checker.
- Canonicalization/removal completed in this phase:
  - Removed callsite indirection wrapper in `compiler/check/callsite/canonical_symbol.go` (`canonicalExprSymbolCandidates`); direct canonical candidate source now `exprSymbolCandidates` only.
  - Removed assignment key-normalization wrapper in `compiler/check/flowbuild/assign/emit.go`; all paths now call `canonicalDynamicKeyType` directly.
  - Removed flowbuild cond effect wrapper (`ExtractEffectFromType`) and switched tests to canonical `check/effects.EffectFromType`.
  - Removed returns->erreffect facade file `compiler/check/returns/error_return_infer.go`; `postflow` now imports `compiler/check/erreffect` directly.
  - Reduced duplicated channel-projection loops in `compiler/check/returns/function_facts.go` by introducing one canonical helper `projectCanonicalFunctionFactChannel` used by summary/narrow/functype views.
- Validation after these removals/refactors:
  - `go test ./... -count=1` => pass
  - `golangci-lint run ./compiler/check/...` => 0 issues
  - `./scripts/verify-suite.sh` => pass; all harness targets report `errors=0 warnings=0 hints=0`.
- Wider redundancy inventory (remaining):
  - Canonical fact vs compatibility mirrors still intentionally co-exist (`FunctionFacts` plus derived `ReturnSummaries`/`NarrowReturns`/`FuncTypes`) for migration compatibility.
  - Callsite symbol APIs intentionally expose three semantics (`CalleeSymbolCandidates`, `CallableCalleeSymbolCandidates`, `ResolverCalleeSymbolCandidates`); not duplicate policy, but distinct lookup contracts.
  - Some thin API adapters remain (hooks/synth facade methods) as public integration surface; no policy divergence detected.

## 2026-02-14T17:14:15Z
- Per request: performed wider sweep for parallel implementations/redundant paths that increase maintenance load.
- Method: wrapper scan + duplicate function body clustering + targeted checker package inspection.
- High-impact parallel/redundant surfaces found:
  - `compiler/check/api/env.go`:
    - 12 duplicated method implementations across `DeclaredEnvImpl` and `NarrowEnvImpl` (`Phase/Graph/Types/Consts/Effects/TypeNames/Bindings/ModuleAliases/ModuleAlias/GlobalType/GlobalTypes/WithGlobalOverlay`).
    - Anchors: declared side around lines `225..414`; narrow side around lines `233..414` with mirrored bodies.
    - Impact: dual edit points for base-env behavior; easy to drift.
  - `compiler/check/phase/scope.go` + `compiler/check/scope/typedefs.go`:
    - duplicated CFG type-param conversion helpers:
      - `toAstTypeParams` (`phase/scope.go:515`)
      - `ToTypeParamExprs` (`scope/typedefs.go:103`)
    - Impact: same conversion policy maintained in two packages.
  - `compiler/check/infer/interproc/postflow.go` + `compiler/check/returns/widen.go`:
    - parallel map-merge policies for captured field/container facts:
      - `mergeCapturedFieldAssigns` (`postflow.go:546`)
      - `WidenCapturedFieldAssigns` (`widen.go:417`)
      - `mergeCapturedContainerMutations` (`postflow.go:584`) + callback merge
      - `WidenCapturedContainerMutations` (`widen.go:469`) + callback merge
      - canonical merge utility exists in `returns/container_mutation_merge.go:14,53`.
    - Impact: same domain merged through separate handwritten loops + callbacks.
  - `compiler/check/callsite/canonical_symbol.go` + `compiler/check/callsite/callee_symbols.go`:
    - overlapping candidate/alias expansion stacks:
      - `CanonicalSymbolFromExprWithAliases` (`canonical_symbol.go:31`)
      - `CalleeSymbolCandidates` (`callee_symbols.go:17`)
      - `CallableCalleeSymbolCandidates` (`callee_symbols.go:79`)
      - `ResolverCalleeSymbolCandidates` (`callee_symbols.go:125`)
    - Impact: callsite symbol-candidate semantics split across two entry families; higher cognitive load even after recent cleanup.
- Medium-impact redundancy/noise surfaces (mostly adapter-heavy):
  - `compiler/check/synth/engine.go` and `compiler/check/synth/phase/extract/synthesizer.go` contain many pass-through forwarding methods.
  - These appear intentional facade APIs, but represent a large indirection layer to maintain.
- Validation status after sweep (no new functional refactor in this step):
  - checker tests/lint/harness remained previously green from last committed cleanup.

## 2026-02-14T17:30:07Z
- Refactor phase: canonicalize redundant merge/conversion paths (no behavior relaxation).
- Implemented:
  - Added canonical captured-field merge kernel in `compiler/check/returns/captured_field_merge.go` (`MergeCapturedFieldSymbolMaps`).
  - Removed duplicate local merge logic in `compiler/check/infer/interproc/postflow.go`; now uses:
    - `returns.MergeCapturedFieldSymbolMaps(..., returns.JoinInterprocTypes)`
    - `returns.MergeCapturedContainerMutationMaps` directly (deleted local wrappers).
  - Rewired `compiler/check/returns/widen.go` `WidenCapturedFieldAssigns` to reuse `MergeCapturedFieldSymbolMaps` with monotone widening merge callback.
  - Removed duplicated CFG->AST type-param converter in `compiler/check/phase/scope.go`; now uses canonical `scope.ToTypeParamExprs`.
  - Reduced duplicated equality map loops in `compiler/check/returns/equal.go` by introducing shared helpers:
    - `symbolTypeVectorMapEqual`
    - `symbolTypeMapEqual`.
  - Centralized alias-expansion candidate primitive in `compiler/check/callsite/candidates.go` (`expandAliasCandidates`) and reused in:
    - `compiler/check/callsite/canonical_symbol.go`
    - `compiler/check/callsite/callee_symbols.go`.
- Validation:
  - `go test ./... -count=1` => pass.
  - `golangci-lint run ./compiler/check/...` => 0 issues.
  - `./scripts/verify-suite.sh` => pass; all harness targets report `errors=0 warnings=0 hints=0`.

## 2026-02-14T17:35:47Z
- Added README disclaimer section at bottom:
  - AI-generated / AI-assisted implementation notice.
  - Type system stable but currently pre-convergence.
- Checker cleanup pass:
  - Removed three low-value single-use swap wrappers in `compiler/check/store/store.go`.
  - Inlined channel swap policies directly in `swapInterprocChannels` with explicit policy comment (overwrite vs widening).
- Verification:
  - `go test ./compiler/check/... -count=1` => pass.
  - `golangci-lint run ./compiler/check/...` => 0 issues.
  - `./scripts/verify-suite.sh` => pass, all harness targets `errors=0 warnings=0 hints=0`.

## 2026-02-14T17:41:37Z
- Additional slop-reduction pass in checker core:
  - Removed duplicated nil-join boilerplate in:
    - `compiler/check/returns/join.go` (`JoinInterprocTypes` now delegates directly to canonical `typ.JoinPreferNonSoft`).
    - `compiler/check/synth/ops/call.go` (`mergeExpectedArgType` delegates directly to canonical `typ.JoinPreferNonSoft`).
  - Removed one-line widen indirection in `compiler/check/returns/widen.go` (`needsConvergenceWiden`), using `hasHigherOrderGrowthRisk` directly.
- Validation:
  - `go test ./... -count=1` => pass.
  - `golangci-lint run ./compiler/check/...` => 0 issues.
  - `./scripts/verify-suite.sh` => pass; all harness targets `errors=0 warnings=0 hints=0`.

## 2026-02-14T17:47:25Z
- Further checker slop cleanup (no semantic changes):
  - `compiler/check/synth/ops/call.go`:
    - removed trivial local helper `mergeExpectedArgType`; callsites now use canonical `typ.JoinPreferNonSoft` directly.
  - `compiler/check/returns/equal.go`:
    - reduced alias duplication:
      - `ParamHintsEqual` now delegates to `ReturnSummariesEqual`.
      - `CapturedTypesEqual` now delegates to `FuncTypesEqual`.
  - `compiler/check/synth/engine.go`:
    - `AllowReturnTransforms` now delegates to `IsNarrowing()` to keep phase predicate single-sourced.
- Validation:
  - `go test ./... -count=1` => pass.
  - `golangci-lint run ./compiler/check/...` => 0 issues.
  - `./scripts/verify-suite.sh` => pass; all harness targets `errors=0 warnings=0 hints=0`.

## 2026-02-14T17:53:07Z
- Removed sibling-merge wrapper layer to reduce indirection:
  - Deleted `compiler/check/siblings/sibling.go` (wrapper around `returns.MergeFunctionFactType`).
  - Updated call sites to canonical merge:
    - `compiler/check/siblings/siblings.go`
    - `compiler/check/infer/nested/processor.go`
  - Deleted wrapper-only tests file `compiler/check/siblings/sibling_test.go`.
  - Updated retained sibling tests to validate canonical behavior via `returns.MergeFunctionFactType`.
- Validation:
  - `go test ./... -count=1` => pass.
  - `golangci-lint run ./compiler/check/...` => 0 issues.
  - `./scripts/verify-suite.sh` => pass; all harness targets `errors=0 warnings=0 hints=0`.

## 2026-02-14T18:02:45Z
- Wrapper/alias cleanup pass focused on pure pass-throughs (kept interface adapters intact):
  - Removed `phase.MergeParamHintsIntoSig` alias from `compiler/check/phase/types.go`.
  - Rewired callers to canonical `paramhints.MergeIntoSignature` in:
    - `compiler/check/phase/scope.go`
    - `compiler/check/pipeline/runner_stages.go`
    - `compiler/check/phase/types_test.go`
  - Removed `returns.JoinInterprocTypes` alias from `compiler/check/returns/join.go`.
  - Rewired all callsites to canonical `typ.JoinPreferNonSoft` in:
    - `compiler/check/returns/widen.go`
    - `compiler/check/returns/join.go`
    - `compiler/check/returns/callgraph.go`
    - `compiler/check/infer/interproc/postflow.go`
    - `compiler/check/returns/container_mutation_merge_test.go`
  - Removed equality alias wrappers from `compiler/check/returns/equal.go`:
    - deleted `ParamHintsEqual` (use `ReturnSummariesEqual` directly)
    - deleted `CapturedTypesEqual` (use `FuncTypesEqual` directly)
    - updated `FactsEqual` and tests accordingly.
- Notes:
  - During this pass, two mechanical replacement mistakes were corrected immediately (`returns.typ.JoinPreferNonSoft` typo and accidental function-name rewrite), then revalidated.
- Validation:
  - `go test ./compiler/check/... -count=1` => pass.
  - `golangci-lint run ./compiler/check/...` => 0 issues.
  - `./scripts/verify-suite.sh` => pass; all harness targets report `errors=0 warnings=0 hints=0`.
  - `go test ./... -count=1` => pass.
  - `golangci-lint run ./...` => 0 issues.

## 2026-02-14T18:09:59Z
- Additional wrapper cleanup pass (dead/pure pass-through only):
  - Removed unused `InferIterVarsWithFlow` from `compiler/check/synth/phase/extract/synthesizer.go`.
  - Removed test-only API wrapper `CanonicalSymbolFromExpr` from `compiler/check/callsite/canonical_symbol.go`.
  - Removed test-only API wrapper `PreferredCalleeSymbol` from `compiler/check/callsite/callee_symbols.go`.
  - Updated callsite tests to assert canonical primitives directly:
    - `selectPreferredSymbol(exprSymbolCandidates(...), prefer)`
    - `selectPreferredSymbol(CalleeSymbolCandidates(...), prefer)`
- Validation:
  - `go test ./compiler/check/... -count=1` => pass.
  - `golangci-lint run ./compiler/check/...` => 0 issues.
  - `./scripts/verify-suite.sh` => pass; all harness targets report `errors=0 warnings=0 hints=0`.
  - `go test ./... -count=1` => pass.
  - `golangci-lint run ./...` => 0 issues.

## 2026-02-14T18:15:16Z
- Removed another non-canonical convenience wrapper:
  - deleted `RunSolveWithResolver` from `compiler/check/phase/solve.go`.
  - updated tests in `compiler/check/phase/solve_test.go` to use canonical `RunSolve(FlowSolveInput{...})` path directly.
- Rationale:
  - `RunSolveWithResolver` was test-only indirection (no production callers), duplicating solve entry semantics.
  - Keeping one canonical solve entry path reduces API surface and maintenance overhead.
- Validation:
  - `go test ./compiler/check/... -count=1` => pass.
  - `golangci-lint run ./compiler/check/...` => 0 issues.
  - `./scripts/verify-suite.sh` => pass; all harness targets report `errors=0 warnings=0 hints=0`.
  - `go test ./... -count=1` => pass.
  - `golangci-lint run ./...` => 0 issues.

## 2026-02-14T18:19:48Z
- Finalized the requested `more` cleanup pass: verified no additional low-value wrapper/parallel-path removals in the current scope.
- Ran wrapper candidate scan over `compiler/check` for single-return functions; remaining matches are canonical constructors/predicates/typed APIs, not one-line alias shims introduced in this pass.
- Re-ran all validations after sweep:
  - `go test ./compiler/check/... -count=1` ✅
  - `golangci-lint run ./compiler/check/...` ✅
  - `./scripts/verify-suite.sh` ✅ (all targets `errors=0 warnings=0 hints=0)
- Notes:
  - `ltype.go` and `ltype_test.go` remain modified in working tree from prior context and were not part of this cleanup scope.
