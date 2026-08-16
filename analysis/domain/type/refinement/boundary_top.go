package refinement

import (
	"github.com/wippyai/go-lua/analysis/domain/type/inspect"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// ContainsBoundaryTop reports whether t contains any value that should stop
// precise boundary projection: direct any/unknown or either nested in a
// structured type.
func ContainsBoundaryTop(t typ.Type) bool {
	return typ.IsAny(t) || typ.IsUnknown(t) || typ.ContainsAny(t) || inspect.ContainsUnknown(t)
}
