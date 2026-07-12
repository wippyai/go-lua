package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// BranchProofTerm is a row-owned symbolic proof for a path whose final segment
// is a specialized scalar key. It emits only the existing call-boundary proof
// vocabulary; unsupported kinds and non-present evidence fail closed.
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
	return arena != nil && p.Kind == pathevidence.BranchProofPathPresence &&
		presence.Equal(p.Presence, presence.Present()) &&
		arena.validPath(p.Table, shape) && arena.validValue(p.Key, shape, make(map[ValueTerm]bool))
}

func (p BranchProofTerm) canonical(arena *Arena) string {
	return fmt.Sprintf("%d:%s:%s:%d", p.Kind, arena.canonicalPath(p.Table), arena.canonicalValue(p.Key), p.Presence)
}
