package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func (s *Solution) applyPointNumericShapeProjection(p cfg.Point, path constraint.Path, base typ.Type) typ.Type {
	if s == nil || base == nil || typ.IsNever(base) || path.IsEmpty() {
		return base
	}
	lower, _, ok := s.LengthBoundsAt(p, path)
	if !ok || lower <= 0 {
		return base
	}
	refined := narrow.RefineByLengthLowerBound(base, lower)
	if refined == nil {
		return base
	}
	return refined
}

func (s *Solution) pointNumericShapeReachable(p cfg.Point, cond constraint.Condition) bool {
	if s == nil || s.numericAt == nil {
		return true
	}
	state := s.numericAt[p]
	if state == nil {
		return true
	}
	reachable := true
	state.ForEachLenBound(func(key constraint.PathKey, lower, _ int64) bool {
		if lower <= 0 {
			return true
		}
		path, ok := s.pathFromCanonicalKeyAtPoint(p, key)
		if !ok {
			return true
		}
		base := s.canonicalKeyTypeAt(p, string(key))
		if base == nil {
			return true
		}
		if cond.HasConstraints() && !cond.IsTrue() && !cond.IsFalse() {
			base = s.applyPointCondition(p, base, path, cond)
		}
		refined := narrow.RefineByLengthLowerBound(base, lower)
		if refined == nil || typ.IsNever(refined) {
			reachable = false
			return false
		}
		return true
	})
	return reachable
}

func (s *Solution) pathFromCanonicalKeyAtPoint(p cfg.Point, key constraint.PathKey) (constraint.Path, bool) {
	if s == nil || s.inputs == nil || s.inputs.Graph == nil || key == "" {
		return constraint.Path{}, false
	}
	sym, version, suffix, ok := pathkey.ParseKeyUnchecked(key)
	if !ok || sym == 0 || version == 0 {
		return constraint.Path{}, false
	}
	visible := s.inputs.Graph.VisibleVersion(p, sym)
	if visible.ID != version {
		return constraint.Path{}, false
	}
	var segments []constraint.Segment
	if suffix != "" {
		segments = pathkey.ParseSuffix(suffix)
		if segments == nil {
			return constraint.Path{}, false
		}
	}
	return constraint.Path{
		Root:     visible.Root,
		Symbol:   sym,
		Version:  version,
		Segments: segments,
	}, true
}
