package effect

import "github.com/wippyai/go-lua/domain/effect/factor"

// Value is effect's Fact type. Every other axis defines its Value/Fact type
// directly at the axis root; effect's is minted in the factor package because
// that is where its Algebra lives. This alias gives effect the same axis-root
// name the other axes already have, without moving the type or its Algebra.
type Value = factor.Value
