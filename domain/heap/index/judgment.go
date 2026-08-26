package index

import (
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// This file is the owner's raw-access judgment surface: the mathematics of the
// indexed read and write, stated in owner types alone.
//
// A judgment here observes and answers. It selects no route, stages nothing,
// and reads no engine state, because the relational plan for raw access makes
// each dependent read an expansion that publishes a route relation and then an
// ordinary equijoin onto it. What the owner owes that plan is exactly the
// enumeration each expansion is over, and that is what these methods are.
//
// The two reductions below carry the same discipline one step further: given
// the finite observation one expansion's equijoins produce, each answers the
// one Value or Heap fact that observation settles.
//
// The hot rule calls the same methods. There is one statement of each of these
// enumerations and reductions in the analyzer, and both the standing plan and
// the protocol it replaces reach it here.

// RawPayload is one payload descriptor the catalog holds, addressed by the
// Heap-issued tag its route names it under.
type RawPayload struct {
	tag heapdomain.RawPayloadTag
	row rawPayload
}

// Available reports whether the descriptor addresses a catalog row.
func (payload RawPayload) Available() bool {
	return payload.tag != 0 && payload.row.kind != rawPayloadInvalid
}

// Tag returns the Heap-issued identity of this payload.
func (payload RawPayload) Tag() heapdomain.RawPayloadTag { return payload.tag }

// IsTail reports whether this payload is the open tail of its pack, which is
// the only payload kind a pack route is published for.
func (payload RawPayload) IsTail() bool { return payload.row.kind == rawPayloadTail }

// IsFixed reports whether this payload is a fixed slot of its pack, which is
// the payload kind a write answers from a semantic source rather than a root.
func (payload RawPayload) IsFixed() bool { return payload.row.kind == rawPayloadFixed }

// Root returns the pack root this payload projects, when it has one.
func (payload RawPayload) Root() (pack.Root, bool) {
	if !payload.Available() {
		return pack.Root{}, false
	}
	return payload.row.payload.Root()
}

// catalogPayload reads the raw catalog row for one payload tag. It requires
// only a live catalog, not a fully sealed Topology, because it is also the
// read a reduction already downstream of its own admission uses.
func (topology *Topology) catalogPayload(tag heapdomain.RawPayloadTag) (rawPayload, bool) {
	if topology == nil || topology.catalog == nil {
		return rawPayload{}, false
	}
	return payloadAt(topology.catalog.payloads, tag)
}

// catalogSource reads the raw catalog row for one source tag. See
// catalogPayload for why the fence is catalog liveness alone.
func (topology *Topology) catalogSource(tag RawSourceTag) (rawSource, bool) {
	if topology == nil || topology.catalog == nil {
		return rawSource{}, false
	}
	return sourceAt(topology.catalog.sources, tag)
}

// catalogBootInitial reads the sealed boot value for one route and payload
// tag directly from the catalog. See catalogPayload for why the fence is
// catalog liveness alone.
func (topology *Topology) catalogBootInitial(route heapdomain.RawRouteTag, payload heapdomain.RawPayloadTag) (valuedomain.Value, bool) {
	if topology == nil || topology.catalog == nil || route == 0 || payload == 0 {
		return valuedomain.Value{}, false
	}
	value, ok := topology.catalog.bootInitials[rawBootInitial{route: route, payload: payload}]
	return value, ok
}

// RawPayloadAt answers the catalog descriptor of one payload tag.
func (topology *Topology) RawPayloadAt(tag heapdomain.RawPayloadTag) (RawPayload, bool) {
	if topology == nil || !topology.valid() {
		return RawPayload{}, false
	}
	row, ok := topology.catalogPayload(tag)
	if !ok {
		return RawPayload{}, false
	}
	return RawPayload{tag: tag, row: row}, true
}

// CoordinateName answers the portable identity the sealed value schema issued
// for one of its coordinates. A raw-access route whose destination is a value
// coordinate publishes its row under this name, so the row is addressed by the
// identity the coordinate's own owner assigned and never by one the route
// derived.
func (topology *Topology) CoordinateName(coordinate valuedomain.Coordinate) (identity.ContentID, bool) {
	if topology == nil || !topology.valid() {
		return identity.ContentID{}, false
	}
	return topology.values.CoordinateContentID(coordinate)
}

// PackRootName answers the portable identity the sealed pack schema issued for
// one of its roots, which is the name a pack route publishes its row under.
func (topology *Topology) PackRootName(root pack.Root) (identity.ContentID, bool) {
	if topology == nil || !topology.valid() || topology.packs == nil {
		return identity.ContentID{}, false
	}
	return topology.packs.RootID(root)
}

// RawWritePayload answers the payload descriptor one write candidate
// addresses. The tag is reissued from the candidate's own access geometry, so
// the descriptor is the one Heap named and never one this layer chose.
func (topology *Topology) RawWritePayload(access Index) (RawPayload, bool) {
	if topology == nil || !topology.valid() || !access.valid() || access.topology != topology {
		return RawPayload{}, false
	}
	tag, ok := topology.heap.RawPayloadTagForIndexAccess(access.indexAccess)
	if !ok {
		return RawPayload{}, false
	}
	return topology.RawPayloadAt(tag)
}

// RawSourceCoordinate answers the value coordinate one semantic source names.
func (topology *Topology) RawSourceCoordinate(tag RawSourceTag) (valuedomain.Coordinate, bool) {
	if topology == nil || !topology.valid() {
		return valuedomain.Coordinate{}, false
	}
	source, ok := topology.catalogSource(tag)
	if !ok {
		return valuedomain.Coordinate{}, false
	}
	return source.coordinate, true
}

// RawBootInitial answers the sealed boot value one route and payload address,
// when the target declares one.
func (topology *Topology) RawBootInitial(route heapdomain.RawRouteTag, payload heapdomain.RawPayloadTag) (valuedomain.Value, bool) {
	if topology == nil || !topology.valid() {
		return valuedomain.Value{}, false
	}
	return topology.catalogBootInitial(route, payload)
}

// VisitPayloadSources enumerates every semantic source one payload declares,
// in the catalog's own order. It is the enumeration a raw-access source
// expansion is over: each visited source is one published row, named by the
// coordinate its own tag addresses.
func (topology *Topology) VisitPayloadSources(payload heapdomain.RawPayloadTag, visit func(RawSourceTag, valuedomain.Coordinate) bool) bool {
	if topology == nil || !topology.valid() || visit == nil {
		return false
	}
	tags, ok := topology.catalog.sourceTags(payload)
	if !ok {
		return false
	}
	for _, tag := range tags {
		coordinate, coordinateOK := topology.RawSourceCoordinate(tag)
		if !coordinateOK || !visit(tag, coordinate) {
			return false
		}
	}
	return true
}

// VisitRoutePayloads enumerates every payload one selected heap route fact
// carries under a key selector. It is the enumeration a raw-access pack
// expansion is over.
//
// A target boot payload has no program descriptor and is not a catalog row;
// the enumeration passes over it rather than refusing, because the boot value
// is answered by RawBootInitial and never by a payload descriptor.
func (topology *Topology) VisitRoutePayloads(route heapdomain.RawRouteTag, fact heapdomain.Value, selector heapdomain.KeySelector, visit func(RawPayload) bool) bool {
	if topology == nil || !topology.valid() || visit == nil {
		return false
	}
	return topology.heap.VisitRawAccessRoute(route, fact, selector, func(raw heapdomain.RawAccess) bool {
		if raw.IsTop() {
			return true
		}
		cell, ok := raw.Cell()
		if !ok {
			return false
		}
		for index := 0; index < cell.PresentCount(); index++ {
			present, presentOK := cell.PresentAt(index)
			if !presentOK {
				return false
			}
			tag, tagged := raw.PayloadTag(present)
			if !tagged {
				if _, _, initial := raw.InitialPayload(present); initial {
					continue
				}
				return false
			}
			payload, payloadOK := topology.RawPayloadAt(tag)
			if !payloadOK || !visit(payload) {
				return false
			}
		}
		return true
	})
}

// RawGetFrame is the authenticated, read-only observation the raw-get
// reduction is stated over. Every field names one owner-issued selection
// already resolved to at most one value; RawGetReduce consumes nothing
// beyond what is named here.
type RawGetFrame struct {
	Scratch     *RawGetScratch
	Key         Selected[valuedomain.Value]
	KeyCount    int
	CallCount   int
	Call        func(uint64) Selected[calldomain.Value]
	HeapCount   int
	Heap        func(heapdomain.RawRouteTag, heapdomain.Key) Selected[heapdomain.Value]
	PackCount   int
	Pack        func(heapdomain.RawPayloadTag) Selected[pack.Value]
	SourceCount int
	Source      func(RawSourceTag) Selected[valuedomain.Value]
}

type rawGetCensus struct {
	scratch *RawGetScratch
	pack    int
	source  int
}

// RawGetReduce answers the raw-get lattice value for one read candidate: the
// receiver's route set observed through receiver, and each route's dependent
// Call/Heap/Pack/Value facts observed through view. It is the one statement
// of the raw-get mathematics in the analyzer; the hot rule and a future
// standing plan reach it here.
func (topology *Topology) RawGetReduce(access Index, receiver valuedomain.Value, view RawGetFrame) (valuedomain.Value, bool, bool) {
	if topology == nil || !topology.valid() || !access.valid() || access.topology != topology || !access.Read() || view.Scratch == nil || view.Call == nil || view.Heap == nil || view.Pack == nil || view.Source == nil {
		return valuedomain.Value{}, false, false
	}
	result, any := topology.values.Bottom(), false
	selectorCount := 0
	if !topology.visitKeySelectors(access, view, func(heapdomain.KeySelector) bool {
		selectorCount++
		return true
	}) {
		return valuedomain.Value{}, false, false
	}
	if selectorCount == 0 {
		return result, false, view.CallCount == 0 && view.HeapCount == 0 && view.PackCount == 0 && view.SourceCount == 0
	}

	callCount := 0
	if !topology.VisitReceiverCallDemand(receiver, func(_ calldomain.Key, tag uint64) bool {
		selected := view.Call(tag)
		if !selected.valid || !selected.found {
			return false
		}
		callCount++
		return true
	}) || callCount != view.CallCount {
		return valuedomain.Value{}, false, false
	}
	census := rawGetCensus{scratch: view.Scratch}
	heapCount := 0
	callStateValid := true
	callState := func(_ calldomain.Key, tag uint64) (calldomain.Value, bool) {
		selected := view.Call(tag)
		if !selected.valid || !selected.found {
			callStateValid = false
			return calldomain.Value{}, false
		}
		return selected.value, selected.present
	}
	valid := topology.VisitReceiver(receiver, callState, func(route Route) bool {
		switch route.Kind() {
		case RouteRoot:
			key, role, rooted := route.Root()
			if !rooted {
				return false
			}
			tag, tagged := topology.heap.RouteTag(key, role)
			if !tagged {
				return false
			}
			selected := view.Heap(tag, key)
			if !selected.valid || !selected.found {
				return false
			}
			heapCount++
			if !selected.present {
				return true
			}
			return topology.visitKeySelectors(access, view, func(selector heapdomain.KeySelector) bool {
				return topology.applyHeap(tag, selected.value, selector, view, &census, &result, &any)
			})
		case RouteUnknown:
			return topology.joinPresentTop(&result, &any)
		case RouteOther:
			return true
		default:
			return false
		}
	})
	if !valid || !callStateValid || heapCount != view.HeapCount || census.pack != view.PackCount || census.source != view.SourceCount {
		return valuedomain.Value{}, false, false
	}
	return result, any, true
}

// VisitKeySelectors enumerates the key selectors one raw-access candidate
// resolves to. A static key resolves the one selector its own slot names; a
// dynamic key resolves the selectors the sealed projection admits for the key
// the frame delivered, and a key the frame proves absent resolves none.
//
// The projection is the owner's, so the owner is the only one that can answer,
// and the pack expansion reads the payloads a route carries under exactly
// these selectors.
func (topology *Topology) VisitKeySelectors(candidate Index, key Selected[valuedomain.Value], keyCount int, visit func(heapdomain.KeySelector) bool) bool {
	if topology == nil || !topology.valid() || !candidate.valid() || candidate.topology != topology || visit == nil {
		return false
	}
	if _, dynamic := candidate.DynamicKey(); !dynamic {
		if keyCount != 0 {
			return false
		}
		slot, slotOK := candidate.Slot()
		if !slotOK {
			return false
		}
		selector, selectorOK := topology.heap.SelectorForSlot(slot)
		return selectorOK && visit(selector)
	}
	if keyCount != 1 || !key.valid || !key.found {
		return false
	}
	if !key.present {
		return true
	}
	selectors := topology.selectors
	return selectors != nil && selectors.Visit(key.value, visit)
}

func (topology *Topology) visitKeySelectors(operand Index, view RawGetFrame, visit func(heapdomain.KeySelector) bool) bool {
	return topology.VisitKeySelectors(operand, view.Key, view.KeyCount, visit)
}

func (topology *Topology) applyHeap(tag heapdomain.RawRouteTag, fact heapdomain.Value, selector heapdomain.KeySelector, view RawGetFrame, census *rawGetCensus, result *valuedomain.Value, any *bool) bool {
	return topology.heap.VisitRawAccessRoute(tag, fact, selector, func(raw heapdomain.RawAccess) bool {
		if raw.IsTop() {
			return topology.joinPresentTop(result, any)
		}
		cell, ok := raw.Cell()
		if !ok {
			return false
		}
		for index := 0; index < cell.PresentCount(); index++ {
			present, ok := cell.PresentAt(index)
			if !ok || !topology.applyPresent(tag, raw, present, view, census, result, any) {
				return false
			}
		}
		return true
	})
}

func (topology *Topology) applyPresent(route heapdomain.RawRouteTag, raw heapdomain.RawAccess, present heapdomain.Present, view RawGetFrame, census *rawGetCensus, result *valuedomain.Value, any *bool) bool {
	valueContainment, _, ok := present.Containment()
	if !ok {
		return false
	}
	if _, _, boot := raw.InitialPayload(present); boot {
		bootTag, bootTagOK := raw.PayloadTag(present)
		if !bootTagOK {
			return false
		}
		value, ok := topology.catalogBootInitial(route, bootTag)
		return ok && topology.reduceAndJoin(valueContainment, value, result, any)
	}
	tag, ok := raw.PayloadTag(present)
	if !ok {
		return false
	}
	payload, ok := topology.catalogPayload(tag)
	if !ok {
		return false
	}
	if !topology.requirePayload(tag, payload, view, census) {
		return false
	}
	switch payload.kind {
	case rawPayloadNil:
		return true
	case rawPayloadInitial:
		return false
	case rawPayloadFixed:
		tags, tagsOK := topology.catalog.sourceTags(tag)
		if !tagsOK || len(tags) != 1 {
			return false
		}
		selected := view.Source(tags[0])
		return selected.valid && selected.found && (!selected.present || topology.reduceAndJoin(valueContainment, selected.value, result, any))
	case rawPayloadTail:
		selected := view.Pack(tag)
		if !selected.valid || !selected.found {
			return false
		}
		if !selected.present {
			return true
		}
		root, rootOK := payload.payload.Root()
		selection, selectionOK := payload.payload.Selection()
		values, valuesOK := payload.payload.Values()
		observation, observed := topology.packs.ObserveScalar(root, selected.value, values, selection)
		if !rootOK || !selectionOK || !valuesOK || !observed {
			return false
		}
		if observation.IsBottom() {
			return true
		}
		if observation.IsTop() {
			return topology.reduceAndJoin(valueContainment, topology.values.Top(), result, any)
		}
		for index := 0; index < observation.Count(); index++ {
			scalar, ok := observation.At(index)
			if !ok || !topology.applyScalar(view, tag, payload, scalar, valueContainment, result, any) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (topology *Topology) applyScalar(view RawGetFrame, payloadTag heapdomain.RawPayloadTag, payload rawPayload, scalar pack.Scalar, containment heapdomain.Containment, result *valuedomain.Value, any *bool) bool {
	if scalar.Kind() == pack.ScalarEndpoint {
		// An exact Pack endpoint is a symbolic reference to one existing Link
		// value, not a value fact embedded in Pack.  Resolve it through Pack's
		// sealed source projection, then use RawGet's already-declared Value
		// selector.  This keeps Value as the sole authority for the fact and
		// preserves the endpoint identity without a body-wide Pack aggregate.
		source, sourceOK := topology.packs.ScalarSource(scalar)
		selected := topology.sourceValue(view, payloadTag, source)
		if !sourceOK || !selected.valid || !selected.found {
			return false
		}
		if !selected.present {
			return true
		}
		return topology.reduceAndJoin(containment, selected.value, result, any)
	}
	kinds, kindsOK := payload.payload.ScalarMayRuntimeKinds(scalar)
	value, valueOK := topology.values.ForRuntimeKinds(kinds)
	return kindsOK && valueOK && topology.reduceAndJoin(containment, value, result, any)
}

func (topology *Topology) sourceValue(view RawGetFrame, payloadTag heapdomain.RawPayloadTag, want pack.SemanticSource) Selected[valuedomain.Value] {
	if topology == nil || topology.catalog == nil {
		return Selected[valuedomain.Value]{}
	}
	tag, found := topology.catalog.sourceTag(payloadTag, want)
	if !found {
		return Selected[valuedomain.Value]{}
	}
	if _, ok := topology.catalogSource(tag); !ok {
		return Selected[valuedomain.Value]{}
	}
	return view.Source(tag)
}

func (topology *Topology) requirePayload(tag heapdomain.RawPayloadTag, payload rawPayload, view RawGetFrame, census *rawGetCensus) bool {
	if census == nil || census.scratch == nil {
		return false
	}
	if payload.kind == rawPayloadTail && !marked(census.scratch.payload, uint64(tag)) {
		selected := view.Pack(tag)
		if !selected.valid || !selected.found {
			return false
		}
		mark(census.scratch.payload, uint64(tag))
		census.pack++
	}
	tags, tagsOK := topology.catalog.sourceTags(tag)
	if !tagsOK {
		return false
	}
	for _, sourceTag := range tags {
		if marked(census.scratch.source, uint64(sourceTag)) {
			continue
		}
		selected := view.Source(sourceTag)
		if !selected.valid || !selected.found {
			return false
		}
		mark(census.scratch.source, uint64(sourceTag))
		census.source++
	}
	return true
}

func (topology *Topology) reduceAndJoin(containment heapdomain.Containment, value valuedomain.Value, result *valuedomain.Value, any *bool) bool {
	values := topology.values
	var stored valuedomain.Value
	var ok bool
	switch containment.Kind() {
	case heapdomain.ContainmentNone:
		stored, ok = values.FilterStoredNone(value)
	case heapdomain.ContainmentUnknown:
		stored, ok = values.FilterStoredUnknown(value)
	case heapdomain.ContainmentExact:
		reference, referenceOK := containment.Reference()
		if !referenceOK {
			return false
		}
		var selector valuedomain.Atom
		if root, role, allocation := reference.Key(); allocation && root.Kind() == heapdomain.RootAllocation {
			selector, ok = values.Allocation(root, role)
		} else if rootID, role, boot := reference.BootID(); boot && role == materialization.Exact {
			selector, ok = values.BootID(rootID)
		}
		if ok {
			stored, ok = values.FilterStoredExact(value, selector)
		}
	default:
		return false
	}
	if !ok {
		return false
	}
	present, ok := values.FilterPresent(stored)
	if !ok {
		return false
	}
	if values.Equal(present, values.Bottom()) {
		return true
	}
	return topology.join(result, any, present)
}

func (topology *Topology) join(result *valuedomain.Value, any *bool, value valuedomain.Value) bool {
	next, ok := topology.values.Join(*result, value)
	if !ok {
		return false
	}
	*result, *any = next, true
	return true
}

func (topology *Topology) joinPresentTop(result *valuedomain.Value, any *bool) bool {
	present, ok := topology.values.FilterPresent(topology.values.Top())
	return ok && topology.join(result, any, present)
}

// RawSetFrame is the authenticated, read-only observation the raw-set commit
// is stated over. Pack/Value facts remain owner-issued selections; the frame
// carries no mutation authority beyond what is named here.
type RawSetFrame struct {
	Key         Selected[valuedomain.Value]
	KeyCount    int
	HeapCount   int
	PackCount   int
	Pack        func(heapdomain.RawPayloadTag) Selected[pack.Value]
	SourceCount int
	Source      func(RawSourceTag) Selected[valuedomain.Value]
}

// RawSetMutateRoute answers the write commit for one selected predecessor
// route: it consumes one exact sealed RHS descriptor and the Pack/Value
// observations view authenticates, then joins only the branches
// RawStore/RawDelete return. Frozen/error outcomes widen to Heap.Top and are
// never converted into ordinary writes. It is the one statement of the
// raw-set mathematics in the analyzer; the hot rule and a future standing
// plan reach it here.
func (topology *Topology) RawSetMutateRoute(access Index, route heapdomain.RawRouteTag, fact heapdomain.Value, view RawSetFrame) (heapdomain.Value, bool) {
	if topology == nil || !topology.valid() || !access.valid() || access.topology != topology || !access.Write() || !fact.Valid() {
		return heapdomain.Value{}, false
	}
	descriptor, descriptorOK := topology.RawWritePayload(access)
	if !descriptorOK {
		return heapdomain.Value{}, false
	}
	if (descriptor.IsTail() && view.Pack == nil) ||
		(descriptor.IsFixed() && view.Source == nil) {
		return heapdomain.Value{}, false
	}
	indexAccess, indexOK := access.IndexAccess()
	slot, slotOK := topology.heap.SlotForIndexAccess(indexAccess)
	payload, payloadOK := topology.heap.PayloadForIndexAccess(indexAccess)
	if !indexOK || !slotOK || !payloadOK {
		return heapdomain.Value{}, false
	}
	values := topology.values
	if _, dynamic := access.DynamicKey(); dynamic && view.Key.found && !values.Equal(view.Key.value, view.Key.value) {
		return heapdomain.Value{}, false
	}
	schema := topology.heap
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
			return topology.applyPayload(raw, descriptor.row, descriptor.tag, view, access, slot, payload, keyChild, &result, &frozen, &changed, &preserved)
		})
	}
	if _, dynamic := access.DynamicKey(); dynamic {
		if !view.Key.valid || !view.Key.found || !view.Key.present || view.Key.value.IsBottom() {
			return fact, true
		}
		if view.Key.value.IsTop() {
			unknown, unknownOK := schema.ContainmentUnknown()
			if !unknownOK || !topology.selectors.Visit(view.Key.value, func(selector heapdomain.KeySelector) bool { return apply(selector, unknown) }) {
				return heapdomain.Value{}, false
			}
		} else {
			selectors := 0
			if !values.VisitAtoms(view.Key.value, func(atom valuedomain.Atom) bool {
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
		selector, keyChild, selectorOK := staticSetSelector(schema, access)
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

func (topology *Topology) applyPayload(
	raw heapdomain.RawAccess, descriptor rawPayload, payloadTag heapdomain.RawPayloadTag,
	view RawSetFrame, access Index, slot heapdomain.Slot, payload heapdomain.Payload,
	keyChild heapdomain.Containment, result *heapdomain.Value, frozen, changed, preserved *bool,
) bool {
	if result == nil || frozen == nil || changed == nil || preserved == nil {
		return false
	}
	schema := topology.heap
	switch descriptor.kind {
	case rawPayloadNil:
		return topology.applyDelete(schema, raw, result, frozen, changed)
	case rawPayloadFixed:
		if descriptor.sourceCount != 1 {
			return false
		}
		tags, tagsOK := topology.catalog.sourceTags(payloadTag)
		if !tagsOK || len(tags) != 1 {
			return false
		}
		tag := tags[0]
		return topology.applySourceTag(schema, raw, tag, view, access, slot, payload, keyChild, result, frozen, changed, preserved)
	case rawPayloadTail:
		selected := view.Pack(payloadTag)
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
		observation, observed := topology.packs.ObserveScalar(root, selected.value, values, selection)
		if !observed {
			return false
		}
		if observation.IsBottom() {
			*preserved = true
			return true
		}
		if observation.IsTop() {
			return topology.applyTop(schema, raw, access, slot, payload, keyChild, result, frozen, changed)
		}
		for index := 0; index < observation.Count(); index++ {
			scalar, scalarOK := observation.At(index)
			if !scalarOK || !topology.applySetScalar(raw, descriptor, payloadTag, scalar, view, access, slot, payload, keyChild, result, frozen, changed, preserved) {
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

func (topology *Topology) applySetScalar(
	raw heapdomain.RawAccess, descriptor rawPayload, payloadTag heapdomain.RawPayloadTag, scalar pack.Scalar, view RawSetFrame,
	access Index, slot heapdomain.Slot, payload heapdomain.Payload, keyChild heapdomain.Containment,
	result *heapdomain.Value, frozen, changed, preserved *bool,
) bool {
	if scalar.Kind() == pack.ScalarEndpoint {
		source, sourceOK := topology.packs.ScalarSource(scalar)
		tag, tagOK := topology.catalog.sourceTag(payloadTag, source)
		return sourceOK && tagOK && topology.applySourceTag(topology.heap, raw, tag, view, access, slot, payload, keyChild, result, frozen, changed, preserved)
	}
	kinds, kindsOK := descriptor.payload.ScalarMayRuntimeKinds(scalar)
	value, valueOK := topology.values.ForRuntimeKinds(kinds)
	return kindsOK && valueOK && topology.applySourceValue(topology.heap, raw, value, access, slot, payload, keyChild, result, frozen, changed, preserved)
}

func (topology *Topology) applySourceTag(
	schema heapdomain.Schema, raw heapdomain.RawAccess, tag RawSourceTag, view RawSetFrame,
	access Index, slot heapdomain.Slot, payload heapdomain.Payload, keyChild heapdomain.Containment,
	result *heapdomain.Value, frozen, changed, preserved *bool,
) bool {
	selected := view.Source(tag)
	if !selected.valid || !selected.found {
		return false
	}
	if !selected.present || selected.value.IsBottom() {
		*preserved = true
		return true
	}
	return topology.applySourceValue(schema, raw, selected.value, access, slot, payload, keyChild, result, frozen, changed, preserved)
}

func (topology *Topology) applySourceValue(
	schema heapdomain.Schema, raw heapdomain.RawAccess, source valuedomain.Value,
	access Index, slot heapdomain.Slot, payload heapdomain.Payload, keyChild heapdomain.Containment,
	result *heapdomain.Value, frozen, changed, preserved *bool,
) bool {
	values := topology.values
	if !values.Equal(source, source) {
		return false
	}
	if source.IsBottom() {
		*preserved = true
		return true
	}
	if source.IsTop() {
		return topology.applyTop(schema, raw, access, slot, payload, keyChild, result, frozen, changed)
	}
	atoms := 0
	if !values.VisitAtoms(source, func(atom valuedomain.Atom) bool {
		atoms++
		if atom.RuntimeKinds().Contains(runtimekind.Nil) {
			return topology.applyDelete(schema, raw, result, frozen, changed)
		}
		valueChild, valueChildOK := keymatch.Containment(schema, values, atom)
		cell, cellOK := schema.CellPresent(slot, payload, valueChild, keyChild)
		return valueChildOK && cellOK && topology.applyStore(schema, raw, cell, result, frozen, changed)
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
func (topology *Topology) applyTop(
	schema heapdomain.Schema, raw heapdomain.RawAccess, access Index, slot heapdomain.Slot,
	payload heapdomain.Payload, keyChild heapdomain.Containment, result *heapdomain.Value, frozen, changed *bool,
) bool {
	values := topology.values
	if topology == nil || values == nil || topology.selectors == nil {
		return false
	}
	// Top admits nil, so raw absence remains one branch of the write.
	if !topology.applyDelete(schema, raw, result, frozen, changed) {
		return false
	}
	var merged heapdomain.CellState
	have := false
	if !topology.selectors.VisitPayloadClasses(values.Top(), func(atom valuedomain.Atom) bool {
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
	return topology.applyStore(schema, raw, merged, result, frozen, changed)
}

func (topology *Topology) applyStore(schema heapdomain.Schema, raw heapdomain.RawAccess, cell heapdomain.CellState, result *heapdomain.Value, frozen, changed *bool) bool {
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

func (topology *Topology) applyDelete(schema heapdomain.Schema, raw heapdomain.RawAccess, result *heapdomain.Value, frozen, changed *bool) bool {
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

func keyContainmentFromSelector(schema heapdomain.Schema, selector heapdomain.KeySelector) (heapdomain.Containment, bool) {
	if !selector.Valid() {
		return heapdomain.Containment{}, false
	}
	none, ok := schema.ContainmentNone()
	return none, ok
}

func staticSetSelector(schema heapdomain.Schema, access Index) (heapdomain.KeySelector, heapdomain.Containment, bool) {
	slot, ok := access.Slot()
	if !ok {
		return heapdomain.KeySelector{}, heapdomain.Containment{}, false
	}
	selector, selectorOK := schema.SelectorForSlot(slot)
	keyChild, childOK := keyContainmentFromSelector(schema, selector)
	return selector, keyChild, selectorOK && childOK
}
