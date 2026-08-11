// Package ingress owns the sole zero-input Heap allocation transition. It
// admits an existing Program allocation root as WorldZero; it never invents a
// published object or a fresh Target root.
package ingress

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Rule declares WorldZero ingress. Its only output is the exact Heap root;
// construction rules must consume that fact rather than infer it from absence.
type Rule struct {
	rule     *engine.Rule[heapdomain.Value, source.Root]
	write    engine.Write[heapdomain.Value]
	owner    *heapowner.Owner
	semantic engine.SemanticKey
}

func Declare(composition *engine.Composition, semantic, family, evidence engine.SemanticKey, owner *heapowner.Owner) (*Rule, bool) {
	if composition == nil || owner == nil || !owner.Schema().ContentID().Available() || !semantic.Available() || !family.Available() || !evidence.Available() || semantic == family || semantic == evidence || family == evidence {
		return nil, false
	}
	decl := &Rule{owner: owner, semantic: semantic}
	rule, ok := engine.DeclareRule(composition, engine.RuleSpec[heapdomain.Value, source.Root]{
		Semantic: semantic, OperandFamily: family, OperandContent: decl.content,
		Output: owner.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByDerivation(evidence, decl.check), Transfer: decl.transfer,
	}, func(rule *engine.Rule[heapdomain.Value, source.Root]) bool {
		write, writeOK := engine.WriteTo(rule, owner.ExactWrite())
		decl.rule, decl.write = rule, write
		return writeOK
	})
	if !ok || rule == nil || decl.rule != rule {
		return nil, false
	}
	return decl, true
}

func (rule *Rule) valid(operand source.Root) (keyspace.ContentID, bool) {
	if rule == nil || rule.owner == nil || !operand.FencedTo(rule.owner.Schema()) {
		return keyspace.ContentID{}, false
	}
	return operand.ID()
}

// admit is the cold structural admission check. Execution and result use the
// exact owner fence above; they must not rebuild Root topology per recurrence
// row after an Instance has already established this binding.
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

// Instance binds one existing Heap allocation Key. The private source.Root
// descriptor is constructed only by this owning Rule, so no caller can pair a
// root with a Heap key, event, or constructor form from another schema.
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
		return engine.InstanceWrite(binding, rule.write, ref)
	})
}

func (rule *Rule) result(operand source.Root) (heapdomain.Key, heapdomain.Value, bool) {
	if _, ok := rule.valid(operand); !ok {
		return heapdomain.Key{}, heapdomain.Value{}, false
	}
	value, ok := rule.owner.Schema().EmptyObject(operand.Key())
	return operand.Key(), value, ok
}

func (rule *Rule) transfer(access engine.Access[heapdomain.Value, source.Root]) bool {
	operand, ok := engine.Operand(access)
	if !ok {
		return false
	}
	_, value, resultOK := rule.result(operand)
	rows := 0
	completed := engine.Product(access, func(row engine.Row) bool {
		rows++
		return rows == 1 && resultOK && engine.StageValue(access, row, value)
	})
	return completed && rows == 1
}

func (rule *Rule) check(derivation engine.RuleDerivation[heapdomain.Value, source.Root]) (engine.RuleEvidence, bool) {
	if rule == nil || rule.owner == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, operandOK := derivation.Operand()
	id, idOK := rule.admit(operand)
	target, want, wantOK := rule.result(operand)
	ref, refOK := rule.owner.Locate(target)
	disposition, dispositionOK := derivation.DispositionAt(0)
	actual, actualOK := disposition.Value()
	targetFact, targetOK := disposition.TargetAt(0)
	if !operandOK || !idOK || !wantOK || !refOK || !derivation.OperandContentMatches([32]byte(id)) || !dispositionOK || disposition.Guard().Empty() || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || !actualOK || !targetOK || !engine.TargetMatchesRef(targetFact, ref) || !rule.owner.Schema().Domain().Equal(actual, want) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}
