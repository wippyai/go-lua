package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// PathTypeEvidence carries the two type sources available for a path read:
// current point-state product evidence and declared root evidence projected to
// the same path. The sources stay separate because checker layers may prefer one
// only for specific semantic purposes such as discriminant guards.
type PathTypeEvidence struct {
	Current  TypedValue
	Declared TypedValue
}

// PathTypeEvidence returns the current and declared type evidence for path.
// PointFacts owns both path-value projection and declared-root member traversal,
// so callers do not repeat product path walks when they need both views.
func (f PointFacts) PathTypeEvidence(path constraint.Path, declaredRoot typ.Type) PathTypeEvidence {
	evidence := PathTypeEvidence{}
	if path.Symbol == 0 {
		return evidence
	}
	if current, ok := f.PathType(path); ok && !typ.IsAbsentOrUnknown(current) {
		evidence.Current = TypedValue{Type: current, State: StateResolved}
	}
	declared := declaredPathType(path, declaredRoot)
	if !typ.IsAbsentOrUnknown(declared) {
		evidence.Declared = TypedValue{Type: declared, State: StateResolved}
	}
	return evidence
}

func declaredPathType(path constraint.Path, root typ.Type) typ.Type {
	if typ.IsAbsentOrUnknown(root) {
		return nil
	}
	if len(path.Segments) == 0 {
		return root
	}
	av, ok := ProductMemberPathValue(product.FromType(root), path.Segments)
	if !ok || av.IsZero() {
		return nil
	}
	return av.ProjectValue()
}
