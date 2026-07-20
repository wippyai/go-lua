package transformer

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func guardedDynamicTermPath(body *relationProgramBody, term PathTerm) (pathdom.Path, bool) {
	if term == 0 {
		return pathdom.Path{}, true
	}
	if body == nil || body.relation.arena == nil || int(term) >= len(body.relation.arena.paths) {
		return pathdom.Path{}, false
	}
	node := body.relation.arena.paths[term]
	if node.environment != 0 {
		return pathdom.Path{Symbol: node.environment, Segments: append([]segment.Segment(nil), node.segments...)}, true
	}
	slot, exact := body.rootValueSlot(node.root)
	id, symbolSlot := key.ParseSymbolValue(slot)
	if !exact || !symbolSlot {
		return pathdom.Path{}, false
	}
	path := pathdom.NewPath(id, "")
	path.Segments = append(path.Segments, node.segments...)
	return path, true
}

func guardedDynamicPathAddress(body *relationProgramBody, point cfg.Point, path pathdom.Path) (visibility.DynamicReadAddress, bool) {
	if body == nil || body.pathSemantics == nil || path.IsEmpty() {
		return visibility.DynamicReadAddress{}, false
	}
	return body.pathSemantics.FreezeDynamicReadAddress(point, path)
}
