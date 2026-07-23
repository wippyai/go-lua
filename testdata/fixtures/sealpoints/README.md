# sealpoints — send-safety seal-point specifications (PENDING)

Executable specifications for the send-safety judgment family (task **ad6c114f**).
At every channel-send / actor-spawn / transfer boundary the checker will emit a
judgment proving the transferred value is either **ISOLATED** (fresh and unaliased,
via escape analysis + freshness evidence) or deeply **IMMUTABLE** (frozen tables);
otherwise **Refuted** / **Unknown**.

A **seal point P** is the program point after which the value is provably no longer
mutated or aliased before it is sent (e.g. a `table.freeze(v)` call, or a
reassignment that severs aliasing). The verdict **Proven-if-sealed-at-P** means the
send is proven safe provided the seal at P dominates the send and nothing
re-aliases / re-mutates the value after P.

## Pending status

This machinery does not exist yet, so every fixture is **skipped** to keep the
suite green:

- `check.skip = "pending: send-safety seal-point judgments (task ad6c114f) not yet emitted"`
- `run.skip  = "checker-only send-safety fixture"`

`runCheckPhase` calls `t.Skip` when `check.skip` is set; the run phase skips when
`run.skip` is set; the oracle suite also skips when `check.skip` is set. The
**un-pend step**, once the judgment family lands, is to remove both skip keys and
replace them with the concrete `check` assertions the oracle should verify (see
"Expected judgment codes" below). The verdicts encoded here are the oracle.

## Fixtures

### 1. `mutate-in-loop-then-send/`
- **Send boundary:** `out:send(acc)` — main.lua line 14.
- **Seal point P:** the post-loop point, main.lua line 13 (immediately after the
  loop `end`; the post-dominator of the last in-loop mutation on lines 11–12).
- **Verdict:** `Proven-if-sealed-at-P`. Inserting `table.freeze(acc)` at P flips
  the send to **Proven**. This fixture encodes the *un-sealed gap case*: with no
  seal the send is **Refuted / Unknown** because `acc` is mutable-and-live at the
  boundary.

### 2. `construct-to-send/`
- **Send boundary:** `out:send(job)` — main.lua line 8.
- **Seal point P:** the construction site, main.lua line 7 (trivially sealed).
- **Verdict:** **Proven ISOLATED**, no seal required — `job` is fresh and
  unaliased with no subsequent mutation. This is the recommended pattern.

### 3. `batch-swap/`
- **Send boundary:** `out:send(batch)` — main.lua line 16.
- **Seal point P:** the swap, main.lua line 15 (`pending = {}`), where reassigning
  `pending` to a fresh table severs aliasing and leaves `batch` as the sole
  reference.
- **Verdict:** **Proven ISOLATED** at the swap point. If code kept mutating
  `batch` after the swap it would require a later seal.

### 4. `conditional-send/`
- **Send boundary:** `process.send(pid, "route.ready", msg)` — main.lua line 15
  (the payload parameter is the send boundary).
- **Seal point P:** the freeze on the send path, main.lua line 14
  (`table.freeze(msg)`), which dominates the send on that path.
- **Verdict:** on the send (urgent) branch, `Proven-if-sealed-at-P`; the other
  branch mutates `msg` (line 17) and does not send, so the join is input-dependent
  and the overall verdict is **conditional** — Proven on the sealed branch, N/A on
  the non-send branch.

## Expected judgment codes

When the machinery lands, the manifests will assert a `send.isolation` judgment
family at each send boundary. Anticipated shape (subject to the final judgment
API):

- `send.isolation.proven` — value is ISOLATED (fresh + unaliased) or deeply
  IMMUTABLE at the boundary; no seal, or seal trivially at the construction site.
- `send.isolation.proven_if_sealed` — proven provided a seal at point P dominates
  the send and nothing re-aliases / re-mutates after P (carries the P site).
- `send.isolation.refuted` — value is mutable-and-live / still aliased at the
  boundary.
- `send.isolation.unknown` — escape / freshness evidence is insufficient to
  decide.
