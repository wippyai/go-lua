# threshold-widening fixture pack

Executable specifications for precision that threshold widening should preserve
during loop fixpoint solving.

## Background

Task `7c17de64` covers solver narrowing and threshold widening. The narrowing
side is covered by `testdata/fixtures/narrowing-recovery`; this pack covers the
threshold-widening half.

The current solver widens loop-carried numeric facts directly toward top-like
ranges such as `[1,+inf)`. Threshold widening should harvest per-body numeric
thresholds from:

- numeric literals, such as `64`;
- `#` expressions, such as `#thresholds`;
- table constructor sizes, such as the length of a literal array.

When widening would jump past a known threshold, it should widen to the next
threshold first. This is the standard bounded-threshold strategy used by SOTA
abstract interpreters such as Astree and Frama-C/EVA.

Every fixture is pending until that solver support exists. Each manifest sets:

- `check.skip = "pending: solver threshold widening (task 7c17de64) not yet implemented"`
- `run.skip   = "checker-only threshold-widening fixture"`

## Expected Bounds

### literal-1-64-bound

- Threshold source: numeric literal `64`, plus the 64-element table constructor.
- Expected body bound: `i` is `[1,64]` while `i <= 64`.
- Consumed at: `local v: number = values[i]`.
- Required result: `values[i]` is proven in-bounds and non-optional.

### length-derived-threshold

- Threshold source: `#thresholds` and the table constructor size `8`.
- Expected body bound: `i` is `[1,#thresholds]`, concretely `[1,8]` for the
  fixture table.
- Consumed at: `local v: number = thresholds[i]`.
- Required result: `thresholds[i]` is proven in-bounds and non-optional.

### nested-loops

- Threshold sources: `#grid`, `#first`, and the nested table constructor sizes.
- Expected outer body bound: `i` is `[1,#grid]`, concretely `[1,4]`.
- Expected inner body bound: `j` is `[1,#first]`, concretely `[1,8]`.
- Consumed at: `local row: {number} = grid[i]` and
  `local cell: number = row[j]`.
- Required result: both the row read and the cell read are proven in-bounds and
  non-optional.

## Un-pending

Once threshold widening is wired, remove both skip flags from each manifest. The
fixtures should then pass without changing the Lua source.
