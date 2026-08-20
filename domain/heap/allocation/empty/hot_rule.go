package empty

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
)

// HotRule is Empty allocation's exact-read/one-carry receipt-native vertical.
// It retains only the Heap-owned issuer and the read receipt issued with the
// exact cold Rule cell; it does not retain a legacy Rule or Factor authority.
type HotRule struct {
	implementation *heapowner.RuleImplementation[source.Root]
	owner          *heapowner.HotOwner
	read           engine.Read[engine.OrderedCells[heapdomain.Value]]
	catalog        *allocationcatalog.Catalog
	schema         heapdomain.Schema
}

// BindHot attaches Empty's private transform and exact row fold to its cold
// fragment through Heap's already-bound output Factor.
func BindHot(fragment *SchemaFragment, owner *heapowner.HotOwner, catalog *allocationcatalog.Catalog) (*HotRule, bool) {
	if fragment == nil || fragment.slot == nil || owner == nil || !owner.Schema().Valid() ||
		catalog == nil || !catalog.FencedToHeap(owner.Schema()) ||
		!fragment.semantic.Available() || !fragment.transform.Available() ||
		!identity.DistinctKeys(fragment.semantic, fragment.transform) {
		return nil, false
	}
	var runtimeRead engine.Read[engine.OrderedCells[heapdomain.Value]]
	implementation, read, ok := heapowner.BindExactReadAndCarryRule(owner, fragment.slot, fragment.read, fragment.carry, fragment.write, engine.HotRuleSpec[heapdomain.Value, source.Root]{
		// Root.New issued the complete cold classification receipt. The hot
		// member, transfer, and derivation paths use only its O(1) fence.
		OperandContent: func(operand source.Root) (source.Root, [32]byte, bool) {
			return emptyContent(owner.Schema(), operand)
		},
		Fold: func(frame engine.Frame[heapdomain.Value, source.Root]) engine.RuleResult[heapdomain.Value] {
			operand, operandOK := engine.Operand(frame)
			if !operandOK {
				return engine.RuleResult[heapdomain.Value]{}
			}
			cells, cellsOK := engine.ReadValue(frame, runtimeRead)
			if !cellsOK || cells.Count() != 1 {
				return engine.RuleResult[heapdomain.Value]{}
			}
			predecessor, present, available := cells.At(0)
			if !available {
				return engine.RuleResult[heapdomain.Value]{}
			}
			if !present {
				return engine.NoCandidate(frame)
			}
			_, next, resultOK := emptyResult(owner.Schema(), operand, predecessor)
			if !resultOK {
				return engine.RuleResult[heapdomain.Value]{}
			}
			return engine.Staged(frame, next)
		},
	}, engine.HotCarrySpec[heapdomain.Value, source.Root]{
		Apply: func(operand source.Root, predecessor heapdomain.Value) (heapdomain.Value, bool) {
			return owner.Schema().Age(predecessor, operand.Key())
		},
	}, func(operand source.Root) (uint64, bool) {
		index, ok := owner.Schema().KeyIndex(operand.Key())
		return uint64(index), ok && index >= 0
	}, func(operand source.Root) (uint64, bool) {
		index, ok := owner.Schema().KeyIndex(operand.Key())
		return uint64(index), ok && index >= 0
	})
	if !ok || implementation == nil {
		return nil, false
	}
	runtimeRead = read
	rule := &HotRule{implementation: implementation, owner: owner, read: read, catalog: catalog, schema: owner.Schema()}
	if !implementation.InstallOperandResolver(rule.resolveOperand) {
		return nil, false
	}
	return rule, true
}

func (rule *HotRule) resolveOperand(coords engine.OperandCoords) (source.Root, bool) {
	if rule == nil || rule.catalog == nil {
		return source.Root{}, false
	}
	mount, mountOK := rule.catalog.ForMount(coords.Mount)
	root, ok := mount.RootForOccurrence(coords.Occurrence)
	return root, mountOK && ok && root.Form() == source.FormEmpty && root.FencedTo(rule.schema)
}

// Implementation returns Heap owner's opaque receipt issuer only after the
// exact SchemaBinding seals.
func (rule *HotRule) Implementation() (*heapowner.RuleImplementation[source.Root], bool) {
	if rule == nil || rule.implementation == nil {
		return nil, false
	}
	_, ok := heapowner.ResolveRuleImplementation(rule.implementation)
	return rule.implementation, ok
}

// SealProgramRule is this typed rule's schema registration.
func SealProgramRule(rule *HotRule) (engine.ProgramRule, bool) {
	if rule == nil {
		return engine.ProgramRule{}, false
	}
	implementation, ok := heapowner.ResolveRuleImplementationFor(rule.owner, rule.implementation)
	if !ok {
		return engine.ProgramRule{}, false
	}
	return engine.SealProgramRule(implementation)
}

func emptyContent(schema heapdomain.Schema, operand source.Root) (source.Root, [32]byte, bool) {
	id, ok := operand.ID()
	if !ok || operand.Form() != source.FormEmpty || !operand.FencedTo(schema) {
		return source.Root{}, [32]byte{}, false
	}
	return operand, [32]byte(id), true
}

func emptyResult(schema heapdomain.Schema, operand source.Root, predecessor heapdomain.Value) (heapdomain.Key, heapdomain.Value, bool) {
	if !schema.Valid() || operand.Form() != source.FormEmpty || !operand.FencedTo(schema) || predecessor.IsBottom() {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	shape := heapdomain.ShapeIneligible
	if operand.Kind() == heapdomain.AllocationTable {
		shape = heapdomain.ShapeEligible
	}
	none, noneOK := schema.ContainmentNone()
	initializer, initOK := schema.BeginObject(shape, heapdomain.FrozenMutable, none)
	fresh, freshOK := initializer.Finish()
	if !noneOK || !initOK || !freshOK {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	next, nextOK := schema.Create(predecessor, operand.Key(), fresh)
	return operand.Key(), next, nextOK
}
