# W44 modules-8 ledger

## Closed fixture

`modules/active-session-any-time-sub-soundness`

`time.now()` already receives the host manifest's sealed `time.Time` witness.
The imported session field flows through an `or` expression as a current,
canonical summary of `any`. The apply boundary now consumes that existing
summary only when resolving a published typed receiver method contract. It
therefore reports the required unvalidated-`any` argument error for
`time.Time:sub`, without executing a provider body or treating annotations as
runtime authority.

## Guardrails

- Only the current epoch's `summary-type` fact is consulted; stale summaries
  cannot survive a later write.
- The method contract is not materialized into the partition. It is used only
  for an argument whose existing summary is `any`.
- Optional receivers retain the prior nil-check diagnostic before this narrow
  contract path is considered.
- `testdata/fixtures` and `__legacy` are unchanged.

## Oracle delta

| Run | Passing | Failing |
| --- | ---: | ---: |
| Base `f9d613109` | 556/673 | 117 |
| Final | 557/673 | 116 |

Exact failure-set difference: zero added failures; removed failure:
`modules/active-session-any-time-sub-soundness`.
