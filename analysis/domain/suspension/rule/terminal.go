package rule

import (
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/suspension"
	suspensionowner "github.com/wippyai/go-lua/analysis/domain/suspension/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
)

// CancelOperand is one exact canceled module-initialization completion.  It
// carries no Module coordinate: Link derives its generation and Suspension is
// the only owner that can consume that generation's lifecycle.
type CancelOperand struct {
	source        *link.Link
	terminal      linkmodule.ModuleInitTerminal
	suspensionKey suspension.Key
	content       keyspace.ContentID
}

// NewCancelOperand accepts only Link's exact Cancel terminal subset.  A
// terminal outcome fixes the ModuleInitGeneration; callers cannot supply a
// loose continuation key or manufacture a terminal-to-generation relation.
func NewCancelOperand(source *link.Link, generations suspension.Schema, terminal linkmodule.ModuleInitTerminal) (CancelOperand, bool) {
	if source == nil || !generations.Valid() {
		return CancelOperand{}, false
	}
	_, generation, _, kind, provenanceOK := source.Module().Terminals().Provenance(terminal)
	content, contentOK := source.Module().Terminals().ID(terminal)
	suspensionKey, suspensionOK := generations.KeyForModuleInitGeneration(generation)
	keyLink, keyLinkOK := suspensionKey.LinkContentID()
	if !provenanceOK || kind != flowkind.OutcomeCancel || !contentOK || !content.Available() || !suspensionOK || !keyLinkOK || keyLink != source.ContentID() {
		return CancelOperand{}, false
	}
	return CancelOperand{source: source, terminal: terminal, suspensionKey: suspensionKey, content: content}, true
}

// TerminalID identifies the sealed Link correspondence being consumed.  It
// is evidence of a particular canceled completion, not another lifecycle key.
func (operand CancelOperand) TerminalID() keyspace.ContentID { return operand.content }

// CancelDeclaration is Suspension's one-read, same-generation cancellation
// transition.  It owns no Module cache observation or output.
type CancelDeclaration struct {
	semantic   engine.SemanticKey
	rule       *engine.Rule[suspension.Value, CancelOperand]
	suspension *suspensionowner.Owner
	read       engine.Read[engine.OrderedCells[suspension.Value]]
	write      engine.Write[suspension.Value]
}

// DeclareCancel consumes the exact recent live lifecycle produced by the
// sibling ModuleInit Rule when Link observes a Cancel completion.  Throw has
// no Suspension transition: Module's terminal rule deliberately observes its
// still-live lifecycle.  This separation is the semantic premise consumed by
// Module's RestoreCold rule, not a scheduling convention.
func DeclareCancel(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, generations *suspensionowner.Owner) (*CancelDeclaration, bool) {
	if composition == nil || generations == nil || !semantic.Available() || !operandFamily.Available() || !evidence.Available() ||
		semantic == operandFamily || semantic == evidence || operandFamily == evidence {
		return nil, false
	}
	declaration := &CancelDeclaration{semantic: semantic, suspension: generations}
	rule, ok := engine.DeclareRule(composition, engine.RuleSpec[suspension.Value, CancelOperand]{
		Semantic:       semantic,
		OperandFamily:  operandFamily,
		OperandContent: cancelOperandContent,
		Output:         generations.Output(),
		Inputs:         1,
		Admission:      engine.AdmitRuleByDerivation(evidence, declaration.check),
		Transfer:       declaration.transfer,
	}, func(rule *engine.Rule[suspension.Value, CancelOperand]) bool {
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

// NewInstance binds the sole input and output to the very same generation
// that Link selected.  It cannot bind another Suspension occurrence.
func (declaration *CancelDeclaration) NewInstance(operand CancelOperand) (*engine.RuleInstance[suspension.Value, CancelOperand], bool) {
	if declaration == nil || declaration.rule == nil || declaration.suspension == nil || !validCancelOperand(operand, declaration.suspension.Schema()) {
		return nil, false
	}
	return engine.NewRuleInstance(declaration.rule, operand, func(binding *engine.RuleBinding[suspension.Value, CancelOperand]) bool {
		ref, ok := declaration.suspension.Locate(operand.suspensionKey)
		return ok && engine.InstanceRead(binding, declaration.read, ref) && engine.InstanceWrite(binding, declaration.write, ref)
	})
}

func cancelOperandContent(operand CancelOperand) (CancelOperand, [32]byte, bool) {
	if !operand.content.Available() || !operand.suspensionKey.Valid() {
		return CancelOperand{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.content), true
}

func validCancelOperand(operand CancelOperand, generations suspension.Schema) bool {
	if operand.source == nil || !operand.content.Available() || !operand.suspensionKey.Valid() || !generations.Valid() {
		return false
	}
	keyLink, keyLinkOK := operand.suspensionKey.LinkContentID()
	keyGeneration, keyGenerationOK := operand.suspensionKey.ModuleInitGeneration()
	_, generation, _, kind, provenanceOK := operand.source.Module().Terminals().Provenance(operand.terminal)
	content, contentOK := operand.source.Module().Terminals().ID(operand.terminal)
	return keyLinkOK && keyGenerationOK && provenanceOK && contentOK && keyLink == operand.source.ContentID() &&
		keyGeneration == generation && kind == flowkind.OutcomeCancel && content == operand.content
}

func (declaration *CancelDeclaration) transfer(access engine.Access[suspension.Value, CancelOperand]) bool {
	operand, operandOK := engine.Operand(access)
	if !operandOK || declaration == nil || declaration.suspension == nil || !validCancelOperand(operand, declaration.suspension.Schema()) {
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

// consumeLiveCompletion is the shared terminal lifecycle law. ModuleInit
// begins at Recent, but the selected generation may subsequently have been
// materialized to Summary. Every terminal completion therefore consumes every
// live alternative that the one exact generation currently carries; it
// neither chooses a different key nor conflates materialization ages. Empty,
// already-consumed, and Top rows have no stronger exact conclusion and do not
// manufacture a write.
func consumeLiveCompletion(schema suspension.Schema, key suspension.Key, current suspension.Value, present bool) (suspension.Value, bool) {
	if !present || !schema.Admits(key, current) || current.IsTop() {
		return suspension.Value{}, false
	}
	next := current
	changed := false
	for _, role := range [...]materialization.Role{materialization.Exact, materialization.Recent, materialization.Summary} {
		consumed, ok := schema.ConsumeLive(next, key, role)
		if !ok {
			continue
		}
		next, changed = consumed, true
	}
	return next, changed
}

func (declaration *CancelDeclaration) check(derivation engine.RuleDerivation[suspension.Value, CancelOperand]) (engine.RuleEvidence, bool) {
	if declaration == nil || declaration.suspension == nil || !declaration.matchesSemantic(derivation.Rule()) || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	input, inputOK := derivation.InputAt(0)
	operand, operandOK := derivation.Operand()
	if !inputOK || input.Guard().Empty() || !operandOK || !validCancelOperand(operand, declaration.suspension.Schema()) || !derivation.OperandContentMatches([32]byte(operand.content)) {
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

// matchesSemantic is the evidence fence for this one declared Rule.  A
// same-shaped Rule cannot reuse this checker merely by reusing its operand and
// factor forms: its sealed semantic identity must be this declaration's.
func (declaration *CancelDeclaration) matchesSemantic(semantic engine.SemanticKey) bool {
	return declaration != nil && declaration.semantic.Available() && declaration.semantic == semantic
}
