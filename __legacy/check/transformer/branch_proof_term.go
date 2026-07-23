package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// BranchProofTerm is a row-owned symbolic presence proof. When Key is zero,
// Table is the complete path. A nonzero Key appends one specialized scalar
// segment to Table. This dual form lets direct relation composition rebase
// ordinary placeholder proofs without baking a caller path into Summary.
type BranchProofTerm struct {
	Kind     pathevidence.BranchProofKind
	Table    PathTerm
	Key      ValueTerm
	Presence presence.Value
}

func (p BranchProofTerm) placeholderPath(arena *Arena) (pathdom.Path, bool) {
	if arena == nil || p.Table == 0 || int(p.Table) >= len(arena.paths) {
		return pathdom.Path{}, false
	}
	node := arena.paths[p.Table]
	if node.root.Kind != RootParam {
		return pathdom.Path{}, false
	}
	out := pathdom.NewPlaceholder(int(node.root.Index))
	out.Segments = append(out.Segments, node.segments...)
	return out, true
}

func (p BranchProofTerm) valid(arena *Arena, shape Shape) bool {
	if arena == nil || p.Kind != pathevidence.BranchProofPathPresence ||
		!presence.Equal(p.Presence, presence.Present()) || !arena.validPath(p.Table, shape) {
		return false
	}
	return p.Key == 0 || arena.validValue(p.Key, shape, make(map[ValueTerm]bool))
}

func (p BranchProofTerm) canonical(arena *Arena) string {
	if p.Key == 0 {
		return fmt.Sprintf("%d:%s:static:%d", p.Kind, arena.canonicalPath(p.Table), p.Presence)
	}
	return fmt.Sprintf("%d:%s:%s:%d", p.Kind, arena.canonicalPath(p.Table), arena.canonicalValue(p.Key), p.Presence)
}
