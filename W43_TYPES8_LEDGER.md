# W43 types8 ledger

## Closed fixture

- `semantic/channel-send-escape`

## Guarded publication

- A direct `Channel<T>:send` now creates sealing and sharing placement events
  only after the receiver's payload contract has already been published.
- The allocation-time typed-send path now projects only its completed child
  placement facts.  It does not project child bindings, diagnostics, or an
  invented channel value.
- Other `:send` lookalikes retain the opaque-call placement behavior.

## Oracle delta

| Run | Passing | Failing |
| --- | ---: | ---: |
| Base `ad47d30bf` | 551/673 | 122 |
| Final | 552/673 | 121 |

Exact failure-set comparison reports zero added failures and one removed
failure: `semantic/channel-send-escape`.

## Verification

- `go build ./...`
- `go vet ./...`
- `go test ./analysis/check/engine -run '^TestStage1Red' -count=1`
- `go test ./analysis/check/fixpoint/front/fronttest -count=1`
- `GOMEMLIMIT=3GiB go test ./analysis/check/engine -run '^TestFullOracle$' -count=1 -v`

The full oracle remains intentionally red because the baseline contains
unfixed fixtures; the scorecard and exact set diff above are the hard gate.
