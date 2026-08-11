package index

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/engine"
)

func (rule *RawGetRule) locatePack(context engine.SelectorContext, access Access) bool {
	seen := rule.takeScratch()
	defer rule.putScratch(seen)
	return rule.visitSelectedPayloads(context, access, func(tag heapdomain.RawPayloadTag, payload rawPayload) bool {
		if payload.kind != rawPayloadTail || marked(seen.payload, uint64(tag)) {
			return true
		}
		root, ok := payload.payload.Root()
		if !ok {
			return false
		}
		ref, ok := rule.packs.Locate(root)
		if !ok {
			return false
		}
		mark(seen.payload, uint64(tag))
		return engine.SelectRoute(context, ref, tag)
	})
}

func (rule *RawGetRule) locateSource(context engine.SelectorContext, access Access) bool {
	seen := rule.takeScratch()
	defer rule.putScratch(seen)
	return rule.visitSelectedPayloads(context, access, func(_ heapdomain.RawPayloadTag, payload rawPayload) bool {
		for _, tag := range payload.sources {
			if marked(seen.source, uint64(tag)) {
				continue
			}
			source, ok := sourceAt(rule.sources, tag)
			if !ok {
				return false
			}
			ref, ok := rule.values.Locate(source.coordinate)
			if !ok {
				return false
			}
			mark(seen.source, uint64(tag))
			if !engine.SelectRoute(context, ref, tag) {
				return false
			}
		}
		return true
	})
}

func (rule *RawGetRule) visitSelectedPayloads(context engine.SelectorContext, access Access, visit func(heapdomain.RawPayloadTag, rawPayload) bool) bool {
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
			if !rule.heap.Schema().VisitRawAccessRoute(route, fact, selector, func(raw heapdomain.RawAccess) bool {
				if raw.IsTop() {
					return true
				}
				cell, ok := raw.Cell()
				if !ok {
					return false
				}
				for n := 0; n < cell.PresentCount(); n++ {
					present, ok := cell.PresentAt(n)
					if !ok {
						return false
					}
					tag, ok := raw.PayloadTag(present)
					if !ok { // Target boot payloads intentionally have no Program descriptor need.
						if _, _, initial := raw.InitialPayload(present); initial {
							continue
						}
						return false
					}
					payload, ok := payloadAt(rule.payloads, tag)
					if !ok || !visit(tag, payload) {
						return false
					}
				}
				return true
			}) {
				return false
			}
		}
		return true
	})
}

func (rule *RawGetRule) takeScratch() *rawGetScratch {
	value := rule.scratch.Get().(*rawGetScratch)
	clear(value.payload)
	clear(value.source)
	return value
}
func (rule *RawGetRule) putScratch(value *rawGetScratch) {
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

func (rule *RawGetRule) visitContextKeySelectors(context engine.SelectorContext, access Access, visit func(heapdomain.KeySelector) bool) bool {
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
	return rule.selectors != nil && rule.selectors.Visit(fact, visit)
}
