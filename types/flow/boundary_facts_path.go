package flow

import (
	"cmp"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/constraint"
)

func validBoundaryPath(p BoundaryPath) bool {
	return (p.Kind == BoundaryPathParam || p.Kind == BoundaryPathReturn) && p.Index >= 0
}

func cloneBoundaryPath(p BoundaryPath) BoundaryPath {
	if len(p.Segments) == 0 {
		p.Segments = nil
		return p
	}
	p.Segments = append([]constraint.Segment(nil), p.Segments...)
	return p
}

func hashBoundaryPath(h uint64, path BoundaryPath) uint64 {
	h = internal.HashCombine(h, uint64(path.Kind))
	h = internal.HashCombine(h, uint64(path.Index+1))
	h = internal.HashCombine(h, internal.FnvString(constraint.FormatSegments(path.Segments)))
	return h
}

func compareBoundaryPath(a, b BoundaryPath) int {
	if c := cmp.Compare(a.Kind, b.Kind); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Index, b.Index); c != 0 {
		return c
	}
	return compareConstraintSegments(a.Segments, b.Segments)
}

func compareBoundaryBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a && b:
		return -1
	default:
		return 1
	}
}
