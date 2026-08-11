// Package rule declares Module's cache-writing cold judgments.  It owns no
// cache relation, Link projection, or composition root.
package rule

import (
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/module"
	moduleowner "github.com/wippyai/go-lua/analysis/domain/module/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
)

// InitOperand is Module's view of one canonical cache-entry-derived
// generation. It retains neither a mutable cache value nor a Suspension
// conclusion; the sibling initiation rule receives the same generation
// independently.
type InitOperand struct {
	source     *link.Link
	generation linkmodule.ModuleInitGeneration
	key        module.Key
	content    keyspace.ContentID
}

// NewInitOperand admits one exact canonical ModuleInit generation for this
// Module schema. The generation's cache entry owns the matching predecessor
// coordinate; no Link-side transaction is materialized.
func NewInitOperand(source *link.Link, schema module.Schema, generation linkmodule.ModuleInitGeneration) (InitOperand, bool) {
	if source == nil || !schema.Valid() {
		return InitOperand{}, false
	}
	linkID, linkOK := schema.LinkContentID()
	if !linkOK || linkID != source.ContentID() {
		return InitOperand{}, false
	}
	_, coordinate, _, _, generationOK := source.Module().Generations().Entry(generation)
	content, contentOK := source.Module().Generations().ID(generation)
	key, keyOK := schema.KeyForCoordinate(coordinate)
	if !generationOK || !contentOK || !content.Available() || !keyOK {
		return InitOperand{}, false
	}
	// This forces the supplied generation to be the exact cache-entry-derived
	// generation for its coordinate, rather than merely one supported there.
	if pending, ok := schema.Pending(key, generation, materialization.Recent); !ok || pending.PendingCount() != 1 {
		return InitOperand{}, false
	}
	return InitOperand{source: source, generation: generation, key: key, content: content}, true
}

// GenerationID identifies the existing cache-entry-derived generation used by
// this operand. It is not a new Rule or cache identity.
func (operand InitOperand) GenerationID() keyspace.ContentID { return operand.content }

// Declaration is Module's typed cold Rule capability.  It can later bind one
// exact Link operand but exposes no Factor coordinate or engine State.
type Declaration struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[module.Value, InitOperand]
	owner    *moduleowner.Owner
	read     engine.Read[engine.OrderedCells[module.Value]]
	write    engine.Write[module.Value]
}

// DeclareInit records Module's half of the two-owner ModuleInit successor.
// Its only Factor read is its own predecessor cache state.  In particular it
// has no Suspension read, so it cannot observe the sibling's uncommitted
// generation patch.
func DeclareInit(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, owner *moduleowner.Owner) (*Declaration, bool) {
	if composition == nil || owner == nil || !semantic.Available() || !operandFamily.Available() || !evidence.Available() {
		return nil, false
	}
	declaration := &Declaration{semantic: semantic, owner: owner}
	rule, ok := engine.DeclareRule(composition, engine.RuleSpec[module.Value, InitOperand]{
		Semantic:       semantic,
		OperandFamily:  operandFamily,
		OperandContent: operandContent,
		Output:         owner.Output(),
		Inputs:         1,
		Admission:      engine.AdmitRuleByDerivation(evidence, declaration.check),
		Transfer:       declaration.transfer,
	}, func(rule *engine.Rule[module.Value, InitOperand]) bool {
		input, inputOK := rule.InputAt(0)
		read, readOK := engine.ReadFrom(rule, input, owner.Read())
		write, writeOK := engine.WriteTo(rule, owner.Write())
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

// NewInstance binds the one exact predecessor/output cache coordinate after
// the enclosing composition seals.  It is intentionally not a composition
// root and does not assemble a Program or Solver.
func (declaration *Declaration) NewInstance(operand InitOperand) (*engine.RuleInstance[module.Value, InitOperand], bool) {
	if declaration == nil || declaration.rule == nil || declaration.owner == nil || !validOperand(operand, declaration.owner.Schema()) {
		return nil, false
	}
	return engine.NewRuleInstance(declaration.rule, operand, func(binding *engine.RuleBinding[module.Value, InitOperand]) bool {
		ref, ok := declaration.owner.Locate(operand.key)
		return ok && engine.InstanceRead(binding, declaration.read, ref) && engine.InstanceWrite(binding, declaration.write, ref)
	})
}

func operandContent(operand InitOperand) (InitOperand, [32]byte, bool) {
	if !operand.content.Available() || !operand.key.Valid() {
		return InitOperand{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.content), true
}

func validOperand(operand InitOperand, schema module.Schema) bool {
	if !schema.Valid() || !operand.key.Valid() || !operand.content.Available() {
		return false
	}
	schemaLinkID, schemaLinkOK := schema.LinkContentID()
	keyLinkID, keyLinkOK := operand.key.LinkContentID()
	if !schemaLinkOK || !keyLinkOK || keyLinkID != schemaLinkID || operand.source == nil || operand.source.ContentID() != schemaLinkID {
		return false
	}
	coordinate, coordinateOK := operand.key.Coordinate()
	_, predecessor, _, _, generationOK := operand.source.Module().Generations().Entry(operand.generation)
	content, contentOK := operand.source.Module().Generations().ID(operand.generation)
	if !coordinateOK || !generationOK || !contentOK || content != operand.content || coordinate != predecessor {
		return false
	}
	pending, pendingOK := schema.Pending(operand.key, operand.generation, materialization.Recent)
	return pendingOK && pending.PendingCount() == 1
}

func (declaration *Declaration) transfer(access engine.Access[module.Value, InitOperand]) bool {
	operand, operandOK := engine.Operand(access)
	if !operandOK || declaration == nil || declaration.owner == nil || !validOperand(operand, declaration.owner.Schema()) {
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
		if !present {
			return engine.NoCandidate(access, row)
		}
		next, reduced := declaration.owner.Schema().BeginInit(current, operand.key, operand.generation)
		if !reduced {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, next)
	})
}

func (declaration *Declaration) check(derivation engine.RuleDerivation[module.Value, InitOperand]) (engine.RuleEvidence, bool) {
	if declaration == nil || declaration.owner == nil || derivation.Rule() != declaration.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	input, inputOK := derivation.InputAt(0)
	if !inputOK || input.Guard().Empty() {
		return engine.RuleEvidence{}, false
	}
	operand, ok := derivation.Operand()
	if !ok || !validOperand(operand, declaration.owner.Schema()) || !derivation.OperandContentMatches([32]byte(operand.content)) {
		return engine.RuleEvidence{}, false
	}
	ref, ok := declaration.owner.Locate(operand.key)
	if !ok || !engine.DerivationReadMatchesRef(derivation, declaration.read, ref) {
		return engine.RuleEvidence{}, false
	}
	disposition, dispositionOK := derivation.DispositionAt(0)
	cells, readOK := engine.DerivationDispositionReadValue(derivation, disposition, declaration.read)
	if !dispositionOK || !readOK || cells.Count() != 1 || !disposition.Guard().Same(input.Guard()) {
		return engine.RuleEvidence{}, false
	}
	current, present, cellOK := cells.At(0)
	if !cellOK {
		return engine.RuleEvidence{}, false
	}
	next, reduced := declaration.owner.Schema().BeginInit(current, operand.key, operand.generation)
	if !present || !reduced {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	value, staged := disposition.Value()
	target, targetOK := disposition.TargetAt(0)
	if disposition.Kind() != engine.RuleDispositionStaged || !staged || disposition.TargetCount() != 1 || !targetOK || !declaration.owner.Schema().Admits(operand.key, value) || !declaration.owner.Schema().Equal(value, next) || !engine.TargetMatchesRef(target, ref) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}
