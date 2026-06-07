package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// BoundaryPathProjection maps point-local addresses onto caller-visible function
// boundary roots. Producers supply the CFG-derived root map; stored fact readers
// own key decoding before entering this boundary algebra.
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

// PathsFromAddress projects a stable symbol-rooted address into all matching
// boundary-relative paths.
func (p BoundaryPathProjection) PathsFromAddress(addr StableAddress) []BoundaryPath {
	sym, ok := addr.Symbol()
	if !ok {
		return nil
	}
	return p.pathsFromSymbolSegments(sym, addr.Segments())
}

// PathsFromPath projects a symbol-rooted point-local path into all matching
// boundary-relative paths.
func (p BoundaryPathProjection) PathsFromPath(path constraint.Path) []BoundaryPath {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return nil
	}
	return p.PathsFromAddress(addr)
}

func (p BoundaryPathProjection) pathsFromSymbolSegments(sym cfg.SymbolID, segments []constraint.Segment) []BoundaryPath {
	if sym == 0 {
		return nil
	}
	var out []BoundaryPath
	if idx, ok := p.paramBySymbol[sym]; ok && idx >= 0 {
		out = append(out, BoundaryPath{
			Kind:     BoundaryPathParam,
			Index:    idx,
			Segments: cloneSegments(segments),
		})
	}
	for _, root := range p.returnBySymbol[sym] {
		nextSegments := segments
		if len(root.Segments) > 0 {
			trimmed, ok := boundaryPathSuffix(segments, root.Segments)
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
