package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// BoundaryPathProjection maps point-local stable paths onto caller-visible
// function boundary roots. Producers supply the CFG-derived root map; flow owns
// stable-key decoding, suffix trimming, and BoundaryPath construction.
type BoundaryPathProjection struct {
	paramBySymbol  map[cfg.SymbolID]int
	returnBySymbol map[cfg.SymbolID][]BoundaryPath
}

// NewBoundaryPathProjection builds a boundary path projector from symbol-root
// maps. The maps are copied so callers can keep using mutable CFG scratch state.
func NewBoundaryPathProjection(paramBySymbol map[cfg.SymbolID]int, returnBySymbol map[cfg.SymbolID][]BoundaryPath) BoundaryPathProjection {
	return BoundaryPathProjection{
		paramBySymbol:  cloneParamBoundaryRoots(paramBySymbol),
		returnBySymbol: cloneReturnBoundaryRoots(returnBySymbol),
	}
}

// PathsFromKey projects a stable path key into all matching boundary-relative
// paths. Non-symbol keys do not cross function boundaries and project to nil.
func (p BoundaryPathProjection) PathsFromKey(key constraint.PathKey) []BoundaryPath {
	path, ok := StablePathFromKey(key)
	if !ok {
		return nil
	}
	return p.PathsFromPath(path)
}

// PathsFromPath projects a symbol-rooted point-local path into all matching
// boundary-relative paths.
func (p BoundaryPathProjection) PathsFromPath(path constraint.Path) []BoundaryPath {
	if path.Symbol == 0 {
		return nil
	}
	var out []BoundaryPath
	if idx, ok := p.paramBySymbol[path.Symbol]; ok && idx >= 0 {
		out = append(out, BoundaryPath{
			Kind:     BoundaryPathParam,
			Index:    idx,
			Segments: cloneSegments(path.Segments),
		})
	}
	for _, root := range p.returnBySymbol[path.Symbol] {
		nextSegments := path.Segments
		if len(root.Segments) > 0 {
			trimmed, ok := boundaryPathSuffix(path.Segments, root.Segments)
			if !ok {
				continue
			}
			nextSegments = trimmed
		}
		root.Segments = cloneSegments(nextSegments)
		out = append(out, root)
	}
	return out
}

// MappedReturnIndices reports return slots represented by this projection.
func (p BoundaryPathProjection) MappedReturnIndices() map[int]bool {
	if len(p.returnBySymbol) == 0 {
		return nil
	}
	out := make(map[int]bool)
	for _, roots := range p.returnBySymbol {
		for _, root := range roots {
			if root.Kind == BoundaryPathReturn && root.Index >= 0 {
				out[root.Index] = true
			}
		}
	}
	return out
}

// BoundaryParamPathFromKey projects a caller-side stable key into a callee
// parameter boundary path when it is inside source.
func BoundaryParamPathFromKey(key constraint.PathKey, source constraint.Path, paramIndex int) (BoundaryPath, bool) {
	path, ok := StablePathFromKey(key)
	if !ok {
		return BoundaryPath{}, false
	}
	return BoundaryParamPathFromPath(path, source, paramIndex)
}

// BoundaryParamPathFromPath projects path into a callee parameter boundary path
// when it is rooted at source.
func BoundaryParamPathFromPath(path, source constraint.Path, paramIndex int) (BoundaryPath, bool) {
	if paramIndex < 0 || source.IsEmpty() {
		return BoundaryPath{}, false
	}
	suffix, ok := boundaryPathSuffixFromSource(path, source)
	if !ok {
		return BoundaryPath{}, false
	}
	return BoundaryPath{
		Kind:     BoundaryPathParam,
		Index:    paramIndex,
		Segments: suffix,
	}, true
}

func cloneParamBoundaryRoots(in map[cfg.SymbolID]int) map[cfg.SymbolID]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]int, len(in))
	for sym, idx := range in {
		if sym != 0 && idx >= 0 {
			out[sym] = idx
		}
	}
	return out
}

func cloneReturnBoundaryRoots(in map[cfg.SymbolID][]BoundaryPath) map[cfg.SymbolID][]BoundaryPath {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID][]BoundaryPath, len(in))
	for sym, roots := range in {
		if sym == 0 || len(roots) == 0 {
			continue
		}
		for _, root := range roots {
			if root.Kind != BoundaryPathReturn || root.Index < 0 {
				continue
			}
			out[sym] = append(out[sym], cloneBoundaryPath(root))
		}
	}
	return out
}

func boundaryPathSuffixFromSource(path, source constraint.Path) ([]constraint.Segment, bool) {
	if !boundaryPathSameRoot(path, source) {
		return nil, false
	}
	return boundaryPathSuffix(path.Segments, source.Segments)
}

func boundaryPathSameRoot(path, source constraint.Path) bool {
	if path.Symbol != 0 || source.Symbol != 0 {
		return path.Symbol != 0 && path.Symbol == source.Symbol
	}
	return path.Root != "" && path.Root == source.Root
}

func boundaryPathSuffix(segments, prefix []constraint.Segment) ([]constraint.Segment, bool) {
	if len(prefix) > len(segments) {
		return nil, false
	}
	for i := range prefix {
		if segments[i] != prefix[i] {
			return nil, false
		}
	}
	return cloneSegments(segments[len(prefix):]), true
}

func cloneSegments(segments []constraint.Segment) []constraint.Segment {
	return append([]constraint.Segment(nil), segments...)
}
