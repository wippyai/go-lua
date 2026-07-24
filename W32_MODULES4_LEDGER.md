# W32 modules4 ledger

## Fixed

None. No candidate reached a closed consumer with existing publication
authority, so no engine change was retained.

## Blocked / deferred

| Fixture | First proof death |
| --- | --- |
| `modules/active-session-any-time-sub-soundness` | The fixture host publishes `time` as an Any boundary; neither `time.now` nor `Time:sub` has a selected callable contract at the consumer. |
| `modules/arithmetic-param-rejects-cross-module-nonnumber` | The non-number argument lies in the body of an uncalled local `run`; no invoked child entry carries the imported record to the arithmetic parameter. |
| `modules/google-client-metadata-regression` | `client.request` is exported as a module callable, but its optional-body call contract has no producer body-summary publication for the imported consumer. |
| `modules/imported-eq-typeof-table-len` | Imported predicate normal returns have no sealed equality/type postcondition. |
| `modules/imported-field-cast-expected-record` | The declared imported array return has no closed literal-element/presence witness. |
| `modules/imported-helper-forwards-arg-to-typed-method` | The unknown caller value and imported receiver authority both end before the helper child entry. |
| `modules/imported-not-nil-field-typeof-table-len` | Imported `not_nil` and `type` predicate normal returns lack refinement publications. |
| `modules/imported-record-return-literal` | The imported record projection lacks a callable receiver-result transport for the `time` method chain. |
| `modules/imported-stable-local-function-export` | The stable exported callable signature carries no sealed parameter-to-return relation. |
| `modules/providers-open-retry-captured-options` | A consumer-side write to an imported module table does not publish an identity/effect summary into the provider closure's captured module cell. |
| `modules/providers-open-retry-captured-options-realtest` | The same imported module-identity and captured retry-option relation is absent through the registry path. |

## Controls

- `testdata/fixtures` was not changed.
- `__legacy` was read only and was not used as implementation authority.
- A trial uncalled-capture diagnostic admission was rejected and reverted: its
  capture facts were not published at the consumer seam, so it could not
  establish the required direct-call evidence without widening authority.
