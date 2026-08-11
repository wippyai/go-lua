// Package empty owns principal construction for closures and zero-field
// tables. It consumes explicit WorldZero ingress and publishes one complete
// object through Heap.Create; it never reconstructs an object from fact
// absence.
package empty

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
)

type Rule struct {
	rule      *engine.Rule[heapdomain.Value, source.Root]
	read      engine.Read[engine.OrderedCells[heapdomain.Value]]
	write     engine.Write[heapdomain.Value]
	owner     *heapowner.Owner
	semantic  engine.SemanticKey
	transform engine.SemanticKey
}

func Declare(composition *engine.Composition, semantic, family, transform, evidence engine.SemanticKey, owner *heapowner.Owner) (*Rule, bool) {
	if composition == nil || owner == nil || !owner.Schema().ContentID().Available() || !distinct(semantic, family, transform, evidence) {
		return nil, false
	}
	decl := &Rule{owner: owner, semantic: semantic, transform: transform}
	rule, ok := engine.DeclareRule(composition, engine.RuleSpec[heapdomain.Value, source.Root]{
		Semantic: semantic, OperandFamily: family, OperandContent: decl.content, Output: owner.Output(), Inputs: 1,
		Admission: engine.AdmitRuleByDerivation(evidence, decl.check), Transfer: decl.transfer,
	}, func(rule *engine.Rule[heapdomain.Value, source.Root]) bool {
		input, inputOK := rule.InputAt(0)
		if !inputOK {
			return false
		}
		read, readOK := engine.ReadFrom(rule, input, owner.ExactRead())
		write, writeOK := engine.WriteTo(rule, owner.ExactWrite())
		if !readOK || !writeOK || !engine.TransformCarryFrom(rule, input, owner.Carry(), transform, func(operand source.Root, predecessor heapdomain.Value) (heapdomain.Value, bool) {
			return owner.Schema().Age(predecessor, operand.Key())
		}) {
			return false
		}
		decl.rule, decl.read, decl.write = rule, read, write
		return true
	})
	if !ok || rule == nil || decl.rule != rule {
		return nil, false
	}
	return decl, true
}

func (rule *Rule) valid(operand source.Root) (keyspace.ContentID, bool) {
	if rule == nil || rule.owner == nil || operand.Form() != source.FormEmpty || !operand.FencedTo(rule.owner.Schema()) {
		return keyspace.ContentID{}, false
	}
	return operand.ID()
}

// admit is the cold structural admission check. Once a Root has bound this
// Rule, recurrent transfer needs only valid's exact schema fence; rebuilding
// allocation topology per Product row changes work, not semantic authority.
func (rule *Rule) admit(operand source.Root) (keyspace.ContentID, bool) {
	id, ok := rule.valid(operand)
	if !ok || !operand.Revalidate(rule.owner.Schema()) {
		return keyspace.ContentID{}, false
	}
	return id, true
}

func (rule *Rule) content(operand source.Root) (source.Root, [32]byte, bool) {
	id, ok := rule.admit(operand)
	return operand, [32]byte(id), ok
}

// Instance binds one existing Heap allocation Key. The exact private source
// descriptor is rebuilt under this Rule's Heap owner rather than crossing the
// allocation package boundary as a second operand vocabulary.
func (rule *Rule) Instance(root heapdomain.Key) (*engine.RuleInstance[heapdomain.Value, source.Root], bool) {
	if rule == nil || rule.owner == nil {
		return nil, false
	}
	operand, ok := source.New(rule.owner.Schema(), root)
	if !ok {
		return nil, false
	}
	return rule.instance(operand)
}

func (rule *Rule) instance(operand source.Root) (*engine.RuleInstance[heapdomain.Value, source.Root], bool) {
	if rule == nil || rule.rule == nil {
		return nil, false
	}
	if _, ok := rule.admit(operand); !ok {
		return nil, false
	}
	ref, ok := rule.owner.Locate(operand.Key())
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[heapdomain.Value, source.Root]) bool {
		return engine.InstanceRead(binding, rule.read, ref) && engine.InstanceWrite(binding, rule.write, ref)
	})
}

// result is the sole empty-object judgment shared by execution and evidence.
func (rule *Rule) result(operand source.Root, predecessor heapdomain.Value) (heapdomain.Key, heapdomain.Value, bool) {
	if _, ok := rule.valid(operand); !ok || predecessor.IsBottom() {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	shape := heapdomain.ShapeIneligible
	if operand.Kind() == heapdomain.AllocationTable {
		shape = heapdomain.ShapeEligible
	}
	none, noneOK := rule.owner.Schema().ContainmentNone()
	initializer, initOK := rule.owner.Schema().BeginObject(shape, heapdomain.FrozenMutable, none)
	fresh, freshOK := initializer.Finish()
	if !noneOK || !initOK || !freshOK {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	next, nextOK := rule.owner.Schema().Create(predecessor, operand.Key(), fresh)
	return operand.Key(), next, nextOK
}

func (rule *Rule) transfer(access engine.Access[heapdomain.Value, source.Root]) bool {
	operand, operandOK := engine.Operand(access)
	if !operandOK {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		cells, cellsOK := engine.ReadValue(access, row, rule.read)
		if !cellsOK || cells.Count() != 1 {
			return false
		}
		predecessor, present, available := cells.At(0)
		if !available {
			return false
		}
		if !present {
			return engine.NoCandidate(access, row)
		}
		_, next, nextOK := rule.result(operand, predecessor)
		return nextOK && engine.StageValue(access, row, next)
	})
}

func (rule *Rule) check(derivation engine.RuleDerivation[heapdomain.Value, source.Root]) (engine.RuleEvidence, bool) {
	if rule == nil || rule.owner == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	id, idOK := rule.admit(operand)
	ref, refOK := rule.owner.Locate(operand.Key())
	if !operandOK || !idOK || !refOK || !derivation.OperandContentMatches([32]byte(id)) || !engine.DerivationReadMatchesRef(derivation, rule.read, ref) {
		return engine.RuleEvidence{}, false
	}
	input, inputOK := derivation.InputAt(0)
	if !inputOK {
		return engine.RuleEvidence{}, false
	}
	if input.Guard().Empty() {
		return engine.RuleEvidence{}, false
	}
	for index := 0; index < derivation.DispositionCount(); index++ {
		disposition, dispositionOK := derivation.DispositionAt(index)
		if !dispositionOK || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, rule.read)
		if !cellsOK || cells.Count() != 1 {
			return engine.RuleEvidence{}, false
		}
		predecessor, present, available := cells.At(0)
		if !available {
			return engine.RuleEvidence{}, false
		}
		if !present {
			_, transformed := disposition.CarryTransform()
			if disposition.Kind() != engine.RuleDispositionNoCandidate || transformed || disposition.TransformOnly() || disposition.TargetCount() != 0 {
				return engine.RuleEvidence{}, false
			}
			continue
		}
		target, next, nextOK := rule.result(operand, predecessor)
		targetRef, targetOK := disposition.TargetAt(0)
		actual, actualOK := disposition.Value()
		transform, transformed := disposition.CarryTransform()
		if !nextOK || !targetOK || !actualOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TransformOnly() || !transformed || transform != rule.transform || disposition.TargetCount() != 1 || !engine.TargetMatchesRef(targetRef, ref) || target != operand.Key() || !rule.owner.Schema().Domain().Equal(actual, next) {
			return engine.RuleEvidence{}, false
		}
	}
	return derivation.Accept()
}

func distinct(keys ...engine.SemanticKey) bool {
	for index, key := range keys {
		if !key.Available() {
			return false
		}
		for prior := 0; prior < index; prior++ {
			if key == keys[prior] {
				return false
			}
		}
	}
	return true
}
