package signature

import "github.com/wippyai/go-lua/analysis/semantic/intrinsic"

// Intrinsic is a sealed semantic identity for a canonical native operation
// whose result depends on caller values. It is assigned only by the lexical
// binding authority; consumers must never reconstruct it from a callee name.
type Intrinsic = intrinsic.Kind

const (
	IntrinsicNone    = intrinsic.None
	IntrinsicLuaType = intrinsic.LuaType
)
