package observation

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/typepath"
	"github.com/wippyai/go-lua/types/typ"
)

// DeclaredPathType projects a path through the admitted declared-type carrier.
// It is a flow-insensitive upper-domain query: callers that need the runtime
// narrowed value should use the solved observation Projector instead.
func DeclaredPathType(declared flow.DeclaredTypes, path constraint.Path) typ.Type {
	if path.Symbol == 0 || len(declared) == 0 {
		return nil
	}
	current := declared[path.Symbol]
	if current == nil {
		return nil
	}
	return typepath.Strict(current, path.Segments)
}
