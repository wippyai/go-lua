# W29 channel port ledger

## Ported

- Reconciled the w27 module type-publication work onto the current w28
  resolver path: current `front.Compilation`, `engine.Result`, and lint
  manifest publication already preserve the same resolver-owned definitions,
  so no competing API or conditional re-lower was restored.
- Admitted an uncalled child only for a direct `Channel<T>:send` whose receiver
  is an exact declared channel formal. The child entry supplies existing
  declaration metadata and a conservative runtime Top value; it does not seed
  a fabricated channel value.
- The apply kernel now checks a direct send against that published channel
  payload type only when the argument is a sealed shape. Unknown/open values
  remain silent.
- Preserved nested literal-member call spans and method display metadata so
  the resulting diagnostic is anchored at the failing member expression with
  ordinary call evidence.

## Fixed

- `semantic/channel-send-payload-contract-diagnostic`

## Remaining channel ledger

- `semantic/channel-summary-witness-composition`: child evaluation still
  stops at an unresolved temporary before it can publish select facts.
- `semantic/nested-channel-select-union-stress` and
  `semantic/typed-channel-coroutine-boundaries`: their select child entries
  still reach unresolved temporaries before diagnostic publication.
- `semantic/channel-send-escape`: the required shared heap/depth/seal
  placement transfer is not yet published.
- `narrowing/partitioning/channel-identity-result-reassigned-no-stale-fact`:
  revocation occurs, but the surviving witness remains concrete `nil` rather
  than the required conservative nilable union.

## Guardrails

- No files under `testdata/fixtures` were changed.
- `__legacy` was not modified or used as an analysis authority.
- The new path is driven by WIR boundary types, apply operands, and existing
  entry publications; it has no fixture-name or source-spelling allowlist.
