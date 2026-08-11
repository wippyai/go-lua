// Package bootstrap declares Heap's Target/Link bootstrap raw-presence seed.
// It owns neither a second heap image nor an initial-state composition root.
package bootstrap

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Root is one sealed complete bootstrap image for one actor-local BootRoot.
// Entries are deliberately aggregated before execution: a table's entries
// coexist within its one WorldExact object and must never be emitted as
// sibling whole-world alternatives for Factor Join to reconcile.
type Root struct {
	key     heapdomain.Key
	entries []heapdomain.BootEntry
	id      keyspace.ContentID
}

// NewRoot aggregates every canonical BootEntry for key in a deterministic
// semantic order. It is the only bootstrap operand constructor.
func NewRoot(schema heapdomain.Schema, key heapdomain.Key) (Root, bool) {
	if !schema.ContentID().Available() || key.Kind() != heapdomain.RootBoot {
		return Root{}, false
	}
	boot, bootOK := key.BootRoot()
	if !bootOK {
		return Root{}, false
	}
	link := schema.Link()
	if link == nil || link.Host() == nil {
		return Root{}, false
	}
	bootID, bootIDOK := link.Host().BootRoots().ID(boot)
	if !bootIDOK {
		return Root{}, false
	}
	entries := make([]heapdomain.BootEntry, 0)
	for index := 0; index < schema.BootEntryCount(); index++ {
		entry, entryOK := schema.BootEntryAt(index)
		entryKey, keyOK := entry.Key()
		if !entryOK || !keyOK {
			return Root{}, false
		}
		if entryKey == key {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(left, right int) bool {
		leftID, _ := entries[left].ID()
		rightID, _ := entries[right].ID()
		return compareID(leftID, rightID) < 0
	})
	for index, entry := range entries {
		id, ok := entry.ID()
		if !ok {
			return Root{}, false
		}
		if index != 0 {
			previous, previousOK := entries[index-1].ID()
			if !previousOK || compareID(previous, id) >= 0 {
				return Root{}, false
			}
		}
	}
	id := rootID(schema.ContentID(), bootID)
	if !id.Available() {
		return Root{}, false
	}
	return Root{key: key, entries: entries, id: id}, true
}

func (root Root) ID() (keyspace.ContentID, bool) { return root.id, root.id.Available() }

func compareID(left, right keyspace.ContentID) int {
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}

func rootID(schemaID, bootID keyspace.ContentID) keyspace.ContentID {
	if !schemaID.Available() || !bootID.Available() {
		return keyspace.ContentID{}
	}
	var image [32 + 32 + 16]byte
	copy(image[:32], schemaID[:])
	copy(image[32:64], bootID[:])
	binary.BigEndian.PutUint64(image[64:72], 0x686561702d626f6f) // "heap-boo"
	binary.BigEndian.PutUint64(image[72:80], 3)
	return sha256.Sum256(image[:])
}

// Rule writes exactly one complete WorldExact image for a sealed actor-local
// BootRoot.
type Rule struct {
	rule  *engine.Rule[heapdomain.Value, Root]
	write engine.Write[heapdomain.Value]
	owner *heapowner.Owner
}

// Declare records the zero-read bootstrap law.  Later body compilation binds
// only entries from Schema.BootEntryAt; no caller can pair a raw slot with an
// arbitrary Heap root or payload.
func Declare(composition *engine.Composition, ruleSemantic, operandFamily, evidenceSemantic engine.SemanticKey, owner *heapowner.Owner) (*Rule, bool) {
	if composition == nil || owner == nil || !owner.Schema().ContentID().Available() || !ruleSemantic.Available() || !operandFamily.Available() || !evidenceSemantic.Available() ||
		ruleSemantic == operandFamily || ruleSemantic == evidenceSemantic || operandFamily == evidenceSemantic {
		return nil, false
	}
	var write engine.Write[heapdomain.Value]
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[heapdomain.Value, Root]{
		Semantic: ruleSemantic, OperandFamily: operandFamily, OperandContent: content, Output: owner.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByDerivation(evidenceSemantic, checker(owner, ruleSemantic)),
		Transfer: func(access engine.Access[heapdomain.Value, Root]) bool {
			root, ok := engine.Operand(access)
			if !ok {
				return false
			}
			_, value, ok := result(owner, root)
			if !ok {
				return false
			}
			count := 0
			return engine.Product(access, func(row engine.Row) bool {
				count++
				return count == 1 && engine.StageValue(access, row, value)
			}) && count == 1
		},
	}, func(rule *engine.Rule[heapdomain.Value, Root]) bool {
		var ok bool
		write, ok = engine.WriteTo(rule, owner.ExactWrite())
		return ok
	})
	if !ok || declared == nil {
		return nil, false
	}
	return &Rule{rule: declared, write: write, owner: owner}, true
}

func (rule *Rule) Instance(root Root) (*engine.RuleInstance[heapdomain.Value, Root], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil {
		return nil, false
	}
	key, _, ok := result(rule.owner, root)
	if !ok {
		return nil, false
	}
	ref, ok := rule.owner.Locate(key)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, root, func(binding *engine.RuleBinding[heapdomain.Value, Root]) bool {
		return engine.InstanceWrite(binding, rule.write, ref)
	})
}

func content(root Root) (Root, [32]byte, bool) {
	id, ok := root.ID()
	return root, [32]byte(id), ok
}

func result(owner *heapowner.Owner, root Root) (heapdomain.Key, heapdomain.Value, bool) {
	if owner == nil {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	schema := owner.Schema()
	canonical, canonicalOK := NewRoot(schema, root.key)
	if !canonicalOK || !sameRoot(root, canonical) {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	none, noneOK := schema.ContainmentNone()
	frozen, frozenOK := schema.BootFrozen(root.key)
	initializer, initializerOK := schema.BeginObject(heapdomain.ShapeEligible, frozen, none)
	if !initializerOK || !noneOK || !frozenOK {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	for _, entry := range root.entries {
		slot, slotOK := entry.Slot()
		raw, payload, projectionOK := entry.Projection()
		selector, selectorOK := schema.SelectorForSlot(slot)
		if !slotOK || !projectionOK || !selectorOK {
			return heapdomain.Key{}, heapdomain.Value{}, false
		}
		var state heapdomain.CellState
		var stateOK bool
		switch raw {
		case heapdomain.RawAbsent:
			state, stateOK = schema.CellAbsent()
		case heapdomain.RawPresent:
			valueChild, childOK := entry.ValueContainment()
			if !childOK {
				return heapdomain.Key{}, heapdomain.Value{}, false
			}
			state, stateOK = schema.CellPresent(slot, payload, valueChild, none)
		default:
			return heapdomain.Key{}, heapdomain.Value{}, false
		}
		if !stateOK || !initializer.Apply(selector, state) {
			return heapdomain.Key{}, heapdomain.Value{}, false
		}
	}
	object, objectOK := initializer.Finish()
	world, worldOK := schema.Exact(root.key, object)
	value, relationOK := schema.Relation(root.key, world)
	if !objectOK || !worldOK || !relationOK {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	return root.key, value, schema.Admits(root.key, value)
}

func sameRoot(left, right Root) bool {
	if left.key != right.key || left.id != right.id || len(left.entries) != len(right.entries) {
		return false
	}
	for index := range left.entries {
		leftID, leftOK := left.entries[index].ID()
		rightID, rightOK := right.entries[index].ID()
		if !leftOK || !rightOK || leftID != rightID {
			return false
		}
	}
	return true
}

func checker(owner *heapowner.Owner, semantic engine.SemanticKey) engine.RuleDerivationChecker[heapdomain.Value, Root] {
	return func(derivation engine.RuleDerivation[heapdomain.Value, Root]) (engine.RuleEvidence, bool) {
		if owner == nil || derivation.Rule() != semantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		root, ok := derivation.Operand()
		id, idOK := root.ID()
		if !ok || !idOK || !derivation.OperandContentMatches([32]byte(id)) {
			return engine.RuleEvidence{}, false
		}
		key, expected, resultOK := result(owner, root)
		ref, refOK := owner.Locate(key)
		disposition, dispositionOK := derivation.DispositionAt(0)
		if !resultOK || !refOK || !dispositionOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		target, targetOK := disposition.TargetAt(0)
		actual, actualOK := disposition.Value()
		if !targetOK || !actualOK || !engine.TargetMatchesRef(target, ref) || !owner.Schema().Domain().Equal(actual, expected) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
