package refinement

import (
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/nodeid"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// NeedsSameExpressionFallback reports whether t contains a leaf that can be
// repaired by a same-expression fallback. This is deliberately broader than
// free type parameters: a summary return may contain unknown/any/deferred leaves
// inside otherwise precise structure, and those holes should be repaired by a
// closed signature observation without replacing the whole value.
func NeedsSameExpressionFallback(t typ.Type) bool {
	scan := newSameExpressionFallbackScan(0)
	return scan.needs(t)
}

// NeedsSameExpressionFallbackWithin is the bounded form of
// NeedsSameExpressionFallback. When maxNodes is positive and the scan exceeds
// it, the returned complete flag is false and the caller should treat this as
// "no precision repair from this optional fallback" rather than as proof that no
// repairable leaf exists.
func NeedsSameExpressionFallbackWithin(t typ.Type, maxNodes int) (needs bool, complete bool) {
	scan := newSameExpressionFallbackScan(maxNodes)
	needs = scan.needs(t)
	return needs, scan.scanner.Complete()
}

type sameExpressionFallbackScan struct {
	scanner *inspect.Scanner
}

func newSameExpressionFallbackScan(maxNodes int) *sameExpressionFallbackScan {
	return &sameExpressionFallbackScan{
		scanner: inspect.NewScanner(inspect.ScanOptions{
			Seen:     inspect.NewPointerSeen(nodeid.Pointer),
			MaxSteps: maxNodes,
		}),
	}
}

func (s *sameExpressionFallbackScan) needs(t typ.Type) bool {
	if !s.scanner.Step() {
		return false
	}
	if t == nil {
		return true
	}
	t = unwrap.Annotated(t)
	if t == nil {
		return true
	}
	if summaryNeedsFallbackLeaf(t) {
		return true
	}
	if !s.scanner.Enter(t) {
		return false
	}
	switch v := t.(type) {
	case *typ.Function:
		// Function parameters are contravariant input positions. A loose summary
		// parameter (`any`, unknown, optional self) should not trigger a fallback
		// call by itself because RefineWithFallback preserves such parameters
		// unless the function has an output/covariant hole to repair.
		for _, ret := range v.Returns {
			if s.needs(ret) {
				return true
			}
		}
		return false
	case *typ.Instantiated:
		for _, arg := range v.TypeArgs {
			if s.needs(arg) {
				return true
			}
		}
		return false
	case *typ.Generic, *typ.Interface:
		return false
	case *typ.Record:
		if (v.MapKey != nil && s.needs(v.MapKey)) ||
			(v.MapValue != nil && s.needs(v.MapValue)) ||
			(v.Metatable != nil && s.needs(v.Metatable)) {
			return true
		}
		for _, field := range v.Fields {
			if s.needs(field.Type) {
				return true
			}
		}
		for _, member := range v.StaticMembers {
			if s.needs(member.Type) {
				return true
			}
		}
		return false
	}

	return s.scanner.WalkChildren(t, func(child typ.Type) bool {
		return s.needs(child)
	})
}

func summaryNeedsFallbackLeaf(t typ.Type) bool {
	if t == nil || typ.AbsentOrUnknown(t) || t.Kind().IsPlaceholder() || t.Kind().IsDeferred() {
		return true
	}
	_, ok := t.(*typ.TypeParam)
	return ok
}
