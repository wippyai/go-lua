# flashrefactor

`flashrefactor` performs one atomic semantic ownership cut in Go source. It
has one authored input: a strict `cutplan.Intent`. There is no action list,
compatibility decoder, shell executor, or forwarding mode.

The command has six mutually exclusive modes:

- `-intent reviewed.json` prepares a write-free exact Lock. It may write only
  explicitly requested `-lock-out` and `-report-out` artifacts.
- `-lock cut.lock.json` replays that Lock write-free and proves every captured
  input, resolver result, route, gate, provider, diff, and tool identity still
  agrees.
- `-lock cut.lock.json -apply` replays under the transaction lease, installs
  only the Lock footprint, runs the declared bounded tests, and restores the
  preimage on every ordinary failure. Exit status `125` means the mandatory
  bounded-test safety limit fired; it is not a passing or retryable result.
- `-discover directory` or `-discover-callers import/path` is read-only
  pattern discovery. It produces candidates, never authority or a Lock.
- `-survey proposal.json` is read-only semantic proposal collection. Its
  selected resolved sources and desired destination yield closure, footprint,
  residue, and ambiguity rows for review; a proposal is not an Intent, Lock,
  or apply input.
- `-recovery inspect|rollback|complete` is an explicit durable-transaction
  action. `complete` additionally requires the exact `-lock`; it reruns the
  postflight rather than guessing that a mixed crash state is valid.

```text
go run ./cmd/flashrefactor -root . -intent reviewed-cut.json \
  -lock-out .flashrefactor/locks/link-cut.lock.json \
  -report-out .flashrefactor/reports/link-cut.prepare.json

go run ./cmd/flashrefactor -root . -lock .flashrefactor/locks/link-cut.lock.json

go run ./cmd/flashrefactor -root . -lock .flashrefactor/locks/link-cut.lock.json -apply
```

Artifacts are atomically written only to `.flashrefactor/locks` or
`.flashrefactor/reports` within the repository, or to an explicitly named
absolute path outside it. An artifact path is rejected if the Intent declares
it as a read or write authority.

The provider registry is immutable and intentionally empty in the command
binary. Any `generate` edit fails closed until a provider is compiled into an
explicit future registry configuration. No provider can be supplied through
JSON, an environment variable, or a command line.

Every declared law has an ID, one concrete package (never `./...` or another
pattern), and exact top-level `Test...` name. It runs only through
`scripts/bounded_test.sh` as `go test -json` with an exact
quoted selector; one top-level run/pass and package pass are required. The
command accepts no regex, custom command, environment prefix, limit, or extra
flag. Residue is likewise generated from every relocation/retirement source
and resolver source site, never from an authored absence list.
