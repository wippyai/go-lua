# W31 transport2 ledger

## Scope and controls

- Isolated clone at `b3d858c47e1be8ea9635ae1a04ced7bc170ec995`.
- `__legacy` and `testdata/fixtures` were read only.
- No engine change was retained: the investigated transports did not reach a
  closed consumer without either fabricating a value or weakening a fixture.

## Per-fixture trace

| Fixture | First proof death | Status |
|---|---|---|
| `modules/imported-eq-typeof-table-len` | The normal return of imported `test.eq` has no published equality postcondition for `type(values) == "table"`. | blocked: requires a sealed normal-return relation summary. |
| `modules/imported-field-cast-expected-record` | `find_all` crosses only its declared array return; its closed literal element/presence proof is not in the module export. | blocked: requires a sealed container-result/presence summary. |
| `modules/imported-helper-forwards-arg-to-typed-method` | The caller's unknown/any member and the imported receiver authority both die before the untyped helper's child entry. | blocked: requires provenance plus imported receiver authority transport. |
| `modules/imported-not-nil-field-typeof-table-len` | Imported `not_nil` and `eq(type(...), "table")` return normally without published non-nil/type predicates. | blocked: requires sealed normal-return refinements. |
| `modules/imported-record-return-literal` | `store.make` projects only the declared `Snapshot`; the imported `time` receiver/method chain has no callable-result proof at the consumer. | blocked: requires typed receiver-result transport across the module boundary. |
| `modules/imported-stable-local-function-export` | The exported callable signature reaches the consumer, but the local identity body's literal return relation does not. | blocked: requires a sealed parameter-to-return relation summary. |

The five closure/callback fixtures, `typed-callback-chain`, and 10 other
`modules/imported-*` fixtures sampled in the same cluster already pass.

## Result

No sound per-fixture change landed in this continuation. The required pass
count rise was therefore not achieved; this lane must not be merged as an
engine fix.
