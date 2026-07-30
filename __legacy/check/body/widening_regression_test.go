package body

// TestMatchingBracketLoopWideningConverges is a reduced fixture from
// kickside.workflows:mapping. A decreasing numeric lower bound is carried
// around the character-scanning loop; without dual-order lower-bound widening
// this body re-runs its 39 CFG nodes tens of thousands of times.
