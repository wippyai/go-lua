package index

import (
	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	"github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// rawSetView is the authenticated finite observation consumed by Fold.
// Pack/Value facts remain owner-issued selections; the view carries no new
// coordinates or mutation authority.
type rawSetView struct {
	key         rawSelected[valuedomain.Value]
	keyCount    int
	heapCount   int
	packCount   int
	pack        func(heapdomain.RawPayloadTag) rawSelected[pack.Value]
	sourceCount int
	source      func(RawSourceTag) rawSelected[valuedomain.Value]
}

func (rule *RawSetRule) fold(frame engine.Frame[heapdomain.Value, Index]) engine.RuleResult[heapdomain.Value] {
	operand, ok := engine.Operand(frame)
	if !ok || !rule.owns(operand) {
		return engine.RuleResult[heapdomain.Value]{}
	}
	receiverCells, receiverOK := engine.ReadValue(frame, rule.receiver)
	keys, keyOK := engine.ReadValue(frame, rule.key)
	heaps, heapOK := engine.ReadValue(frame, rule.heapRead)
	packs, packOK := engine.ReadValue(frame, rule.packRead)
	sources, sourceOK := engine.ReadValue(frame, rule.source)
	if !receiverOK || !keyOK || !heapOK || !packOK || !sourceOK || receiverCells.Count() != 1 {
		return engine.RuleResult[heapdomain.Value]{}
	}
	_, receiverPresent, receiverAvailable := receiverCells.At(0)
	if !receiverAvailable {
		return engine.RuleResult[heapdomain.Value]{}
	}
	descriptor, descriptorOK := rule.payloadForWrite(operand)
	if !descriptorOK {
		return engine.RuleResult[heapdomain.Value]{}
	}
	scratch := rule.takeSetScratch()
	defer rule.putSetScratch(scratch)
	view, viewOK := transferRawSetView(frame, operand, keys, heaps, packs, sources, scratch)
	if !viewOK || !rawSetSelectionShape(operand, descriptor.descriptor, view) {
		return engine.RuleResult[heapdomain.Value]{}
	}
	// An empty Heap route is the explicit no-candidate disposition. This
	// covers absent receivers, selected-but-absent dynamic keys, and
	// definitely invalid dynamic nil/NaN keys; Pack/Value selectors must
	// be empty downstream of that route.
	if view.heapCount == 0 {
		if _, dynamic := operand.DynamicKey(); dynamic {
			if receiverPresent && view.keyCount != 1 || !receiverPresent && view.keyCount != 0 {
				return engine.RuleResult[heapdomain.Value]{}
			}
			// A live receiver's no-route branch is still authenticated by
			// one exact dynamic-key selection. Its cell may be absent only
			// because every downstream selection is empty; routed Heap
			// mutation remains stricter below.
			if receiverPresent && (!view.key.valid || !view.key.found) {
				return engine.RuleResult[heapdomain.Value]{}
			}
		}
		return engine.NoSelection(frame, heaps)
	}
	if !receiverPresent {
		return engine.RuleResult[heapdomain.Value]{}
	}
	return engine.Routed(frame, heaps, func(tag heapdomain.RawRouteTag, cells engine.OrderedCells[heapdomain.Value]) (heapdomain.Value, bool) {
		if cells.Count() != 1 {
			return heapdomain.Value{}, false
		}
		fact, present, available := cells.At(0)
		if !available {
			return heapdomain.Value{}, false
		}
		if !present {
			return rule.heapSchema().Bottom(), true
		}
		return rule.mutateRoute(operand, tag, fact, view)
	})
}

func transferRawSetView(
	frame engine.Frame[heapdomain.Value, Index], operand Index,
	keys engine.Selection[uint64, engine.OrderedCells[valuedomain.Value]],
	heaps engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]],
	packs engine.Selection[heapdomain.RawPayloadTag, engine.OrderedCells[pack.Value]],
	sources engine.Selection[RawSourceTag, engine.OrderedCells[valuedomain.Value]],
	scratch *rawSetScratch,
) (rawSetView, bool) {
	if scratch == nil {
		return rawSetView{}, false
	}
	view := rawSetView{}
	var ok bool
	view.keyCount, ok = engine.SelectionCount(frame, keys)
	if !ok {
		return rawSetView{}, false
	}
	view.heapCount, ok = engine.SelectionCount(frame, heaps)
	if !ok {
		return rawSetView{}, false
	}
	view.packCount, ok = engine.SelectionCount(frame, packs)
	if !ok {
		return rawSetView{}, false
	}
	view.sourceCount, ok = engine.SelectionCount(frame, sources)
	if !ok {
		return rawSetView{}, false
	}
	if !buildTransferIndex(frame, packs, view.packCount, &scratch.pack) ||
		!buildTransferIndex(frame, sources, view.sourceCount, &scratch.source) {
		return rawSetView{}, false
	}
	if _, dynamic := operand.DynamicKey(); dynamic {
		if view.keyCount > 1 {
			return rawSetView{}, false
		}
		if view.keyCount == 1 {
			view.key = transferSelectionValue(frame, keys, nil, uint64(1))
			if !view.key.valid || !view.key.found {
				return rawSetView{}, false
			}
		}
	} else if view.keyCount != 0 {
		return rawSetView{}, false
	}
	view.pack = func(tag heapdomain.RawPayloadTag) rawSelected[pack.Value] {
		return transferSelectionValue(frame, packs, &scratch.pack, tag)
	}
	view.source = func(tag RawSourceTag) rawSelected[valuedomain.Value] {
		return transferSelectionValue(frame, sources, &scratch.source, tag)
	}
	return view, true
}

func rawSetSelectionShape(access Index, descriptor rawPayload, view rawSetView) bool {
	if view.keyCount < 0 || view.heapCount < 0 || view.packCount < 0 || view.sourceCount < 0 {
		return false
	}
	if descriptor.kind != rawPayloadFixed && descriptor.kind != rawPayloadTail && descriptor.kind != rawPayloadNil {
		return false
	}
	if _, dynamic := access.DynamicKey(); dynamic {
		if view.keyCount > 1 || view.heapCount > 0 && view.keyCount != 1 {
			return false
		}
		if view.keyCount == 1 && (!view.key.valid || !view.key.found) {
			return false
		}
		// A routed write must be downstream of the one present dynamic-key
		// observation. A selected-but-absent key lawfully settles only the
		// explicit all-empty no-route branch; it cannot carry a Heap route.
		if view.heapCount > 0 && (!view.key.valid || !view.key.found || !view.key.present) {
			return false
		}
	} else if view.keyCount != 0 {
		return false
	}
	if view.heapCount == 0 {
		return view.packCount == 0 && view.sourceCount == 0
	}
	wantPack := 0
	switch descriptor.kind {
	case rawPayloadFixed, rawPayloadNil:
		// These payloads have no Pack frontier.
	case rawPayloadTail:
		wantPack = 1
	default:
		return false
	}
	return view.packCount == wantPack && view.sourceCount == int(descriptor.sourceCount)
}

// mutateRoute is the common Heap-owned reducer used by Fold. It consumes one
// selected predecessor route, one exact sealed RHS
// descriptor, and existing Pack/Value observations, then joins only the
// branches returned by RawStore/RawDelete. Frozen/error outcomes widen to
// Heap.Top and are never converted into ordinary writes.
func (rule *RawSetRule) mutateRoute(access Index, route heapdomain.RawRouteTag, fact heapdomain.Value, view rawSetView) (heapdomain.Value, bool) {
	if !rule.owns(access) || !fact.Valid() {
		return heapdomain.Value{}, false
	}
	descriptor, descriptorOK := rule.payloadForWrite(access)
	if !descriptorOK {
		return heapdomain.Value{}, false
	}
	if (descriptor.descriptor.kind == rawPayloadTail && view.pack == nil) ||
		(descriptor.descriptor.kind == rawPayloadFixed && view.source == nil) {
		return heapdomain.Value{}, false
	}
	indexAccess, indexOK := access.IndexAccess()
	slot, slotOK := rule.heapSchema().SlotForIndexAccess(indexAccess)
	payload, payloadOK := rule.heapSchema().PayloadForIndexAccess(indexAccess)
	if !indexOK || !slotOK || !payloadOK {
		return heapdomain.Value{}, false
	}
	values := rule.valueSchema()
	if _, dynamic := access.DynamicKey(); dynamic && view.key.found && !values.Equal(view.key.value, view.key.value) {
		return heapdomain.Value{}, false
	}
	schema := rule.heapSchema()
	result := schema.Bottom()
	var frozen, changed, preserved bool
	apply := func(selector heapdomain.KeySelector, keyChild heapdomain.Containment) bool {
		if !selector.Valid() || !keyChild.Valid() {
			return false
		}
		return schema.VisitRawAccessRoute(route, fact, selector, func(raw heapdomain.RawAccess) bool {
			if !raw.Valid() {
				return false
			}
			return rule.applyPayload(raw, descriptor.descriptor, descriptor.tag, view, access, slot, payload, keyChild, &result, &frozen, &changed, &preserved)
		})
	}
	if _, dynamic := access.DynamicKey(); dynamic {
		if !view.key.valid || !view.key.found || !view.key.present || view.key.value.IsBottom() {
			return fact, true
		}
		if view.key.value.IsTop() {
			unknown, unknownOK := schema.ContainmentUnknown()
			if !unknownOK || !rule.topology.selectors.Visit(view.key.value, func(selector heapdomain.KeySelector) bool { return apply(selector, unknown) }) {
				return heapdomain.Value{}, false
			}
		} else {
			selectors := 0
			if !values.VisitAtoms(view.key.value, func(atom valuedomain.Atom) bool {
				alternative, projected := keymatch.Project(schema, values, atom)
				if !projected {
					return true
				}
				selectors++
				return apply(alternative.Selector(), alternative.Containment())
			}) {
				return heapdomain.Value{}, false
			}
			if selectors == 0 {
				preserved = true
			}
		}
	} else {
		selector, keyChild, selectorOK := staticSetSelector(rule, access)
		if !selectorOK || !apply(selector, keyChild) {
			return heapdomain.Value{}, false
		}
	}
	if frozen {
		return schema.Top(), true
	}
	if preserved {
		joined, joinedOK := heapdomain.Join(result, fact)
		if !joinedOK {
			return heapdomain.Value{}, false
		}
		result = joined
	}
	if !changed && !preserved {
		return fact, true
	}
	return result, true
}

func (rule *RawSetRule) applyPayload(
	raw heapdomain.RawAccess, descriptor rawPayload, payloadTag heapdomain.RawPayloadTag,
	view rawSetView, access Index, slot heapdomain.Slot, payload heapdomain.Payload,
	keyChild heapdomain.Containment, result *heapdomain.Value, frozen, changed, preserved *bool,
) bool {
	if result == nil || frozen == nil || changed == nil || preserved == nil {
		return false
	}
	schema := rule.heapSchema()
	switch descriptor.kind {
	case rawPayloadNil:
		return rule.applyDelete(schema, raw, result, frozen, changed)
	case rawPayloadFixed:
		if descriptor.sourceCount != 1 {
			return false
		}
		tags, tagsOK := rule.topology.catalog.sourceTags(payloadTag)
		if !tagsOK || len(tags) != 1 {
			return false
		}
		tag := tags[0]
		return rule.applySourceTag(schema, raw, tag, view, access, slot, payload, keyChild, result, frozen, changed, preserved)
	case rawPayloadTail:
		selected := view.pack(payloadTag)
		if !selected.valid || !selected.found {
			return false
		}
		if !selected.present || selected.value.IsBottom() {
			*preserved = true
			return true
		}
		root, rootOK := descriptor.payload.Root()
		selection, selectionOK := descriptor.payload.Selection()
		values, valuesOK := descriptor.payload.Values()
		if !rootOK || !selectionOK || !valuesOK {
			return false
		}
		observation, observed := rule.packSchema().ObserveScalar(root, selected.value, values, selection)
		if !observed {
			return false
		}
		if observation.IsBottom() {
			*preserved = true
			return true
		}
		if observation.IsTop() {
			return rule.applyTop(schema, raw, access, slot, payload, keyChild, result, frozen, changed)
		}
		for index := 0; index < observation.Count(); index++ {
			scalar, scalarOK := observation.At(index)
			if !scalarOK || !rule.applyScalar(raw, descriptor, payloadTag, scalar, view, access, slot, payload, keyChild, result, frozen, changed, preserved) {
				return false
			}
		}
		return true
	case rawPayloadInitial:
		return false
	default:
		return false
	}
}

func (rule *RawSetRule) applyScalar(
	raw heapdomain.RawAccess, descriptor rawPayload, payloadTag heapdomain.RawPayloadTag, scalar pack.Scalar, view rawSetView,
	access Index, slot heapdomain.Slot, payload heapdomain.Payload, keyChild heapdomain.Containment,
	result *heapdomain.Value, frozen, changed, preserved *bool,
) bool {
	if scalar.Kind() == pack.ScalarEndpoint {
		source, sourceOK := rule.packSchema().ScalarSource(scalar)
		tag, tagOK := rule.sourceTag(payloadTag, source)
		return sourceOK && tagOK && rule.applySourceTag(rule.heapSchema(), raw, tag, view, access, slot, payload, keyChild, result, frozen, changed, preserved)
	}
	kinds, kindsOK := descriptor.payload.ScalarMayRuntimeKinds(scalar)
	value, valueOK := rule.valueSchema().ForRuntimeKinds(kinds)
	return kindsOK && valueOK && rule.applySourceValue(rule.heapSchema(), raw, value, access, slot, payload, keyChild, result, frozen, changed, preserved)
}

func (rule *RawSetRule) applySourceTag(
	schema heapdomain.Schema, raw heapdomain.RawAccess, tag RawSourceTag, view rawSetView,
	access Index, slot heapdomain.Slot, payload heapdomain.Payload, keyChild heapdomain.Containment,
	result *heapdomain.Value, frozen, changed, preserved *bool,
) bool {
	selected := view.source(tag)
	if !selected.valid || !selected.found {
		return false
	}
	if !selected.present || selected.value.IsBottom() {
		*preserved = true
		return true
	}
	return rule.applySourceValue(schema, raw, selected.value, access, slot, payload, keyChild, result, frozen, changed, preserved)
}

func (rule *RawSetRule) applySourceValue(
	schema heapdomain.Schema, raw heapdomain.RawAccess, source valuedomain.Value,
	access Index, slot heapdomain.Slot, payload heapdomain.Payload, keyChild heapdomain.Containment,
	result *heapdomain.Value, frozen, changed, preserved *bool,
) bool {
	values := rule.valueSchema()
	if !values.Equal(source, source) {
		return false
	}
	if source.IsBottom() {
		*preserved = true
		return true
	}
	if source.IsTop() {
		return rule.applyTop(schema, raw, access, slot, payload, keyChild, result, frozen, changed)
	}
	atoms := 0
	if !values.VisitAtoms(source, func(atom valuedomain.Atom) bool {
		atoms++
		if atom.RuntimeKinds().Contains(runtimekind.Nil) {
			return rule.applyDelete(schema, raw, result, frozen, changed)
		}
		valueChild, valueChildOK := keymatch.Containment(schema, values, atom)
		cell, cellOK := schema.CellPresent(slot, payload, valueChild, keyChild)
		return valueChildOK && cellOK && rule.applyStore(schema, raw, cell, result, frozen, changed)
	}) {
		return false
	}
	if atoms == 0 {
		*preserved = true
	}
	return true
}

// applyTop stores an unconstrained right-hand side. Top denotes every sealed
// alternative, so the write must install the child edges an enumerated source
// would install: the read lens selects a disjoint stored class per containment
// kind, and one opaque edge therefore answers only the untracked band while
// dropping every tracked allocation and boot child the same write admits.
// Every alternative lands at the same selected key, so their disjunction is one
// cell rather than one world apiece, and the payload-class quotient keeps that
// cell finite by collapsing alternatives that denote the same child edge.
func (rule *RawSetRule) applyTop(
	schema heapdomain.Schema, raw heapdomain.RawAccess, access Index, slot heapdomain.Slot,
	payload heapdomain.Payload, keyChild heapdomain.Containment, result *heapdomain.Value, frozen, changed *bool,
) bool {
	values := rule.valueSchema()
	if values == nil || rule.topology == nil || rule.topology.selectors == nil {
		return false
	}
	// Top admits nil, so raw absence remains one branch of the write.
	if !rule.applyDelete(schema, raw, result, frozen, changed) {
		return false
	}
	var merged heapdomain.CellState
	have := false
	if !rule.topology.selectors.VisitPayloadClasses(values.Top(), func(atom valuedomain.Atom) bool {
		if atom.RuntimeKinds().Contains(runtimekind.Nil) {
			return true
		}
		valueChild, valueChildOK := keymatch.Containment(schema, values, atom)
		if !valueChildOK {
			return false
		}
		cell, cellOK := schema.CellPresent(slot, payload, valueChild, keyChild)
		if !cellOK {
			return false
		}
		if !have {
			merged, have = cell, true
			return true
		}
		union, unionOK := schema.CellUnion(merged, cell)
		if !unionOK {
			return false
		}
		merged = union
		return true
	}) {
		return false
	}
	if !have {
		return true
	}
	return rule.applyStore(schema, raw, merged, result, frozen, changed)
}

func (rule *RawSetRule) applyStore(schema heapdomain.Schema, raw heapdomain.RawAccess, cell heapdomain.CellState, result *heapdomain.Value, frozen, changed *bool) bool {
	branches, ok := schema.RawStore(raw, cell, heapdomain.MutationLicence{})
	if !ok {
		return false
	}
	if branches.FrozenError() {
		*frozen = true
	}
	if next, nextOK := branches.Normal(); nextOK {
		joined, joinedOK := heapdomain.Join(*result, next)
		if !joinedOK {
			return false
		}
		*result = joined
		*changed = true
	}
	return true
}

func (rule *RawSetRule) applyDelete(schema heapdomain.Schema, raw heapdomain.RawAccess, result *heapdomain.Value, frozen, changed *bool) bool {
	branches, ok := schema.RawDelete(raw, heapdomain.MutationLicence{})
	if !ok {
		return false
	}
	if branches.FrozenError() {
		*frozen = true
	}
	if next, nextOK := branches.Normal(); nextOK {
		joined, joinedOK := heapdomain.Join(*result, next)
		if !joinedOK {
			return false
		}
		*result = joined
		*changed = true
	}
	return true
}

func (rule *RawSetRule) takeSetScratch() *rawSetScratch {
	value, ok := rule.scratch.Get().(*rawSetScratch)
	if !ok || value == nil {
		return &rawSetScratch{}
	}
	return value
}

func (rule *RawSetRule) putSetScratch(value *rawSetScratch) {
	if value != nil {
		rule.scratch.Put(value)
	}
}
