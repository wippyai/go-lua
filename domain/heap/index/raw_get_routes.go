package index

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func (rule *RawGetRule) locatePack(context engine.SelectorContext, access Index) bool {
	seen := rule.takeScratch()
	defer rule.putScratch(seen)
	return rule.visitSelectedPayloads(context, access, func(payload RawPayload) bool {
		tag := payload.Tag()
		if !payload.IsTail() || marked(seen.payload, uint64(tag)) {
			return true
		}
		root, ok := payload.Root()
		if !ok {
			return false
		}
		if rule.runtime.packRoute == nil {
			return false
		}
		mark(seen.payload, uint64(tag))
		return rule.runtime.packRoute(context, root, tag)
	})
}

func (rule *RawGetRule) locateSource(context engine.SelectorContext, access Index) bool {
	seen := rule.takeScratch()
	defer rule.putScratch(seen)
	return rule.visitSelectedPayloads(context, access, func(payload RawPayload) bool {
		return rule.runtime.topology.VisitPayloadSources(payload.Tag(), func(tag RawSourceTag, coordinate valuedomain.Coordinate) bool {
			if marked(seen.source, uint64(tag)) {
				return true
			}
			if rule.runtime.sourceRoute == nil || !rule.runtime.sourceRoute(context, coordinate, tag) {
				return false
			}
			mark(seen.source, uint64(tag))
			return true
		})
	})
}

func (rule *RawGetRule) visitSelectedPayloads(context engine.SelectorContext, access Index, visit func(RawPayload) bool) bool {
	if rule == nil || !rule.owns(access) || visit == nil {
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
	// The staged Heap read is an authenticated route selection.  An empty
	// selection therefore proves that no Heap payload was selected; Pack and
	// Value have no downstream route to project, and must not reopen the key
	// selector merely to discover the same empty frontier.
	if count == 0 {
		return true
	}
	return rule.visitContextKeySelectors(context, access, func(selector heapdomain.KeySelector) bool {
		for index := 0; index < count; index++ {
			route, cells, selectedOK := engine.SelectorSelectionAt(context, selected, index)
			if !selectedOK || cells.Count() != 1 {
				return false
			}
			fact, present, available := cells.At(0)
			if !available {
				return false
			}
			if !present {
				continue
			}
			if !rule.runtime.topology.VisitRoutePayloads(route, fact, selector, visit) {
				return false
			}
		}
		return true
	})
}

func (rule *RawGetRule) takeScratch() *RawGetScratch {
	value := rule.scratch.Get().(*RawGetScratch)
	clear(value.payload)
	clear(value.source)
	return value
}
func (rule *RawGetRule) putScratch(value *RawGetScratch) {
	if value != nil {
		rule.scratch.Put(value)
	}
}
func marked(words []uint64, value uint64) bool {
	if value == 0 {
		return false
	}
	value--
	return int(value>>6) < len(words) && words[value>>6]&(uint64(1)<<(value&63)) != 0
}
func mark(words []uint64, value uint64) {
	if value == 0 {
		return
	}
	value--
	if int(value>>6) < len(words) {
		words[value>>6] |= uint64(1) << (value & 63)
	}
}

func (rule *RawGetRule) visitContextKeySelectors(context engine.SelectorContext, access Index, visit func(heapdomain.KeySelector) bool) bool {
	if visit == nil {
		return false
	}
	if _, dynamic := access.DynamicKey(); !dynamic {
		selector, ok := rule.keySelector(access)
		return ok && visit(selector)
	}
	selection, ok := engine.SelectorRead(context, rule.key)
	if !ok {
		return false
	}
	count, ok := engine.SelectorSelectionCount(context, selection)
	if !ok || count != 1 {
		return false
	}
	tag, cells, ok := engine.SelectorSelectionAt(context, selection, 0)
	if !ok || tag != 1 || cells.Count() != 1 {
		return false
	}
	fact, present, available := cells.At(0)
	if !available {
		return false
	}
	if !present {
		return true
	}
	selectors := rule.runtime.topology.selectors
	return selectors != nil && selectors.Visit(fact, visit)
}
