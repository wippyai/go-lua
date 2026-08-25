package index

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// RawSetRule is Heap's one indexed-write judgment.  It admits only the
// already-sealed Index write geometry and routes its output through Heap's
// exact RawStore/RawDelete owner operations.  Value and Pack are read owners;
// this Rule never creates a mutation licence, a source identity, or an
// ObjectInit mutation path.
type RawSetRule struct {
	receiver engine.Read[engine.OrderedCells[valuedomain.Value]]
	key      engine.Read[engine.Selection[uint64, engine.OrderedCells[valuedomain.Value]]]
	heapRead engine.Read[engine.Selection[heapdomain.RawRouteTag, engine.OrderedCells[heapdomain.Value]]]
	packRead engine.Read[engine.Selection[heapdomain.RawPayloadTag, engine.OrderedCells[pack.Value]]]
	source   engine.Read[engine.Selection[RawSourceTag, engine.OrderedCells[valuedomain.Value]]]

	scratch  sync.Pool
	runtime  *rawSetRuntime
	topology *Topology
}

// rawSetRuntime is the shared reducer's typed owner seam. Legacy declaration
// installs the same operations from cold Owners; receipt-native construction
// installs them from HotOwners and exact SchemaBinding cells. The reducer
// never receives a copied Factor, coordinate map, or erased callback.
type rawSetRuntime struct {
	values      *valuedomain.Schema
	heap        heapdomain.Schema
	valueRoute  func(engine.SelectorContext, valuedomain.Coordinate, uint64) bool
	sourceRoute func(engine.SelectorContext, valuedomain.Coordinate, RawSourceTag) bool
	heapRoute   func(engine.SelectorContext, heapdomain.Key, heapdomain.RawRouteTag) bool
	packRoute   func(engine.SelectorContext, pack.Root, heapdomain.RawPayloadTag) bool
}

func (rule *RawSetRule) valueSchema() *valuedomain.Schema {
	if rule == nil || rule.runtime == nil {
		return nil
	}
	return rule.runtime.values
}
func (rule *RawSetRule) heapSchema() heapdomain.Schema {
	if rule == nil || rule.runtime == nil {
		return heapdomain.Schema{}
	}
	return rule.runtime.heap
}
func (rule *RawSetRule) packSchema() *pack.Schema {
	if rule == nil || rule.topology == nil {
		return nil
	}
	return rule.topology.packs
}

func (rule *RawSetRule) valueRoute(context engine.SelectorContext, coordinate valuedomain.Coordinate, tag uint64) bool {
	if rule == nil || rule.runtime == nil || rule.runtime.valueRoute == nil {
		return false
	}
	return rule.runtime.valueRoute(context, coordinate, tag)
}
func (rule *RawSetRule) sourceRoute(context engine.SelectorContext, coordinate valuedomain.Coordinate, tag RawSourceTag) bool {
	if rule == nil || rule.runtime == nil || rule.runtime.sourceRoute == nil {
		return false
	}
	return rule.runtime.sourceRoute(context, coordinate, tag)
}
func (rule *RawSetRule) heapRoute(context engine.SelectorContext, key heapdomain.Key, tag heapdomain.RawRouteTag) bool {
	if rule == nil || rule.runtime == nil || rule.runtime.heapRoute == nil {
		return false
	}
	return rule.runtime.heapRoute(context, key, tag)
}
func (rule *RawSetRule) packRoute(context engine.SelectorContext, root pack.Root, tag heapdomain.RawPayloadTag) bool {
	if rule == nil || rule.runtime == nil || rule.runtime.packRoute == nil {
		return false
	}
	return rule.runtime.packRoute(context, root, tag)
}

// rawSetScratch is solve-local indexing storage. Tags remain the semantic
// route/source identities issued by Heap/Value/Pack; these indexes only avoid
// repeatedly scanning one authenticated Selection during one reduction.
type rawSetScratch struct {
	pack   rawSelectionIndex
	source rawSelectionIndex
}

func (rule *RawSetRule) valid() bool {
	return rule != nil && rule.runtime != nil && rule.topology != nil && rule.topology.valid() && rule.valueSchema() != nil && rule.heapSchema().Valid() && rule.packSchema() != nil
}

func (rule *RawSetRule) owns(access Index) bool {
	return rule.valid() && access.valid() && access.topology == rule.topology && rule.topology.values == rule.valueSchema() && rule.topology.heap == rule.heapSchema() && access.Write()
}

// payloadForWrite reissues the exact Heap payload tag from IndexAccessGeometry
// before using the cold descriptor lookup.
func (rule *RawSetRule) payloadForWrite(access Index) (RawPayload, bool) {
	if !rule.owns(access) || rule.topology.catalog == nil {
		return RawPayload{}, false
	}
	return rule.topology.RawWritePayload(access)
}

func (rule *RawSetRule) sourcesFor(access Index) []rawSource {
	if rule != nil && rule.owns(access) && rule.topology.catalog != nil {
		return rule.topology.catalog.sources
	}
	return nil
}

func (rule *RawSetRule) locateKey(context engine.SelectorContext, access Index) bool {
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
	return rule.valueRoute(context, coordinate, uint64(1))
}

func (rule *RawSetRule) locateHeap(context engine.SelectorContext, access Index) bool {
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
	keyValid := false
	if !rule.visitContextKeySelectors(context, access, func(heapdomain.KeySelector) bool {
		keyValid = true
		return true
	}) {
		return false
	}
	if !keyValid {
		return true
	}
	return rule.topology.VisitReceiver(receiver, nil, func(route Route) bool {
		key, role, rooted := route.Root()
		if !rooted {
			// Unknown and non-table alternatives have no finite Heap key to
			// mutate. They remain represented by the receiver Value itself.
			return true
		}
		tag, tagged := rule.heapSchema().RouteTag(key, role)
		return tagged && rule.heapRoute(context, key, tag)
	})
}

func (rule *RawSetRule) visitContextKeySelectors(context engine.SelectorContext, access Index, visit func(heapdomain.KeySelector) bool) bool {
	if visit == nil {
		return false
	}
	if _, dynamic := access.DynamicKey(); !dynamic {
		slot, ok := access.Slot()
		if !ok {
			return false
		}
		selector, ok := rule.heapSchema().SelectorForSlot(slot)
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
	return rule.topology.selectors.Visit(fact, visit)
}
