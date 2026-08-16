package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// ExactScalarKeySegment converts exact string/integer type evidence into its
// canonical static path spelling.
func ExactScalarKeySegment(reg *axis.Registry, cache *Cache, value product.Value) (segment.Segment, bool) {
	if reg == nil {
		return segment.Segment{}, false
	}
	t, ok := cache.TypeOf(reg, value)
	if !ok {
		return segment.Segment{}, false
	}
	lit, ok := unwrap.Alias(t).(*typ.Literal)
	if !ok {
		return segment.Segment{}, false
	}
	switch lit.Base() {
	case kind.String:
		name, ok := lit.Value().(string)
		if !ok {
			return segment.Segment{}, false
		}
		return segment.Segment{Kind: segment.SegmentIndexString, Name: name}, true
	case kind.Integer:
		index, ok := lit.Value().(int64)
		if !ok || int64(int(index)) != index {
			return segment.Segment{}, false
		}
		return segment.Segment{Kind: segment.SegmentIndexInt, Index: int(index)}, true
	default:
		return segment.Segment{}, false
	}
}
