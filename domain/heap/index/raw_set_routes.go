package index

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// locatePack and locateSource project only the RHS descriptor for this write
// row. They are downstream of the selected Heap routes, so invalid dynamic
// keys and absent receivers produce an authenticated empty route rather than
// an invented Pack/Value read.
func (rule *RawSetRule) locatePack(context engine.SelectorContext, access Index) bool {
	descriptor, ok := rule.payloadForWrite(access)
	if !ok {
		return false
	}
	selected, ok := engine.SelectorRead(context, rule.heapRead)
	if !ok {
		return false
	}
	count, ok := engine.SelectorSelectionCount(context, selected)
	if !ok {
		return false
	}
	if !validateSelectedRoutes(context, selected, count) {
		return false
	}
	if count == 0 || descriptor.descriptor.kind != rawPayloadTail {
		return true
	}
	root, rootOK := descriptor.descriptor.payload.Root()
	return rootOK && rule.packRoute(context, root, descriptor.tag)
}

func (rule *RawSetRule) locateSource(context engine.SelectorContext, access Index) bool {
	descriptor, ok := rule.payloadForWrite(access)
	if !ok {
		return false
	}
	selected, ok := engine.SelectorRead(context, rule.heapRead)
	if !ok {
		return false
	}
	count, ok := engine.SelectorSelectionCount(context, selected)
	if !ok || !validateSelectedRoutes(context, selected, count) {
		return false
	}
	if count == 0 {
		return true
	}
	tags, tagsOK := rule.topology.catalog.sourceTags(descriptor.tag)
	if !tagsOK {
		return false
	}
	for _, tag := range tags {
		source, sourceOK := sourceAt(rule.sourcesFor(access), tag)
		if !sourceOK || !rule.sourceRoute(context, source.coordinate, tag) {
			return false
		}
	}
	return true
}

func validateSelectedRoutes(context engine.SelectorContext, selection engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]], count int) bool {
	if count < 0 {
		return false
	}
	for ordinal := 0; ordinal < count; ordinal++ {
		_, cells, selected := engine.SelectorSelectionAt(context, selection, ordinal)
		if !selected || cells.Count() != 1 {
			return false
		}
		_, _, available := cells.At(0)
		if !available {
			return false
		}
	}
	return true
}
