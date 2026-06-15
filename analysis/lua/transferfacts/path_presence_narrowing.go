package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func (l *lowerer) rootPresenceRefinement(target path.Path, cond bool) (factflow.BranchRefinement, bool) {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return factflow.BranchRefinement{}, false
	}
	rootType, ok := l.symbolTypes[target.Symbol]
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	narrowed, ok := narrowTypeByPathPresence(rootType, target.Segments, 0)
	if !ok || typ.SameNodeOrAcyclicEqual(rootType, narrowed) {
		return factflow.BranchRefinement{}, false
	}
	root := target
	root.Segments = nil
	value := factflow.NewValueConstraint(typevalue.FromType(l.registry, narrowed))
	if cond {
		return factflow.NewBranchRefinement(root, value, true, factflow.ValueRefinement{}, false), true
	}
	return factflow.NewBranchRefinement(root, factflow.ValueRefinement{}, false, value, true), true
}

func narrowTypeByPathPresence(t typ.Type, suffix []segment.Segment, depth int) (typ.Type, bool) {
	if t == nil || len(suffix) == 0 || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch v := unwrap.Annotated(unwrap.NormalizeNil(t)).(type) {
	case *typ.Alias:
		return narrowTypeByPathPresence(v.UnaliasedTarget(), suffix, depth+1)
	case *typ.Optional:
		return narrowTypeByPathPresence(v.Inner, suffix, depth+1)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		changed := false
		for _, member := range v.Members {
			if pathCanBePresent(member, suffix, depth+1) {
				out = append(out, member)
				continue
			}
			changed = true
		}
		if !changed {
			return t, false
		}
		if len(out) == 0 {
			return typ.Never, true
		}
		return normalize.UnionForEvidence(out...), true
	default:
		if pathCanBePresent(t, suffix, depth+1) {
			return t, false
		}
		return typ.Never, true
	}
}

func pathCanBePresent(t typ.Type, suffix []segment.Segment, depth int) bool {
	projected, ok := luatypeprojection.ApplySegments(t, suffix)
	if !ok {
		return false
	}
	return typeCanBePresent(projected, depth+1)
}

func typeCanBePresent(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	switch v := unwrap.Annotated(unwrap.NormalizeNil(t)).(type) {
	case *typ.Alias:
		return typeCanBePresent(v.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return typeCanBePresent(v.Inner, depth+1)
	case *typ.Union:
		for _, member := range v.Members {
			if typeCanBePresent(member, depth+1) {
				return true
			}
		}
		return false
	default:
		return v.Kind() != kind.Nil && !typ.IsNever(v)
	}
}
