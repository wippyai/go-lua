# W42 module-signature ledger

## Measured failures

- `modules/active-session-any-time-sub-soundness`
- `modules/arithmetic-param-rejects-cross-module-nonnumber`
- `modules/google-client-metadata-regression`
- `modules/imported-helper-forwards-arg-to-typed-method`
- `modules/providers-open-retry-captured-options`
- `modules/providers-open-retry-captured-options-realtest`

## Boundary finding

The current project flow publishes only an export type and finite return
templates (`exportrelation.Summary`) for a local module. It does not publish
evaluated callable-body effects or captured write postconditions into
`manifest.FunctionSignatures`. Imported calls are instead modeled as external
calls, so their bodies are intentionally unavailable to consumer evaluation.

This is sufficient for declared return type rehydration, but not for the
required receiver, alias, and captured-options postconditions. A sound fix
needs a producer-side, param-relative signature-effect extraction and a
consumer-side application kernel for those certified effects. It cannot be
implemented by inferring facts from source spelling, hardcoding fixture names,
or evaluating an importer against a producer's private body.

No fixture or `__legacy` file was changed.
