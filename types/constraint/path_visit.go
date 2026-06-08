package constraint

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

// VisitPaths calls fn for each path referenced by c.
//
// It is the allocation-free counterpart to Constraint.Paths(). Callers must
// treat visited paths as read-only.
func VisitPaths(c Constraint, fn func(Path) bool) bool {
	switch v := c.(type) {
	case Truthy:
		if fn(v.Path) {
			return true
		}
		return visitParentFieldPath(v.Path, fn)
	case Falsy:
		if fn(v.Path) {
			return true
		}
		return visitParentFieldPath(v.Path, fn)
	case IsNil:
		return fn(v.Path)
	case NotNil:
		return fn(v.Path)
	case HasType:
		return fn(v.Path)
	case NotHasType:
		return fn(v.Path)
	case HasField:
		return fn(v.Path)
	case FieldEquals:
		if fn(v.Target) {
			return true
		}
		return visitParentFieldPath(v.Target, fn)
	case FieldNotEquals:
		if fn(v.Target) {
			return true
		}
		return visitParentFieldPath(v.Target, fn)
	case IndexEquals:
		return fn(v.Target)
	case IndexNotEquals:
		return fn(v.Target)
	case EqPath:
		return fn(v.Left) || fn(v.Right)
	case NotEqPath:
		return fn(v.Left) || fn(v.Right)
	case FieldEqualsPath:
		return fn(v.Target) || fn(v.Value)
	case FieldNotEqualsPath:
		return fn(v.Target) || fn(v.Value)
	case IndexEqualsPath:
		return fn(v.Target) || fn(v.Value)
	case IndexNotEqualsPath:
		return fn(v.Target) || fn(v.Value)
	case VariantCaseEquals:
		return fn(v.Target)
	case VariantCaseNotEquals:
		return fn(v.Target)
	case KeyOf:
		return fn(v.Table) || fn(v.Key)
	default:
		return false
	}
}

// FirstPath returns the first path referenced by c, if any.
func FirstPath(c Constraint) (Path, bool) {
	var first Path
	ok := false
	VisitPaths(c, func(p Path) bool {
		first = p
		ok = true
		return true
	})
	return first, ok
}

func visitParentFieldPath(path Path, fn func(Path) bool) bool {
	if len(path.Segments) == 0 {
		return false
	}
	if path.Segments[len(path.Segments)-1].Kind != SegmentField {
		return false
	}
	parent := Path{Root: path.Root, Symbol: path.Symbol}
	if len(path.Segments) > 1 {
		parent.Segments = path.Segments[:len(path.Segments)-1]
	}
	return fn(parent)
}

// SemanticAffectedPaths returns the full set of semantic access paths a
// Constraint reads from. Unlike VisitPaths (which exposes only each
// constraint's root path), this exposes every path the constraint's truth
// value depends on — including synthetic field/index sub-paths.
//
// This is the kill-on-assignment visitor for the propagate dataflow (see
// DOMAIN_DESIGN.md §8.2). The kill rule used by propagate is the precise
// "ancestor-only" prefix check: assigning to path w invalidates a literal L
// iff w is a (non-strict) prefix of some path in SemanticAffectedPaths(L).
// The design rev 2 wrote a symmetric "ancestor or descendant" check, but the
// descendant direction over-kills (writing x.value would invalidate reads of
// x.kind because x is also a SemanticAffectedPath of FieldEquals{x,"kind"}).
// The precise rule is sound by itself: subpath writes already kill via the
// literal's full read paths (e.g., x.kind ∈ paths kills FieldEquals{x,
// "kind"} when w=x.kind), so the descendant arm is unnecessary.
//
// Mapping (per DOMAIN_DESIGN.md §8.2):
//
//	Truthy{p}                    → [p]
//	Falsy{p}                     → [p]
//	IsNil{p}                     → [p]
//	NotNil{p}                    → [p]
//	HasType{p, T}                → [p]
//	NotHasType{p, T}             → [p]
//	HasField{p, f}               → [p, p.f]
//	FieldEquals{p, f, lit}       → [p, p.f]
//	FieldNotEquals{p, f, lit}    → [p, p.f]
//	FieldEqualsPath{p, f, q}     → [p, p.f, q]
//	FieldNotEqualsPath{p, f, q}  → [p, p.f, q]
//	IndexEquals{p, k, lit}       → [p, p[k]] (synthetic index when k literal)
//	IndexNotEquals{p, k, lit}    → [p, p[k]]
//	IndexEqualsPath{p, k, q}     → [p, p[k], q]
//	IndexNotEqualsPath{p, k, q}  → [p, p[k], q]
//	EqPath{l, r}                 → [l, r]
//	NotEqPath{l, r}              → [l, r]
//	KeyOf{table, key}            → [table, key]
//
// The returned slice is read-only.
func SemanticAffectedPaths(c Constraint) []Path {
	var out []Path
	VisitSemanticAffectedPaths(c, func(path Path) bool {
		out = append(out, path)
		return false
	})
	return out
}

// VisitSemanticAffectedPaths calls fn for each semantic access path c reads.
// It is the canonical, allocation-light form of SemanticAffectedPaths for
// consumers that need membership, liveness, or write invalidation instead of a
// materialized path slice.
func VisitSemanticAffectedPaths(c Constraint, fn func(Path) bool) bool {
	if fn == nil {
		return false
	}
	return visitSemanticAffectedPathViews(c, semanticAffectedPathFunc{fn: fn})
}

type semanticAffectedPathVisitor interface {
	Path(Path) bool
	PathWithSegment(Path, Segment) bool
}

type semanticAffectedPathFunc struct {
	fn func(Path) bool
}

func (v semanticAffectedPathFunc) Path(path Path) bool {
	return v.fn(path)
}

func (v semanticAffectedPathFunc) PathWithSegment(path Path, seg Segment) bool {
	return visitPathWithSegment(path, seg, v.fn)
}

func visitSemanticAffectedPathViews(c Constraint, visitor semanticAffectedPathVisitor) bool {
	switch v := c.(type) {
	case Truthy:
		return visitor.Path(v.Path)
	case Falsy:
		return visitor.Path(v.Path)
	case IsNil:
		return visitor.Path(v.Path)
	case NotNil:
		return visitor.Path(v.Path)
	case HasType:
		return visitor.Path(v.Path)
	case NotHasType:
		return visitor.Path(v.Path)
	case HasField:
		return visitor.Path(v.Path) || visitor.PathWithSegment(v.Path, Segment{Kind: SegmentField, Name: v.Field})
	case FieldEquals:
		return visitor.Path(v.Target) || visitor.PathWithSegment(v.Target, Segment{Kind: SegmentField, Name: v.Field})
	case FieldNotEquals:
		return visitor.Path(v.Target) || visitor.PathWithSegment(v.Target, Segment{Kind: SegmentField, Name: v.Field})
	case FieldEqualsPath:
		return visitor.Path(v.Target) ||
			visitor.PathWithSegment(v.Target, Segment{Kind: SegmentField, Name: v.Field}) ||
			visitor.Path(v.Value)
	case FieldNotEqualsPath:
		return visitor.Path(v.Target) ||
			visitor.PathWithSegment(v.Target, Segment{Kind: SegmentField, Name: v.Field}) ||
			visitor.Path(v.Value)
	case IndexEquals:
		if seg, ok := indexSegmentForKey(v.Key); ok {
			return visitor.Path(v.Target) || visitor.PathWithSegment(v.Target, seg)
		}
		return visitor.Path(v.Target)
	case IndexNotEquals:
		if seg, ok := indexSegmentForKey(v.Key); ok {
			return visitor.Path(v.Target) || visitor.PathWithSegment(v.Target, seg)
		}
		return visitor.Path(v.Target)
	case IndexEqualsPath:
		if seg, ok := indexSegmentForKey(v.Key); ok {
			return visitor.Path(v.Target) || visitor.PathWithSegment(v.Target, seg) || visitor.Path(v.Value)
		}
		return visitor.Path(v.Target) || visitor.Path(v.Value)
	case IndexNotEqualsPath:
		if seg, ok := indexSegmentForKey(v.Key); ok {
			return visitor.Path(v.Target) || visitor.PathWithSegment(v.Target, seg) || visitor.Path(v.Value)
		}
		return visitor.Path(v.Target) || visitor.Path(v.Value)
	case EqPath:
		return visitor.Path(v.Left) || visitor.Path(v.Right)
	case NotEqPath:
		return visitor.Path(v.Left) || visitor.Path(v.Right)
	case VariantCaseEquals:
		return visitor.Path(v.Target)
	case VariantCaseNotEquals:
		return visitor.Path(v.Target)
	case KeyOf:
		return visitor.Path(v.Table) || visitor.Path(v.Key)
	default:
		return false
	}
}

// ConstraintAffectedByWrite reports whether a write to writePath invalidates
// any semantic read path of c.
func ConstraintAffectedByWrite(c Constraint, writePath Path) bool {
	if writePath.Symbol == 0 {
		return false
	}
	return ConstraintAffectedBySymbolWrite(c, writePath.Symbol, writePath.Segments)
}

// ConstraintAffectedBySymbolWrite is the AST-free form of
// ConstraintAffectedByWrite for propagation data that already stores the write
// target as a symbol plus segment suffix.
func ConstraintAffectedBySymbolWrite(c Constraint, writeSym cfg.SymbolID, writeSegs []Segment) bool {
	if writeSym == 0 {
		return false
	}
	return visitSemanticAffectedPathViews(c, semanticWriteAffectedVisitor{sym: writeSym, segs: writeSegs})
}

type semanticWriteAffectedVisitor struct {
	sym  cfg.SymbolID
	segs []Segment
}

func (v semanticWriteAffectedVisitor) Path(path Path) bool {
	return PathAffectedBySymbolWrite(path, v.sym, v.segs)
}

func (v semanticWriteAffectedVisitor) PathWithSegment(path Path, seg Segment) bool {
	return pathWithSegmentAffectedBySymbolWrite(path, seg, v.sym, v.segs)
}

// PathAffectedByWrite reports whether writing writePath shadows readPath.
func PathAffectedByWrite(readPath, writePath Path) bool {
	if writePath.Symbol == 0 {
		return false
	}
	return PathAffectedBySymbolWrite(readPath, writePath.Symbol, writePath.Segments)
}

// PathAffectedBySymbolWrite reports whether writing (writeSym, writeSegs)
// shadows readPath. The assignment must be at or above the read path; sibling
// and descendant writes do not invalidate the read.
func PathAffectedBySymbolWrite(readPath Path, writeSym cfg.SymbolID, writeSegs []Segment) bool {
	if readPath.Symbol == 0 || readPath.Symbol != writeSym {
		return false
	}
	if len(writeSegs) > len(readPath.Segments) {
		return false
	}
	for i := range writeSegs {
		if writeSegs[i] != readPath.Segments[i] {
			return false
		}
	}
	return true
}

// pathWithSegmentAffectedBySymbolWrite matches a semantic read path formed by
// appending one synthetic segment to base without materializing that child path.
func pathWithSegmentAffectedBySymbolWrite(base Path, tail Segment, writeSym cfg.SymbolID, writeSegs []Segment) bool {
	if base.Symbol == 0 || base.Symbol != writeSym {
		return false
	}
	readLen := len(base.Segments) + 1
	if len(writeSegs) > readLen {
		return false
	}
	for i, seg := range writeSegs {
		if i < len(base.Segments) {
			if seg != base.Segments[i] {
				return false
			}
			continue
		}
		if seg != tail {
			return false
		}
	}
	return true
}

func visitPathWithSegment(path Path, seg Segment, fn func(Path) bool) bool {
	if path.IsEmpty() {
		return false
	}
	segs := make([]Segment, len(path.Segments)+1)
	copy(segs, path.Segments)
	segs[len(path.Segments)] = seg
	return fn(Path{Root: path.Root, Symbol: path.Symbol, Version: path.Version, Segments: segs})
}

// indexSegmentForKey converts a literal index Key into a Path Segment, when
// the key is a primitive constant we can encode in path segment form (string
// or integer). Returns (zero, false) for non-literal keys or types we cannot
// reduce to a single segment (e.g. table keys, function keys, unresolved
// types). Callers that get false omit the synthetic per-key path and use
// only the container's root path for the kill check.
func indexSegmentForKey(key typ.Type) (Segment, bool) {
	lit, ok := key.(*typ.Literal)
	if !ok || lit == nil {
		return Segment{}, false
	}
	switch lit.Base {
	case kind.String:
		s, ok := lit.Value.(string)
		if !ok {
			return Segment{}, false
		}
		return Segment{Kind: SegmentIndexString, Name: s}, true
	case kind.Integer:
		v, ok := lit.Value.(int64)
		if !ok {
			return Segment{}, false
		}
		return Segment{Kind: SegmentIndexInt, Index: int(v)}, true
	default:
		return Segment{}, false
	}
}
