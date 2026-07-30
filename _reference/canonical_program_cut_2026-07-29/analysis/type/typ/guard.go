package typ

// DefaultRecursionDepth bounds recursive type-graph traversals as a
// termination backstop that is independent of cycle detection.
//
// Cycle-pair/cycle-node memoization (subtype's inProgress/memo maps,
// access/typecall's query-path tracking) terminates a genuine cycle in a
// handful of frames, because a repeated node or pair is detected on sight. It
// does not bound a non-cyclic chain that manufactures a new, distinct node at
// every recursive step -- e.g. a programmatically nested table type 10,000
// levels deep is 10,000 distinct *typ.Record allocations, so no pair ever
// repeats and an unguarded walk recurses to the chain's full depth.
//
// 4096 is chosen with large headroom over any nesting depth real Lua source
// produces (in practice table/alias/generic nesting stays two to three orders
// of magnitude shallower than this, even after wrapper unwrapping fans a
// single logical level out into several recursive steps), while remaining a
// finite, cheap ceiling for adversarial or generated input. This is a
// soundness backstop, not a precision knob: raising it does not change what
// is provable, only how deep a pathological chain must be to fail closed.
const DefaultRecursionDepth = 4096
