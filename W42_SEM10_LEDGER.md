# W42 semantic rank-1 ledger

Base: `341a04e49`. Fixture files and `__legacy` were not modified.

| Fixture | Census result | Disposition |
| --- | --- | --- |
| any-trust-validation-composition | validation evidence chain | deferred |
| array-length-floor-index-read | relational index witness | deferred |
| assignment-target-diagnostics-evidence-chain | assignment evidence projection | deferred |
| bracket-key-variant-write-invalidates-guard | variant-write invalidation | deferred |
| call-summary-obligation-diagnostics | formal -> member-call -> caller argument transport | resolved |
| captured-guard-invalidated-by-write | capture write invalidation | deferred |
| channel-send-escape | channel escape placement | deferred |
| channel-summary-witness-composition | channel summary witnesses | deferred |
| concat-nilability-provenance | nilability provenance | deferred |
| cross-module-array-callback-alias | module callback alias | deferred |
| cross-module-callable-field-assignment | module callable-field write | deferred |
| cross-module-declared-inferred-boundaries | declared/inferred module boundary | deferred |
| cross-module-generic-callback-pipeline | generic callback module flow | deferred |
| cross-module-recursive-union | recursive-union module relation | deferred |
| cross-module-type-witness-wrapper | type-witness wrapper publication | deferred |
| deep-placement-callback-send | callback-send placement | deferred |
| dynamic-key-variant-write-invalidates-alias | dynamic-key alias invalidation | deferred |
| dynamic-key-variant-write-invalidates-guard | dynamic-key guard invalidation | deferred |
| element-guard-does-not-prove-array | element/array proof separation | deferred |
| json-type-witness-unmarshal | unmarshaled witness provenance | deferred |
| lint-policy-diagnostics-evidence | lint evidence projection | deferred |
| map-entry-guard-does-not-prove-map | entry/map proof separation | deferred |
| mixed-diagnostic-evidence-chain-adversarial | mixed evidence chain | deferred |
| nested-channel-select-union-stress | select union transport | deferred |
| placement-cross-module-assigned-owned-store | assigned owned-store placement | deferred |
| placement-cross-module-owned-store | module owned-store placement | deferred |
| placement-deep-local-callback-send | local callback-send placement | deferred |
| placement-owned-heap-store | owned heap-store placement | deferred |
| placement-relational-param-summary | relational parameter placement | deferred |
| redundant-nil-condition-diagnostics | nil-condition evidence | deferred |
| shape-polymorphic-return-flow | polymorphic return flow | deferred |
| type-engine-edge-matrix | type-engine edge evidence | deferred |
| typed-channel-coroutine-boundaries | typed coroutine channel boundary | deferred |
| unresolved-reference-diagnostics-evidence-chain | unresolved-reference evidence | deferred |

Resolved path: a no-result static member call is admitted only when the child entry receives the callable-member capability already published on the caller's exact argument. A refuted child argument is materialized at the enclosing caller argument only when the child operand is exactly a declared formal and the caller argument span is present. All other aliases, expressions, dynamic member access, missing spans, and incomplete entries fail closed.
