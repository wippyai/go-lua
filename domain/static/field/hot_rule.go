package field

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapindex "github.com/wippyai/go-lua/domain/heap/index"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	staticowner "github.com/wippyai/go-lua/domain/static/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// HotRule is Static's owner-local direct-field transfer. It retains only the
// existing mounted Index operand, one Static read capability, and the two
// sealed owners needed to reissue that operand. It does not retain an Index
// directory, a copied Value/Heap fact, or a second coordinate denominator.
type HotRule struct {
	implementation *staticowner.RuleImplementation[heapindex.Index]
	receiverRead   engine.Read[engine.OrderedCells[staticdomain.TypeFact]]
	statics        *staticowner.HotOwner
	heap           *heapowner.HotOwner
	topology       *heapindex.Topology
}

// BindHot binds the Static direct-field rule to the exact shared schema
// binding. Heap is used only to reissue the already-sealed mounted occurrence
// row; the transfer itself reads and writes Static TypeFacts exclusively.
func BindHot(binding *engine.SchemaBinding, fragment *SchemaFragment, statics *staticowner.HotOwner, heap *heapowner.HotOwner, topology *heapindex.Topology) (*HotRule, bool) {
	if binding == nil || fragment == nil || fragment.slot == nil || statics == nil || !statics.MatchesBinding(binding) || statics.Classes() == nil || statics.ValueSchema() == nil || heap == nil || !heap.MatchesBinding(binding) || !heap.Schema().Valid() || topology == nil {
		return nil, false
	}
	rule := &HotRule{statics: statics, heap: heap, topology: topology}
	implementation, bound := staticowner.BindCarryRuleDirect(statics, fragment.slot, fragment.carry, fragment.write, statics.FactorRef(), engine.HotRuleSpec[staticdomain.TypeFact, heapindex.Index]{
		OperandContent:  rule.operandContent,
		OperandResolver: rule.resolveOperand,
		Fold:            rule.fold,
	}, engine.HotCarrySpec[staticdomain.TypeFact, heapindex.Index]{}, func(access heapindex.Index) (uint64, bool) {
		result, resultOK := access.Result()
		index, indexOK := statics.ValueSchema().CoordinateIndex(result)
		return uint64(index), resultOK && indexOK
	})
	if !bound || implementation == nil {
		return nil, false
	}
	receiverRead, readOK := staticowner.AddSelectedRuleDirectExactRead(implementation, fragment.receiverRead, statics.FactorRef(), func(access heapindex.Index) (uint64, bool) {
		receiver, receiverOK := access.Receiver()
		index, indexOK := statics.ValueSchema().CoordinateIndex(receiver)
		return uint64(index), receiverOK && indexOK
	})
	if !readOK {
		return nil, false
	}
	rule.implementation, rule.receiverRead = implementation, receiverRead
	return rule, true
}

func (rule *HotRule) Implementation() (*staticowner.RuleImplementation[heapindex.Index], bool) {
	if rule == nil || rule.statics == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := staticowner.ResolveRuleImplementationFor(rule.statics, rule.implementation)
	return rule.implementation, ok
}

func (rule *HotRule) valid() bool {
	return rule != nil && rule.statics != nil && rule.statics.Classes() != nil && rule.statics.ValueSchema() != nil && rule.heap != nil && rule.heap.Schema().Valid() && rule.topology != nil
}

// operandContent uses Index's own sealed identity. The rule never hashes or
// copies receiver/result coordinates to manufacture a parallel operand ID.
func (rule *HotRule) operandContent(access heapindex.Index) (heapindex.Index, [32]byte, bool) {
	if !rule.valid() || !access.Read() || !directGeometry(rule.statics.ValueSchema(), access) {
		return heapindex.Index{}, [32]byte{}, false
	}
	id, ok := access.ID()
	if !ok || !id.Available() {
		return heapindex.Index{}, [32]byte{}, false
	}
	return access, [32]byte(id), true
}

// resolveOperand reissues Heap's canonical mounted index row. It follows the
// same occurrence inverse as heap/index's raw-read rules, preserving duplicate
// mounts and the exact row's receiver/result geometry.
func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (heapindex.Index, bool) {
	if !rule.valid() || !coords.Mount.Available() || !coords.Occurrence.Available() {
		return heapindex.Index{}, false
	}
	mount, mountOK := rule.heap.Schema().OccurrenceMountForModule(coords.Mount)
	if !mountOK {
		return heapindex.Index{}, false
	}
	indexAccess, accessOK := mount.IndexAccessForOccurrence(coords.Occurrence, true)
	if !accessOK {
		return heapindex.Index{}, false
	}
	access, accessOK := rule.topology.Access(indexAccess)
	if !accessOK || !access.Read() || !directGeometry(rule.statics.ValueSchema(), access) {
		return heapindex.Index{}, false
	}
	return access, true
}

func (rule *HotRule) fold(frame engine.Frame[staticdomain.TypeFact, heapindex.Index]) engine.RuleResult[staticdomain.TypeFact] {
	if !rule.valid() {
		return engine.RuleResult[staticdomain.TypeFact]{}
	}
	access, operandOK := engine.Operand(frame)
	if !operandOK || !access.Read() || !directGeometry(rule.statics.ValueSchema(), access) {
		return engine.RuleResult[staticdomain.TypeFact]{}
	}
	cells, readOK := engine.ReadValue(frame, rule.receiverRead)
	if !readOK || cells.Count() != 1 {
		return engine.RuleResult[staticdomain.TypeFact]{}
	}
	base, present, available := cells.At(0)
	if !available {
		return engine.RuleResult[staticdomain.TypeFact]{}
	}
	// An unwritten receiver is not evidence for a field projection. The
	// explicit NoCandidate result lets Static's carry preserve prior facts
	// without manufacturing AnyValue or language-level Unknown.
	if !present {
		return engine.NoCandidate(frame)
	}
	key, keyOK := directFieldKey(access)
	if !keyOK || !base.Valid() || !rule.statics.Classes().OwnsTypeFact(base) {
		return engine.NoCandidate(frame)
	}
	projected, projectedOK := rule.statics.Classes().TypeFactField(base, key)
	if !projectedOK || !projected.Valid() || !rule.statics.Classes().OwnsTypeFact(projected) {
		// Missing concrete fields, foreign facts, and derived/opaque classes all
		// abstain here. In particular, none widen to TypeTop/AnyValue.
		return engine.NoCandidate(frame)
	}
	return engine.Staged(frame, projected)
}

func directGeometry(values *valuedomain.Schema, access heapindex.Index) bool {
	if values == nil || !values.Valid() || !access.Read() {
		return false
	}
	receiver, receiverOK := access.Receiver()
	result, resultOK := access.Result()
	_, receiverIndexOK := values.CoordinateIndex(receiver)
	_, resultIndexOK := values.CoordinateIndex(result)
	return receiverOK && resultOK && receiverIndexOK && resultIndexOK
}

// directFieldKey accepts only Heap's one exact literal slot, and only its
// string member. Dynamic, finite, kind, and object-reference selectors are
// unsupported direct fields and therefore never become a guessed spelling.
func directFieldKey(access heapindex.Index) (string, bool) {
	slot, slotOK := access.Slot()
	if !slotOK {
		return "", false
	}
	kind, exact, _, originOK := slot.Origin()
	if !originOK || kind != heapdomain.SlotExact {
		return "", false
	}
	literal, literalOK := exact.Literal()
	if !literalOK || literal.Kind != keyspace.LiteralString {
		return "", false
	}
	return literal.String, true
}

// The mounted Index row already authenticates both coordinates. The helper
// deliberately reissues neither coordinate nor a second index; it is only a
// small readability fence shared by the operand and Fold paths.
