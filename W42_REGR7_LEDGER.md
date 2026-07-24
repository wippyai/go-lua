# W42 regression-7 ledger

No file below `testdata/fixtures` was changed. No `__legacy` source was read
as implementation authority or modified.

| Fixture | Required consumer fact | Current closed-fact result |
| --- | --- | --- |
| `async-closure-member-not-sync-proof` | async receiver result | absent; only the non-callable member path is closed |
| `concat-operand-narrows-inferred-optional` | `string.match` result nilability | unavailable at the child concat consumer |
| `deadlock-compiler-lua` | host module exports | not published by the fixture's manifest |
| `error-return-second-slot-contract` | paired local-call tuple result | unavailable before the sibling assignment |
| `error-sibling-without-guard-stays-optional` | paired local-call tuple result | unavailable before method dispatch |
| `generic-ctor-concrete-mismatch-rejected` | instantiated local generic result | no closed instantiation/result fact |
| `gradual-or-default-field-untyped-source` | gradual origin through `or` | absent at the argument consumer |
| `gradual-typing-adversarial` | validated structural result | incomplete validation does not publish the required fact |
| `local-function-fact-authority` | captured member callable result | current member capability is unavailable at the return boundary |
| `non-dominating-field-defined-wrapper-return` | branch-joined captured member result | no guarded join publication |
| `non-dominating-field-write-call-assignment` | branch-joined member call result | no guarded join publication |
| `one-sided-predicate-false-edge-no-narrow` | false-edge gradual provenance | not carried to the assignment consumer |
| `signature-variant-correlation` | literal-argument selected return variant | local result-relation fact is not published |

The attempted admission changes were discarded: they evaluated additional
uncalled bodies but did not create any of the required producer facts. Retaining
them would have widened the publication boundary without evidence.
