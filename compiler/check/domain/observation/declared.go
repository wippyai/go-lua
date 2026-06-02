package observation

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	querycore "github.com/wippyai/go-lua/types/query/core"
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
	for _, segment := range path.Segments {
		var next typ.Type
		switch segment.Kind {
		case constraint.SegmentField, constraint.SegmentIndexString:
			next, _ = querycore.Field(current, segment.Name)
			if next == nil {
				next, _ = querycore.Index(current, typ.LiteralString(segment.Name))
			}
		case constraint.SegmentIndexInt:
			next, _ = querycore.Index(current, typ.LiteralInt(int64(segment.Index)))
		default:
			return nil
		}
		if next == nil {
			return nil
		}
		current = next
	}
	return current
}
