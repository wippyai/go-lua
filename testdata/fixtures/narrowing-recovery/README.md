# narrowing-recovery fixture pack

Executable specifications for precision that a bounded **decreasing (narrowing)**
solver pass recovers after widening.

## Background

The abstract-interpretation fixpoint currently has `{Bottom, Join, Widen,
LessEqual}` but no `Narrow`. It stops at the **widened** post-fixpoint: widening
jumps loop-affected numeric quantities to `+inf` (a loop counter or a growing
array length becomes `[1,+inf)`). SOTA engines (Astree, Frama-C/EVA) then run
bounded decreasing (narrowing) iterations that re-descend to the true bound.
Every narrowing step stays `>=` the least fixpoint (sound); a step bound
guarantees termination.

These fixtures encode the precision that pass must recover. They are **pending**
on task `7c17de64` (solver `Narrow` lattice op + bounded decreasing pass, #1378),
which does not exist yet, so each fixture sets both:

- `check.skip = "pending: solver narrowing (decreasing) pass (task 7c17de64) not yet implemented"`
- `run.skip   = "checker-only narrowing-precision fixture"`

Removing those two skips **un-pends** a fixture once `Narrow` is wired. Numeric
axes come first: `lenFloors`, `numFloors`, `IntRange`.

## Oracle per fixture

### loop-counter-bounded

- File: `loop-counter-bounded/main.lua`
- Widened result: counter `i` in `[1,+inf)` inside the loop body.
- Narrowed (recovered) result: `i` in `[1,10]` inside the loop body.
- Consumed at: `local k: number = i` (line 15) — `k` inherits the recovered
  bound `[1,10]`.

### array-fill-then-in-bounds-read

- File: `array-fill-then-in-bounds-read/main.lua`
- Widened result: length floor of `a` is lost, so the guarded read `a[k]` is
  typed optional `number?`.
- Narrowed (recovered) result: exact length relationship recovered; under the
  guard `k >= 1 and k <= #a` the read `a[k]` is proven in-bounds and typed
  non-optional `number`.
- Consumed at: `local v: number = a[k]` (line 30) — requires `a[k] : number`,
  not `number?`.

### accumulator-bounded

- File: `accumulator-bounded/main.lua`
- Widened result: accumulator `count` in `[0,+inf)`.
- Narrowed (recovered) result: `count` in `[0,#xs]` (incremented at most once per
  element of `xs`).
- Consumed at: `local v: number = xs[count]` (line 24) — the guard
  `count >= 1 and count <= #xs` plus the recovered upper bound `count <= #xs`
  prove the read in-bounds and non-optional `number`.

### nested-loop-index

- File: `nested-loop-index/main.lua`
- Widened result: both counters `i` and `j` in `[1,+inf)`.
- Narrowed (recovered) result: `i` in `[1,rows]` and `j` in `[1,cols]`.
- Consumed at: `local cell: number = row[j]` (line 22) — the outer read
  `grid[i]` (bounded by `i in [1,rows]`) and the inner read `row[j]` (bounded by
  `j in [1,cols]`) are both proven in-bounds and non-optional `number`.

## Un-pending

Once the `Narrow` pass lands and re-descends these numeric axes, delete the
`check.skip` and `run.skip` entries from each `manifest.json`. The fixtures then
assert the narrowed types/bounds above.
