package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

// StaticMemberRefinementReads are the point-local candidate reads used when a
// guard refines a static member path. Exact static-member facts and current
// base-value projection are separate sources because callers may need to compare
// their narrowed precision instead of taking a simple fallback.
type StaticMemberRefinementReads struct {
	Existing ProductValue
	Base     ProductValue
}

// StaticMemberRefinementReads returns the exact cached member fact and the
// corresponding read projected from base, when each is available.
func (f PointFacts) StaticMemberRefinementReads(path constraint.Path, base product.AbstractValue, hasBase bool) StaticMemberRefinementReads {
	if path.Symbol == 0 || len(path.Segments) == 0 {
		return StaticMemberRefinementReads{}
	}
	reads := StaticMemberRefinementReads{}
	if existing, ok := f.StaticMemberValue(path); ok && !existing.IsZero() {
		reads.Existing = ProductValue{Value: existing, State: StateResolved}
	}
	if hasBase && !base.IsZero() {
		if read, ok := ProductMemberPathValue(base, path.Segments); ok && !read.IsZero() {
			reads.Base = ProductValue{Value: read, State: StateResolved}
		}
	}
	return reads
}

// Preferred returns the exact cached fact when present, otherwise the base
// projection. Use StaticMemberRefinementReads directly when both candidates must
// participate in precision selection.
func (r StaticMemberRefinementReads) Preferred() ProductValue {
	if r.Existing.State == StateResolved {
		return r.Existing
	}
	return r.Base
}
