package relation

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

// NormalizeIntersectionForMeet applies semantic meet policy explicitly requested
// by relation and projection code.
func NormalizeIntersectionForMeet(members ...Type) Type {
	return normalize.IntersectionForMeet(members...)
}
