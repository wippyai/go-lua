package index

import (
	"sync"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/heap/keymatch"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// RawSetRule is Heap's one indexed-write judgment.  It admits only the
// already-sealed Access write geometry and routes its output through Heap's
// exact RawStore/RawDelete owner operations.  Value and Pack are read owners;
// this Rule never creates a mutation licence, a source identity, or an
// ObjectInit mutation path.
type RawSetRule struct {
	topology *Topology
	values   *valueowner.Owner
	heap     *heapowner.Owner
	packs    *packowner.Owner

	selectors *keymatch.SelectorProjection
	payloads  []rawPayload
	sources   []rawSource
	// writePayloads is cold declaration support keyed by Heap's existing
	// IndexAccess handle. It is a descriptor lookup, not a second access or
	// identity plane: every use reissues and cross-checks the Heap payload and
	// IndexGeometry through the owning Schema.
	writePayloads map[heapdomain.IndexAccess]rawSetPayload

	rule     *engine.Rule[heapdomain.Value, Access]
	receiver engine.Read[engine.OrderedCells[valuedomain.Value]]
	key      engine.Read[engine.Selection[uint64, engine.OrderedCells[valuedomain.Value]]]
	heapRead engine.Read[engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]]]
	packRead engine.Read[engine.Selection[heapdomain.RawPayloadTag, engine.OrderedCells[pack.Value]]]
	source   engine.Read[engine.Selection[rawSourceTag, engine.OrderedCells[valuedomain.Value]]]
	write    engine.Write[heapdomain.Value]

	scratch sync.Pool
}

type rawSetPayload struct {
	tag        heapdomain.RawPayloadTag
	descriptor rawPayload
}

// rawSetScratch is solve-local indexing storage. Tags remain the semantic
// route/source identities issued by Heap/Value/Pack; these indexes only avoid
// repeatedly scanning one authenticated Selection during one reduction.
type rawSetScratch struct {
	pack   rawSelectionIndex
	source rawSelectionIndex
}

// DeclareRawSet declares the sole Heap-owned indexed-write Rule.  Pack is a
// required read owner even for fixed writes: the sealed payload descriptor is
// complete only when Tail and NilFill forms have been admitted explicitly.
func DeclareRawSet(
	composition *engine.Composition,
	semantic, family, evidence engine.SemanticKey,
	topology *Topology,
	values *valueowner.Owner,
	heap *heapowner.Owner,
	packs *packowner.Owner,
) (*RawSetRule, bool) {
	if composition == nil || topology == nil || values == nil || heap == nil || packs == nil ||
		!semantic.Available() || !family.Available() || !evidence.Available() ||
		semantic == family || semantic == evidence || family == evidence ||
		!topology.valid() || values.Schema() != topology.values || heap.Schema() != topology.heap ||
		packs.Schema() == nil || packs.Schema().Link() != values.Schema().Link() {
		return nil, false
	}
	payloads, sources, writePayloads, ok := buildRawSetPayloads(topology, packs.Schema())
	if !ok {
		return nil, false
	}
	selectors, ok := keymatch.NewSelectorProjection(heap.Schema(), values.Schema())
	if !ok {
		return nil, false
	}
	result := &RawSetRule{
		topology: topology, values: values, heap: heap, packs: packs,
		selectors: selectors, payloads: payloads, sources: sources,
		writePayloads: writePayloads,
	}
	result.scratch.New = func() any { return &rawSetScratch{} }
	result.scratch.Put(&rawSetScratch{})
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[heapdomain.Value, Access]{
		Semantic: semantic, OperandFamily: family, OperandContent: rawSetContent,
		Output: heap.Output(), Inputs: 3,
		Admission: engine.AdmitRuleByDerivation(evidence, result.check(semantic)),
		Transfer:  result.transfer,
	}, result.declare)
	if !ok || declared == nil || result.rule != declared {
		return nil, false
	}
	return result, true
}

func rawSetContent(access Access) (Access, [32]byte, bool) {
	id, ok := access.ID()
	return access, [32]byte(id), ok && access.Write()
}

func (rule *RawSetRule) valid() bool {
	return rule != nil && rule.topology != nil && rule.topology.valid() && rule.values != nil && rule.heap != nil && rule.packs != nil &&
		rule.values.Schema() == rule.topology.values && rule.heap.Schema() == rule.topology.heap &&
		rule.packs.Schema() != nil && rule.packs.Schema().Link() == rule.values.Schema().Link() &&
		rule.selectors != nil && rule.writePayloads != nil
}

func (rule *RawSetRule) owns(access Access) bool {
	return rule.valid() && rule.topology.OwnsAccess(access) && access.Write()
}

// buildRawSetPayloads seals the complete write geometry against the existing
// Heap RawPayload universe. Every admitted write must resolve one descriptor;
// a missing Fixed, Tail, or NilFill row is a declaration failure, never an
// omitted write branch.
func buildRawSetPayloads(topology *Topology, packs *pack.Schema) ([]rawPayload, []rawSource, map[heapdomain.IndexAccess]rawSetPayload, bool) {
	payloads, sources, ok := buildRawPayloads(topology, packs)
	if !ok || topology == nil || !topology.valid() {
		return nil, nil, nil, false
	}
	bySource := make(map[rawPayloadSource]heapdomain.RawPayloadTag)
	for index := 1; index < len(payloads); index++ {
		row := payloads[index]
		if row.kind == rawPayloadInvalid || row.source.values == 0 {
			continue
		}
		if previous, exists := bySource[row.source]; exists && previous != heapdomain.RawPayloadTag(index) {
			return nil, nil, nil, false
		}
		bySource[row.source] = heapdomain.RawPayloadTag(index)
	}
	writePayloads := make(map[heapdomain.IndexAccess]rawSetPayload)
	for index := 0; index < topology.heap.IndexAccessCount(); index++ {
		indexAccess, accessOK := topology.heap.IndexAccessAt(index)
		access, topologyOK := topology.Access(indexAccess)
		if !accessOK || !topologyOK {
			return nil, nil, nil, false
		}
		if !access.Write() {
			continue
		}
		geometry, geometryOK := topology.heap.IndexAccessGeometry(indexAccess)
		payload, payloadOK := topology.heap.PayloadForIndexAccess(indexAccess)
		shard, valuesTerm, offset, sourceOK := payload.Source()
		if !geometryOK || !payloadOK || geometry.ReadTerm != 0 || geometry.WriteTerm == 0 ||
			geometry.Shard != shard || geometry.Values != valuesTerm || geometry.Position != offset || !sourceOK {
			return nil, nil, nil, false
		}
		key := rawPayloadSource{shard: shard, values: valuesTerm, offset: offset}
		tag, found := bySource[key]
		descriptor, descriptorOK := payloadAt(payloads, tag)
		if !found || !descriptorOK || descriptor.source != key {
			return nil, nil, nil, false
		}
		writePayloads[indexAccess] = rawSetPayload{tag: tag, descriptor: descriptor}
	}
	return payloads, sources, writePayloads, true
}

// payloadForWrite reissues the exact Heap payload and cross-checks its source
// against IndexAccessGeometry before using the cold descriptor lookup.
func (rule *RawSetRule) payloadForWrite(access Access) (rawSetPayload, bool) {
	if !rule.owns(access) {
		return rawSetPayload{}, false
	}
	indexAccess, indexOK := access.IndexAccess()
	geometry, geometryOK := rule.heap.Schema().IndexAccessGeometry(indexAccess)
	payload, payloadOK := rule.heap.Schema().PayloadForIndexAccess(indexAccess)
	shard, valuesTerm, offset, sourceOK := payload.Source()
	if !indexOK || !geometryOK || !payloadOK || geometry.ReadTerm != 0 || geometry.WriteTerm == 0 ||
		geometry.Shard != shard || geometry.Values != valuesTerm || geometry.Position != offset || !sourceOK {
		return rawSetPayload{}, false
	}
	descriptor, found := rule.writePayloads[indexAccess]
	if !found || descriptor.descriptor.source != (rawPayloadSource{shard: shard, values: valuesTerm, offset: offset}) {
		return rawSetPayload{}, false
	}
	return descriptor, true
}

func (rule *RawSetRule) declare(raw *engine.Rule[heapdomain.Value, Access]) bool {
	valueIn, a := raw.InputAt(0)
	heapIn, b := raw.InputAt(1)
	packIn, c := raw.InputAt(2)
	if !a || !b || !c {
		return false
	}
	var ok bool
	rule.receiver, ok = engine.ReadFrom(raw, valueIn, rule.values.ExactRead())
	if !ok {
		return false
	}
	rule.key, ok = engine.SelectRead[heapdomain.Value, Access, valuedomain.Value, engine.OrderedCells[valuedomain.Value], uint64](raw, valueIn, rule.values.ExactRead(), []engine.Dependency{engine.ReadDependency(rule.receiver)}, rule.locateKey)
	if !ok {
		return false
	}
	rule.heapRead, ok = engine.SelectRead[heapdomain.Value, Access, heapdomain.Value, engine.OrderedCells[heapdomain.Value], heapdomain.RawRouteTag](raw, heapIn, rule.heap.ExactRead(), []engine.Dependency{engine.ReadDependency(rule.receiver), engine.ReadDependency(rule.key)}, rule.locateHeap)
	if !ok {
		return false
	}
	rule.packRead, ok = engine.SelectRead[heapdomain.Value, Access, pack.Value, engine.OrderedCells[pack.Value], heapdomain.RawPayloadTag](raw, packIn, rule.packs.ExactRead(), []engine.Dependency{engine.ReadDependency(rule.receiver), engine.ReadDependency(rule.key), engine.ReadDependency(rule.heapRead)}, rule.locatePack)
	if !ok {
		return false
	}
	rule.source, ok = engine.SelectRead[heapdomain.Value, Access, valuedomain.Value, engine.OrderedCells[valuedomain.Value], rawSourceTag](raw, valueIn, rule.values.ExactRead(), []engine.Dependency{engine.ReadDependency(rule.receiver), engine.ReadDependency(rule.key), engine.ReadDependency(rule.heapRead), engine.ReadDependency(rule.packRead)}, rule.locateSource)
	if !ok {
		return false
	}
	if !engine.CarryFrom(raw, heapIn, rule.heap.Carry()) {
		return false
	}
	rule.write, ok = engine.RouteWrite(raw, rule.heapRead)
	if ok {
		rule.rule = raw
	}
	return ok
}

// Instance binds only existing receiver and factor-owned selector forms. The
// RHS is resolved by its sealed Payload kind during transfer/evidence, so a
// Tail or NilFill row never needs a fabricated Value coordinate here.
func (rule *RawSetRule) Instance(access Access) (*engine.RuleInstance[heapdomain.Value, Access], bool) {
	_, payloadOK := rule.payloadForWrite(access)
	if rule == nil || rule.rule == nil || !payloadOK {
		return nil, false
	}
	receiver, receiverOK := access.Receiver()
	receiverRef, receiverRefOK := rule.values.Locate(receiver)
	if !receiverOK || !receiverRefOK {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, access, func(binding *engine.RuleBinding[heapdomain.Value, Access]) bool {
		return engine.InstanceRead(binding, rule.receiver, receiverRef) &&
			engine.InstanceSelectorRead(binding, rule.key, rule.values.ExactRead()) &&
			engine.InstanceSelectorRead(binding, rule.heapRead, rule.heap.ExactRead()) &&
			engine.InstanceSelectorRead(binding, rule.packRead, rule.packs.ExactRead()) &&
			engine.InstanceSelectorRead(binding, rule.source, rule.values.ExactRead()) &&
			engine.InstanceRouteWrite(binding, rule.write, rule.heapRead)
	})
}

func (rule *RawSetRule) locateKey(context engine.SelectorContext, access Access) bool {
	_, present, valid := selectorSingle(context, rule.receiver)
	if !rule.owns(access) || !valid {
		return false
	}
	if !present {
		return true
	}
	coordinate, dynamic := access.DynamicKey()
	if !dynamic {
		return true
	}
	ref, ok := rule.values.Locate(coordinate)
	return ok && engine.SelectRoute(context, ref, uint64(1))
}

func (rule *RawSetRule) locateHeap(context engine.SelectorContext, access Access) bool {
	if _, payloadOK := rule.payloadForWrite(access); !payloadOK {
		return false
	}
	receiver, present, valid := selectorSingle(context, rule.receiver)
	if !rule.owns(access) || !valid {
		return false
	}
	if !present {
		return true
	}
	return rule.visitContextKeySelectors(context, access, func(selector heapdomain.KeySelector) bool {
		return rule.topology.VisitReceiver(receiver, nil, func(route Route) bool {
			if route.Kind() != RouteRoot {
				return true
			}
			key, role, rooted := route.Root()
			if !rooted {
				return false
			}
			tag, tagged := rule.heap.Schema().RouteTag(key, role)
			ref, found := rule.heap.Locate(key)
			if !tagged || !found {
				return false
			}
			return engine.SelectRoute(context, ref, tag)
		})
	})
}

func (rule *RawSetRule) visitContextKeySelectors(context engine.SelectorContext, access Access, visit func(heapdomain.KeySelector) bool) bool {
	if visit == nil {
		return false
	}
	if _, dynamic := access.DynamicKey(); !dynamic {
		slot, ok := access.Slot()
		if !ok {
			return false
		}
		selector, ok := rule.heap.Schema().SelectorForSlot(slot)
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
	tag, cells, selected := engine.SelectorSelectionAt(context, selection, 0)
	if !selected || tag != 1 || cells.Count() != 1 {
		return false
	}
	fact, present, available := cells.At(0)
	if !available {
		return false
	}
	if !present {
		return true
	}
	// SelectorProjection deliberately succeeds with no callback for a
	// definitely invalid nil/NaN key. locateHeap then emits no route, and the
	// RouteWrite transfer settles that authenticated empty selection via
	// NoSelection/NoCandidate.
	return rule.selectors.Visit(fact, visit)
}

func keyContainmentFromSelector(schema heapdomain.Schema, selector heapdomain.KeySelector) (heapdomain.Containment, bool) {
	if !selector.Valid() {
		return heapdomain.Containment{}, false
	}
	none, ok := schema.ContainmentNone()
	return none, ok
}

func staticSetSelector(rule *RawSetRule, access Access) (heapdomain.KeySelector, heapdomain.Containment, bool) {
	slot, ok := access.Slot()
	if !ok {
		return heapdomain.KeySelector{}, heapdomain.Containment{}, false
	}
	selector, selectorOK := rule.heap.Schema().SelectorForSlot(slot)
	keyChild, childOK := keyContainmentFromSelector(rule.heap.Schema(), selector)
	return selector, keyChild, selectorOK && childOK
}
