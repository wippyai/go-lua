// Package source declares Pack's authored structural source Rule.
//
// Pack owns its source grammar: a source occurrence is either a closed list
// of exact scalar endpoints or the same finite prefix followed by one
// owner-issued free Pack tail.  This package only transports that complete
// domain value to the exact occurrence root.  It neither reads Value facts nor
// reconstructs Program, Link, or Target identities.
package source

import (
	packdomain "github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// Rule is the sole zero-read source authority for an authored Pack Source.
// Its raw engine Rule and write capability remain private so a caller cannot
// bind an arbitrary Pack root independently of the sealed source descriptor.
type Rule struct {
	rule  *engine.Rule[packdomain.Value, packdomain.Source]
	write engine.Write[packdomain.Value]
	owner *packowner.Owner
}

// Declare records one complete Pack-source Rule.  Every identity is supplied
// by the composition root: this package has no local registry, fallback rule,
// or secondary source vocabulary.
func Declare(
	composition *engine.Composition,
	ruleSemantic engine.SemanticKey,
	operandFamily engine.SemanticKey,
	evidenceSemantic engine.SemanticKey,
	owner *packowner.Owner,
) (*Rule, bool) {
	if composition == nil || owner == nil || owner.Schema() == nil ||
		!ruleSemantic.Available() || !operandFamily.Available() || !evidenceSemantic.Available() ||
		ruleSemantic == operandFamily || ruleSemantic == evidenceSemantic || operandFamily == evidenceSemantic {
		return nil, false
	}

	var write engine.Write[packdomain.Value]
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[packdomain.Value, packdomain.Source]{
		Semantic:       ruleSemantic,
		OperandFamily:  operandFamily,
		OperandContent: sourceContent,
		Output:         owner.Output(),
		Inputs:         0,
		Admission:      engine.AdmitRuleByDerivation(evidenceSemantic, checker(owner, ruleSemantic)),
		Transfer: func(access engine.Access[packdomain.Value, packdomain.Source]) bool {
			source, ok := engine.Operand(access)
			if !ok {
				return false
			}
			_, fact, ok := result(owner, source)
			if !ok {
				return false
			}
			rows := 0
			complete := engine.Product(access, func(row engine.Row) bool {
				rows++
				return rows == 1 && engine.StageValue(access, row, fact)
			})
			return complete && rows == 1
		},
	}, func(rule *engine.Rule[packdomain.Value, packdomain.Source]) bool {
		var written bool
		write, written = engine.WriteTo(rule, owner.ExactWrite())
		return written
	})
	if !ok || declared == nil {
		return nil, false
	}
	return &Rule{rule: declared, write: write, owner: owner}, true
}

// Instance binds both the operand and the exact Pack root from the same
// immutable Source.  In particular, roots for two calls in the same Program
// body cannot be swapped or merged at this boundary.
func (rule *Rule) Instance(source packdomain.Source) (*engine.RuleInstance[packdomain.Value, packdomain.Source], bool) {
	if rule == nil || rule.rule == nil || rule.owner == nil {
		return nil, false
	}
	root, _, ok := result(rule.owner, source)
	if !ok {
		return nil, false
	}
	ref, ok := rule.owner.Locate(root)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, source, func(binding *engine.RuleBinding[packdomain.Value, packdomain.Source]) bool {
		return engine.InstanceWrite(binding, rule.write, ref)
	})
}

func sourceContent(source packdomain.Source) (packdomain.Source, [32]byte, bool) {
	id, ok := source.ContentID()
	return source, [32]byte(id), ok && [32]byte(id) != [32]byte{}
}

// result is shared by transfer and the evidence checker.  The owner first
// reissues the source from its own Schema; a same-content Source from another
// sealed Schema cannot cross the factor boundary.
func result(owner *packowner.Owner, source packdomain.Source) (packdomain.Root, packdomain.Value, bool) {
	if owner == nil || owner.Schema() == nil {
		return packdomain.Root{}, packdomain.Value{}, false
	}
	root, ok := source.Root()
	if !ok {
		return packdomain.Root{}, packdomain.Value{}, false
	}
	authorized, ok := owner.Schema().Source(root)
	if !ok || authorized != source || source.Count() != 1 {
		return packdomain.Root{}, packdomain.Value{}, false
	}
	item, ok := source.At(0)
	if !ok {
		return packdomain.Root{}, packdomain.Value{}, false
	}
	builder, ok := owner.Schema().Builder(root)
	if !ok {
		return packdomain.Root{}, packdomain.Value{}, false
	}
	term, ok := sourceTerm(builder, item)
	if !ok {
		return packdomain.Root{}, packdomain.Value{}, false
	}
	port, ok := item.Port()
	if !ok {
		return packdomain.Root{}, packdomain.Value{}, false
	}
	equation, ok := builder.Pack(port, term)
	if !ok {
		return packdomain.Root{}, packdomain.Value{}, false
	}
	caseValue, ok := builder.Case(equation)
	if !ok {
		return packdomain.Root{}, packdomain.Value{}, false
	}
	fact, ok := builder.Value(caseValue)
	if !ok || !owner.Schema().Admit(root, fact) {
		return packdomain.Root{}, packdomain.Value{}, false
	}
	return root, fact, true
}

func sourceTerm(builder packdomain.Builder, item packdomain.SourceItem) (packdomain.Term, bool) {
	fixed := make([]packdomain.Scalar, item.FixedCount())
	for index := range fixed {
		endpoint, ok := item.FixedAt(index)
		if !ok {
			return packdomain.Term{}, false
		}
		fixed[index], ok = builder.Endpoint(endpoint)
		if !ok {
			return packdomain.Term{}, false
		}
	}
	tail, offset, open := item.Tail()
	if !open {
		return builder.Closed(fixed...)
	}
	free, ok := builder.FreeTail(tail)
	if !ok {
		return packdomain.Term{}, false
	}
	rest, ok := builder.Tail(free, offset)
	if !ok {
		return packdomain.Term{}, false
	}
	return builder.Open(fixed, rest, nil)
}

func checker(owner *packowner.Owner, semantic engine.SemanticKey) engine.RuleDerivationChecker[packdomain.Value, packdomain.Source] {
	return func(derivation engine.RuleDerivation[packdomain.Value, packdomain.Source]) (engine.RuleEvidence, bool) {
		if owner == nil || derivation.Rule() != semantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
			return engine.RuleEvidence{}, false
		}
		source, ok := derivation.Operand()
		id, idOK := source.ContentID()
		if !ok || !idOK || !derivation.OperandContentMatches([32]byte(id)) {
			return engine.RuleEvidence{}, false
		}
		root, expected, resultOK := result(owner, source)
		ref, refOK := owner.Locate(root)
		disposition, dispositionOK := derivation.DispositionAt(0)
		if !resultOK || !refOK || !dispositionOK || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || disposition.Guard().Empty() {
			return engine.RuleEvidence{}, false
		}
		target, targetOK := disposition.TargetAt(0)
		actual, actualOK := disposition.Value()
		if !targetOK || !actualOK || !engine.TargetMatchesRef(target, ref) || !owner.Schema().Lattice().Equal(actual, expected) {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
}
