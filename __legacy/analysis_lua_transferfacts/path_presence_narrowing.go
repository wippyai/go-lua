package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/internal/typegraph"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	"github.com/wippyai/go-lua/analysis/domain/type/normalize"
	"github.com/wippyai/go-lua/analysis/domain/type/subst"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

func (l *lowerer) rootPresenceRefinement(target path.Path, cond bool) (factflow.BranchRefinement, bool) {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return factflow.BranchRefinement{}, false
	}
	rootType, ok := l.symbolTypes[target.Symbol]
	if !ok {
		return factflow.BranchRefinement{}, false
	}
	narrowed, ok := narrowTypeByPathPresenceSeen(rootType, target.Segments, &typegraph.Path{})
	if !ok || typ.SameNodeOrAcyclicEqual(rootType, narrowed) {
		return factflow.BranchRefinement{}, false
	}
	root := target.RootOnly()
	value := factflow.NewValueConstraint(l.valueFromType(narrowed))
	if cond {
		return factflow.NewBranchRefinement(root, value, true, factflow.ValueRefinement{}, false), true
	}
	return factflow.NewBranchRefinement(root, factflow.ValueRefinement{}, false, value, true), true
}

func narrowTypeByPathPresenceSeen(t typ.Type, suffix []segment.Segment, active *typegraph.Path) (typ.Type, bool) {
	if t == nil || len(suffix) == 0 {
		return nil, false
	}
	t = unwrap.Annotated(unwrap.NormalizeNil(t))
	if !active.Enter(t, len(suffix)) {
		return t, false
	}
	defer active.Leave(t, len(suffix))
	switch v := t.(type) {
	case *typ.Alias:
		return narrowTypeByPathPresenceSeen(v.UnaliasedTarget(), suffix, active)
	case *typ.Optional:
		return narrowTypeByPathPresenceSeen(v.Inner, suffix, active)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return t, false
		}
		return narrowTypeByPathPresenceSeen(v.Body, suffix, active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return t, false
		}
		return narrowTypeByPathPresenceSeen(expanded, suffix, active)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		changed := false
		for _, member := range v.Members {
			if pathCanBePresent(member, suffix) {
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
		if pathCanBePresent(t, suffix) {
			return t, false
		}
		return typ.Never, true
	}
}

func pathCanBePresent(t typ.Type, suffix []segment.Segment) bool {
	projected, ok := luatypeprojection.ApplySegments(t, suffix)
	if !ok {
		return false
	}
	return typeCanBePresent(projected)
}

func typeCanBePresent(t typ.Type) bool {
	if t == nil {
		return true
	}
	return typeCanBePresentSeen(t, &typegraph.Path{})
}

func typeCanBePresentSeen(t typ.Type, active *typegraph.Path) bool {
	if t == nil {
		return false
	}
	t = unwrap.Annotated(unwrap.NormalizeNil(t))
	if !active.Enter(t, 0) {
		return false
	}
	defer active.Leave(t, 0)
	switch v := t.(type) {
	case *typ.Alias:
		return typeCanBePresentSeen(v.UnaliasedTarget(), active)
	case *typ.Optional:
		return typeCanBePresentSeen(v.Inner, active)
	case *typ.Recursive:
		return v.Body != nil && v.Body != t && typeCanBePresentSeen(v.Body, active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		return expanded != nil && expanded != t && typeCanBePresentSeen(expanded, active)
	case *typ.Union:
		for _, member := range v.Members {
			if typeCanBePresentSeen(member, active) {
				return true
			}
		}
		return false
	default:
		return v.Kind() != kind.Nil && !typ.IsNever(v)
	}
}
