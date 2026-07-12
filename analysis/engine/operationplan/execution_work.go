package operationplan

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/workplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// PointWork projects a rich operation row onto the type-independent phase
// ownership consumed by CFG schedulers. Signature and extension payloads stay
// owned by Plan and never cross this boundary.
func (p *Plan) PointWork(point cfg.Point) (workplan.PointWork, error) {
	if p == nil || uint64(point) >= uint64(len(p.rows)) {
		return 0, fmt.Errorf("operationplan: point %d leaves plan", point)
	}
	var work workplan.PointWork
	var last Barrier
	cursor := p.Cursor(point)
	for {
		cell, ok := cursor.Next()
		if !ok {
			break
		}
		meta, exists := Describe(cell.Kind())
		if !exists || meta.Barrier == 0 || meta.Barrier < last {
			return 0, fmt.Errorf("operationplan: non-canonical operation row at %d", point)
		}
		switch meta.Phase {
		case Node:
			work |= workplan.Node
		case Edge:
			work |= workplan.Edge
		default:
			return 0, fmt.Errorf("operationplan: unclassified operation row at %d", point)
		}
		if meta.Stages.Has(E5CallEffects) {
			work |= workplan.Edge
		}
		last = meta.Barrier
	}
	extensions := p.ExtensionCursor(point)
	for extension, ok := extensions.Next(); ok; extension, ok = extensions.Next() {
		meta := extension.Metadata()
		if meta.Class == 0 || meta.Barrier == 0 || meta.Barrier < last {
			return 0, fmt.Errorf("operationplan: non-canonical extension row at %d", point)
		}
		switch meta.Phase {
		case Node:
			work |= workplan.Node
		case Edge:
			work |= workplan.Edge
		default:
			return 0, fmt.Errorf("operationplan: unclassified extension row at %d", point)
		}
		if meta.Stages.Has(E5CallEffects) {
			work |= workplan.Edge
		}
		last = meta.Barrier
	}
	return work, nil
}
