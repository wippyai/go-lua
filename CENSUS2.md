# CENSUS2 — full-oracle failure census

## Method and result

This is a read-only census of `cc91b273e`.  It was made from one unfiltered
run of:

```sh
GOMEMLIMIT=3GiB go test ./analysis/check/engine -run '^TestFullOracle$' -count=1 -v
```

The run reported **444/673 fixtures pass; 229 fail**.  It also reported
**261/630 expected diagnostics hit; 369 miss**.  No source, test, fixture, or
`testdata` file was altered to produce this report.

“Signature” below means the observed expectation/difference shape in the
oracle output; “missing mechanism” is the smallest existing-engine capability
that would have to publish the missing fact or diagnostic.  It is not a
proposal to relax an expectation or add a special case.  Counts are fixtures,
not diagnostic lines: a fixture with both a MISS and a FALSE+ is counted once.

The raw evidence has these overlapping signature totals: 123 fixtures have an
unemitted generic error/count expectation, 95 have `lint.claim.unproven`
FALSE+ output, 46 have a missing structured diagnostic, 33 have a placement
expectation failure, and 11 have `lint.analysis.conservative` FALSE+ output.

## Ranked mechanism groups

| Rank | Failure signature (fixture prefix) | Failed fixtures | Missing mechanism |
| ---: | --- | ---: | --- |
| 1 | Cross-cutting semantic contracts (`semantic`) | 45 | Publish and preserve proven facts through alias invalidation, calls/summaries, channels, recursive values, module boundaries, placement, and diagnostic evidence. |
| 2 | End-to-end application compositions (`realworld`) | 39 | Compose the existing type/flow facts across multi-step modules, callbacks, metatables, registries, and runtime pipelines without losing the proof. |
| 3 | Regression edge contracts (`regression`) | 26 | Revalidate invalidation, dominance, callback-return, gradual-value, and function-field facts after the relevant write/call transition. |
| 4 | Predicate/narrowing partitions (`narrowing`) | 20 | Carry correlated predicate facts to the correct branch and revoke them on reassignment, foreign writes, and function boundaries. |
| 5 | Cast/type-system precision (`types`) | 18 | Materialize cast, numeric/integer, table, recursive-type, depth-limit, and standard-library result facts into checkable contracts. |
| 6 | Imported interface contracts (`modules`) | 17 | Publish imported/exported signatures, aliases, method receivers, nil correlation, and qualified types at the module boundary. |
| 7 | Escape/placement classification (`placement`) | 13 | Publish sound stack/owned/shared/unknown placement and lifetime/escape facts for coroutine, event-loop, and transitive-send paths. |
| 8 | Function/control and call contracts (`functions`) | 13 | Model closure capture, loop/goto legality, union methods, method self/parameters, variadics, and return/call contracts. |
| 9 | Decomposability classification (`decomposable`) | 6 | Derive decomposable versus unknown placement from capture, dynamic index, identity, iterator, function, and metatable effects. |
| 10 | Core predicates/control (`core`) | 6 | Implement `type`/narrowing predicate effects and the remaining control/module-method checks. |
| 11 | Generic instantiation/recursion (`generics`) | 5 | Preserve substitutions and constraints through inference, method returns, and recursive generic shapes. |
| 12 | Frame-local lifetime (`frame-local`) | 5 | Propagate capture, return, store, and suspension escape evidence before assigning frame-local/dies-before-suspension. |
| 13 | CFG/result flow (`flow`) | 5 | Preserve loop/write completion, callback result, return type, and branch-merge nilability facts. |
| 14 | Nil-origin provenance (`origins`) | 4 | Track nil birth through joins and optional-field origins to the eventual unsafe use. |
| 15 | Advisory findings (`advice`) | 4 | Publish loop-invariance, proven-guard, shape-polymorphism, and split-birth evidence to the advice reporter. |
| 16 | Narrowing recovery (`narrowing-recovery`) | 1 | Recover an in-bounds array fact after fill without dropping the preceding proof. |
| 17 | Soundness guard suite (`soundness`) | 1 | Enforce the memory-model guard set and emit all required soundness diagnostics. |
| 18 | Transitive library consumers (`transitive-libs`) | 1 | Keep distinct consumer facts when a shared library is analyzed transitively. |

The group counts sum to **229**.  The ranking intentionally treats
`realworld` and `semantic` as broad integration signatures rather than
claiming that their individual scenarios have one identical root cause; their
complete membership is recorded below.

## Complete membership (each failing fixture exactly once)

| Prefix | Count | Failing fixture names |
| --- | ---: | --- |
| semantic | 45 | actor-supervisor-recursive-app; alias-variant-write-invalidates-guard; annotation-lie-callback-signature-rejected; any-trust-validation-composition; array-length-floor-index-read; assignment-target-diagnostics-evidence-chain; bracket-key-variant-write-invalidates-guard; call-summary-obligation-diagnostics; captured-guard-invalidated-by-write; cast-any-remains-untrusted; channel-selected-value-assignment; channel-send-escape; channel-send-payload-contract-diagnostic; channel-summary-witness-composition; concat-nilability-provenance; concat-operand-diagnostics-evidence; cross-module-array-callback-alias; cross-module-callable-field-assignment; cross-module-declared-inferred-boundaries; cross-module-generic-callback-pipeline; cross-module-recursive-union; cross-module-type-witness-wrapper; deep-placement-callback-send; dynamic-key-variant-write-invalidates-alias; dynamic-key-variant-write-invalidates-guard; element-guard-does-not-prove-array; json-type-witness-unmarshal; lint-policy-diagnostics-evidence; map-entry-guard-does-not-prove-map; mixed-diagnostic-evidence-chain-adversarial; nested-channel-select-union-stress; placement-cross-module-assigned-owned-store; placement-cross-module-owned-store; placement-deep-local-callback-send; placement-owned-heap-store; placement-relational-param-summary; reassigned-function-field-invalidates-callable-type; recursive-method-table-chain; recursive-tree-parent-child; redundant-nil-condition-diagnostics; shape-polymorphic-return-flow; static-bracket-member-read-diagnostic; type-engine-edge-matrix; typed-channel-coroutine-boundaries; unresolved-reference-diagnostics-evidence-chain |
| realworld | 39 | advanced-type-system-stress; agent-workflow-engine; agent-workflow-engine-soundness; context-merge-pipeline; cqrs-order-runtime; cqrs-order-runtime-soundness; discriminated-tool-dispatch; event-bus-saga-runtime; event-bus-saga-runtime-soundness; factory-constructor; fluent-prompt-builder; generic-registry; index-presence-laws; iterator-pipeline; lookup-table-cast; metatable-oop; metatable-shared-self; middleware-session-router; middleware-session-router-soundness; module-with-generics; notification-delivery-runtime; notification-delivery-runtime-soundness; plugin-runtime-pipeline; plugin-runtime-pipeline-soundness; plugin-supervisor-runtime; plugin-supervisor-runtime-soundness; recursive-alias-array-index; recursive-alias-method-chain; recursive-alias-module-return; recursive-alias-nested-field; service-locator; table-builder-pattern; tenant-policy-runtime; tenant-policy-runtime-soundness; trait-registry; transactional-saga-orchestrator; transactional-saga-orchestrator-soundness; typed-callback-chain; typed-enum-constants |
| regression | 26 | array-index-out-of-range-stays-optional; async-closure-member-not-sync-proof; callback-nested-preserves-types; concat-operand-narrows-inferred-optional; constructor-return-variant-inference; deadlock-compiler-lua; deadlock-dataflow-node; error-return-second-slot-contract; error-sibling-without-guard-stays-optional; field-defined-wrapper-return; field-defined-wrapper-return-local-alias; field-defined-wrapper-return-local-alias-reassigned; generic-ctor-concrete-mismatch-rejected; generic-record-with-method; gradual-field-incomplete-guard-rejected; gradual-or-default-field-untyped-source; gradual-typing-adversarial; local-function-fact-authority; non-cast-call-leaves-argument-gradual; non-dominating-field-defined-wrapper-return; non-dominating-field-write-call-assignment; one-sided-predicate-false-edge-no-narrow; pairs-foreign-write-weakens-closed-field; reassigned-field-call-assignment; signature-variant-correlation; type-alias-function-return |
| narrowing | 20 | assert-lib-is-nil-inverse; congruence-access; declared-any-field-write-violates-typed-map; discriminator-wrong-method; else-branch-wrong-type; error-return-without-check; guard-index-in-range; partitioning/channel-identity-result-reassigned-no-stale-fact; partitioning/dependent-reassigned; partitioning/discriminant-reassigned; partitioning/function-boundary-out-of-scope; partitioning/migration-bootloader-runner-status; partitioning/websocket-echo-select-payload; relational-index; type-eq-multivariant-dispatch; union-else-wrong-type; union-no-narrowing-fails; union-wrong-field-after-narrowing; wrapper-conditional-fails; wrapper-without-assert-fails |
| types | 18 | array-covariance-heap-tracked; cast-arithmetic-mixed; cast-arithmetic-multiple; cast-integer-arithmetic; cast-integer-comparison; cast-integer-table-field; cast-integer-typed; cast-number-arithmetic; cast-number-typed; cast-table-constructor; cast-then-index; cast-tostring-concat; cast-type-is-direct; cast-type-is-falsy-fail; cast-type-is-not-fail; over-depth-limit; recursive-mismatch-rejected; string-stdlib-return-types |
| modules | 17 | active-session-any-time-sub-soundness; arithmetic-param-rejects-cross-module-nonnumber; google-client-metadata-regression; host-global-qualified-type; imported-eq-typeof-table-len; imported-field-cast-expected-record; imported-helper-forwards-arg-to-typed-method; imported-inferred-is-nil-sibling-correlation; imported-map-of-record-store; imported-map-of-time-record-store; imported-not-nil-field-typeof-table-len; imported-qualified-type-alias-signature; imported-record-return-literal; imported-self-method-store; imported-stable-local-function-export; providers-open-retry-captured-options; providers-open-retry-captured-options-realtest |
| placement | 13 | bridge-main-event-loop; conditional-cache-store; coroutine-resume-retention; coroutine-yield-escape; enrichment-debounce; enrichment-derive-clean; hub-inbound-parking; list-inbox-clean; select-receive-local; select-receive-shared-store; transitive-cross-module-send-escape; transitive-lib-local-chain; upload-materialized-row-clean |
| functions | 13 | break-in-function-inside-loop; break-outside-loop; closure-nested-3-levels; goto-duplicate-label; goto-undefined; method-call-with-params; method-call-with-self; method-on-union-fails; return-call-wrong-arg; string-method-resolution; table-remove-returns-element; variadic-wrong-type; wrong-return-type |
| decomposable | 6 | captured-closure; dynamic-index; identity-compare; pairs-iteration; passed-function; setmetatable |
| core | 6 | control-for-string-init; module-with-method; narrow-no-check-fails; type-is-basic; type-is-direct-condition; type-is-falsy-excludes |
| generics | 5 | constraint-violation-rejected; generic-multi-infer; method-returns-type-param; mutually-recursive-generic; recursive-generic-box |
| frame-local | 5 | captured; loop-suspension; receive-before-last-use; returned; stored-module-state |
| flow | 5 | break-in-for; callback-result-preserves-type; higher-order-function-types; return-wrong-type; uninitialized-local-nil-survives-branch-merge |
| origins | 4 | nil-born-survives-join-reaches-use; nil-through-three-joins; nil-through-two-joins; optional-field-origin |
| advice | 4 | invariant-loop-read; redundant-guard; shape-polymorphic; split-birth-discriminant |
| narrowing-recovery | 1 | array-fill-then-in-bounds-read |
| soundness | 1 | memory-model-guards |
| transitive-libs | 1 | shared-lib-divergent-consumers |

## Scorecard and set-diff proof

| Check | Result |
| --- | --- |
| Clone base | `cc91b273e` |
| Initial full oracle | `444/673`, `229 fail`, `261/630` expected diagnostics hit, `369 miss` |
| Fixture failure-set extraction | 229 unique `--- FAIL: TestFullOracle/<fixture>` records |
| Baseline comparison | The requested baseline is `444/673`; this clone is exactly that base and the observed result is exactly `444/673` (pass-count delta `0`). |
| Classification accounting | Ranked-group total `229`; complete-membership table total `229`. |
| Build | `go build ./...` passed |
| Vet | `go vet ./...` passed |
| Engine unit suites (excluding the intentionally-red full oracle) | `go test ./analysis/check/engine -run '^(TestCheck|TestPublished|TestRecursive|TestValueAgainst)' -count=1` passed |
| Stage-1 | `go test ./analysis/check/engine -run '^TestStage1Red' -count=1` passed |
| Final full oracle | `444/673`, `229 fail`, `261/630` expected diagnostics hit, `369 miss` (expected non-zero test exit) |
| Exact post-artifact set diff | `pre=229`, `post=229`, `comm -3 pre post = 0`; no added or removed failing fixture |
| Regression/count disposition | **Neutral — analysis-only artifact.**  No behavior, fixture, or test inputs changed, and the exact post-artifact diff is zero; there is therefore no pass-count rise to claim. |

The full oracle is expected to remain red at this base; its exact count, not a
green exit code, is the census acceptance condition.
