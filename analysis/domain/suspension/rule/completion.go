package rule

import (
	"github.com/wippyai/go-lua/analysis/domain/suspension"
	suspensionowner "github.com/wippyai/go-lua/analysis/domain/suspension/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
)

// CompletionOperand is one successful ModuleInit completion. Link's existing
// ReadySubject projection proves that the outcome is Normal or Return and
// fixes the exact generation. The subject itself remains Module's cache
// publication input; Suspension retains no second ready-result relation.
type CompletionOperand struct {
	source        *link.Link
	outcome       linkmodule.ModuleInitOutcome
	suspensionKey suspension.Key
	content       keyspace.ContentID
}

// NewCompletionOperand derives the selected Suspension generation directly
// from one Link-owned successful outcome. A bare outcome kind, cache key, or
// module subject cannot manufacture this lifecycle completion.
func NewCompletionOperand(source *link.Link, generations suspension.Schema, outcome linkmodule.ModuleInitOutcome) (CompletionOperand, bool) {
	if source == nil || !generations.Valid() {
		return CompletionOperand{}, false
	}
	generation, _, _, provenanceOK := source.Module().Outcomes().Provenance(outcome)
	_, readyOK := source.Module().Outcomes().ReadySubject(outcome)
	content, contentOK := source.Module().Outcomes().ID(outcome)
	suspensionKey, suspensionOK := generations.KeyForModuleInitGeneration(generation)
	keyLink, keyLinkOK := suspensionKey.LinkContentID()
	if !provenanceOK || !readyOK || !contentOK || !content.Available() || !suspensionOK || !keyLinkOK || keyLink != source.ContentID() {
		return CompletionOperand{}, false
	}
	return CompletionOperand{source: source, outcome: outcome, suspensionKey: suspensionKey, content: content}, true
}

// OutcomeID is Link's existing exact successful completion identity.
func (operand CompletionOperand) OutcomeID() keyspace.ContentID { return operand.content }

// CompletionDeclaration consumes a successful ModuleInit generation. It has
// one Suspension input/output at the same exact generation and neither reads
// nor writes Module state; Module's PublishReady independently observes the
// matching live premise.
type CompletionDeclaration struct {
	semantic   engine.SemanticKey
	rule       *engine.Rule[suspension.Value, CompletionOperand]
	suspension *suspensionowner.Owner
	read       engine.Read[engine.OrderedCells[suspension.Value]]
	write      engine.Write[suspension.Value]
}

// DeclareCompletion records the Suspension half of a successful module-init
// completion. The terminal reduction is deliberately shared with Cancel:
// lifecycle termination depends on the exact selected generation, not on the
// cache publication subject or on evaluator scheduling.
func DeclareCompletion(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, generations *suspensionowner.Owner) (*CompletionDeclaration, bool) {
	if composition == nil || generations == nil || !semantic.Available() || !operandFamily.Available() || !evidence.Available() ||
		semantic == operandFamily || semantic == evidence || operandFamily == evidence {
		return nil, false
	}
	declaration := &CompletionDeclaration{semantic: semantic, suspension: generations}
	rule, ok := engine.DeclareRule(composition, engine.RuleSpec[suspension.Value, CompletionOperand]{
		Semantic:       semantic,
		OperandFamily:  operandFamily,
		OperandContent: completionOperandContent,
		Output:         generations.Output(),
		Inputs:         1,
		Admission:      engine.AdmitRuleByDerivation(evidence, declaration.check),
		Transfer:       declaration.transfer,
	}, func(rule *engine.Rule[suspension.Value, CompletionOperand]) bool {
		input, inputOK := rule.InputAt(0)
		read, readOK := engine.ReadFrom(rule, input, generations.Read())
		write, writeOK := engine.WriteTo(rule, generations.Write())
		if !inputOK || !readOK || !writeOK {
			return false
		}
		declaration.rule, declaration.read, declaration.write = rule, read, write
		return true
	})
	if !ok || rule == nil || declaration.rule != rule {
		return nil, false
	}
	return declaration, true
}

// NewInstance binds both ends only to Link's selected generation.
func (declaration *CompletionDeclaration) NewInstance(operand CompletionOperand) (*engine.RuleInstance[suspension.Value, CompletionOperand], bool) {
	if declaration == nil || declaration.rule == nil || declaration.suspension == nil || !validCompletionOperand(operand, declaration.suspension.Schema()) {
		return nil, false
	}
	return engine.NewRuleInstance(declaration.rule, operand, func(binding *engine.RuleBinding[suspension.Value, CompletionOperand]) bool {
		ref, ok := declaration.suspension.Locate(operand.suspensionKey)
		return ok && engine.InstanceRead(binding, declaration.read, ref) && engine.InstanceWrite(binding, declaration.write, ref)
	})
}

func completionOperandContent(operand CompletionOperand) (CompletionOperand, [32]byte, bool) {
	if !operand.content.Available() || !operand.suspensionKey.Valid() {
		return CompletionOperand{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.content), true
}

func validCompletionOperand(operand CompletionOperand, generations suspension.Schema) bool {
	if operand.source == nil || !operand.content.Available() || !operand.suspensionKey.Valid() || !generations.Valid() {
		return false
	}
	keyLink, keyLinkOK := operand.suspensionKey.LinkContentID()
	keyGeneration, keyGenerationOK := operand.suspensionKey.ModuleInitGeneration()
	generation, _, _, provenanceOK := operand.source.Module().Outcomes().Provenance(operand.outcome)
	_, readyOK := operand.source.Module().Outcomes().ReadySubject(operand.outcome)
	content, contentOK := operand.source.Module().Outcomes().ID(operand.outcome)
	return keyLinkOK && keyGenerationOK && provenanceOK && readyOK && contentOK && keyLink == operand.source.ContentID() &&
		keyGeneration == generation && content == operand.content
}

func (declaration *CompletionDeclaration) transfer(access engine.Access[suspension.Value, CompletionOperand]) bool {
	operand, operandOK := engine.Operand(access)
	if !operandOK || declaration == nil || declaration.suspension == nil || !validCompletionOperand(operand, declaration.suspension.Schema()) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		cells, readOK := engine.ReadValue(access, row, declaration.read)
		if !readOK || cells.Count() != 1 {
			return false
		}
		current, present, cellOK := cells.At(0)
		if !cellOK {
			return false
		}
		next, consumed := consumeLiveCompletion(declaration.suspension.Schema(), operand.suspensionKey, current, present)
		if !consumed {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, next)
	})
}

func (declaration *CompletionDeclaration) check(derivation engine.RuleDerivation[suspension.Value, CompletionOperand]) (engine.RuleEvidence, bool) {
	if declaration == nil || declaration.suspension == nil || !declaration.matchesSemantic(derivation.Rule()) ||
		derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	input, inputOK := derivation.InputAt(0)
	operand, operandOK := derivation.Operand()
	if !inputOK || input.Guard().Empty() || !operandOK || !validCompletionOperand(operand, declaration.suspension.Schema()) || !derivation.OperandContentMatches([32]byte(operand.content)) {
		return engine.RuleEvidence{}, false
	}
	ref, refOK := declaration.suspension.Locate(operand.suspensionKey)
	if !refOK || !engine.DerivationReadMatchesRef(derivation, declaration.read, ref) {
		return engine.RuleEvidence{}, false
	}
	disposition, dispositionOK := derivation.DispositionAt(0)
	cells, cellsOK := engine.DerivationDispositionReadValue(derivation, disposition, declaration.read)
	if !dispositionOK || !cellsOK || cells.Count() != 1 || !disposition.Guard().Same(input.Guard()) {
		return engine.RuleEvidence{}, false
	}
	current, present, cellOK := cells.At(0)
	if !cellOK {
		return engine.RuleEvidence{}, false
	}
	next, consumed := consumeLiveCompletion(declaration.suspension.Schema(), operand.suspensionKey, current, present)
	if !consumed {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	value, staged := disposition.Value()
	target, targetOK := disposition.TargetAt(0)
	if disposition.Kind() != engine.RuleDispositionStaged || !staged || disposition.TargetCount() != 1 || !targetOK ||
		!declaration.suspension.Schema().Admits(operand.suspensionKey, value) || !declaration.suspension.Schema().Equal(value, next) || !engine.TargetMatchesRef(target, ref) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}

func (declaration *CompletionDeclaration) matchesSemantic(semantic engine.SemanticKey) bool {
	return declaration != nil && declaration.semantic.Available() && declaration.semantic == semantic
}
