// Package rule declares Suspension's lifecycle-writing cold judgments.  It
// owns neither Module cache state nor a composition root.
package rule

import (
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/module"
	moduleowner "github.com/wippyai/go-lua/analysis/domain/module/owner"
	"github.com/wippyai/go-lua/analysis/domain/suspension"
	suspensionowner "github.com/wippyai/go-lua/analysis/domain/suspension/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
)

// InitOperand is Suspension's own typed view of one canonical
// cache-entry-derived ModuleInit generation. The two sibling rules
// independently validate this same cold correspondence; neither observes the
// other's uncommitted output.
type InitOperand struct {
	source        *link.Link
	generation    linkmodule.ModuleInitGeneration
	moduleKey     module.Key
	suspensionKey suspension.Key
	content       keyspace.ContentID
}

// NewInitOperand derives the two owner-local keys from one canonical
// cache-entry-derived generation. It rejects a loose coordinate/generation
// pairing and keeps import cycles out of both this Rule and Suspension.
func NewInitOperand(source *link.Link, modules module.Schema, generations suspension.Schema, generation linkmodule.ModuleInitGeneration) (InitOperand, bool) {
	if source == nil || !modules.Valid() || !generations.Valid() || modules.Link() != source || generations.Link() != source {
		return InitOperand{}, false
	}
	_, coordinate, _, _, generationOK := source.Module().Generations().Entry(generation)
	content, contentOK := source.Module().Generations().ID(generation)
	moduleKey, moduleOK := modules.KeyForCoordinate(coordinate)
	suspensionKey, suspensionOK := generations.KeyForModuleInitGeneration(generation)
	if !generationOK || !contentOK || !content.Available() || !moduleOK || !suspensionOK {
		return InitOperand{}, false
	}
	moduleLink, moduleLinkOK := moduleKey.LinkContentID()
	suspensionLink, suspensionLinkOK := suspensionKey.LinkContentID()
	if !moduleLinkOK || !suspensionLinkOK || moduleLink != source.ContentID() || suspensionLink != source.ContentID() {
		return InitOperand{}, false
	}
	if value, ok := generations.Live(suspensionKey, materialization.Recent); !ok || !generations.Admits(suspensionKey, value) {
		return InitOperand{}, false
	}
	return InitOperand{source: source, generation: generation, moduleKey: moduleKey, suspensionKey: suspensionKey, content: content}, true
}

// GenerationID returns the existing cache-entry-derived identity carried by
// this operand. It is evidence of a sealed correspondence, not a new key.
func (operand InitOperand) GenerationID() keyspace.ContentID { return operand.content }

// Declaration is Suspension's typed cold initiation Rule capability.
type Declaration struct {
	semantic   engine.SemanticKey
	rule       *engine.Rule[suspension.Value, InitOperand]
	modules    *moduleowner.Owner
	suspension *suspensionowner.Owner
	read       engine.Read[engine.OrderedCells[module.Value]]
	write      engine.Write[suspension.Value]
}

// DeclareInit records the Suspension half of ModuleInit.  It reads only the
// common Module predecessor Cold state and writes only the fresh generation's
// live lifecycle; the Module Pending patch is a sibling transaction output.
func DeclareInit(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, modules *moduleowner.Owner, generations *suspensionowner.Owner) (*Declaration, bool) {
	if composition == nil || modules == nil || generations == nil || !semantic.Available() || !operandFamily.Available() || !evidence.Available() ||
		semantic == operandFamily || semantic == evidence || operandFamily == evidence || modules.Link() == nil || generations.Link() == nil || modules.Link() != generations.Link() {
		return nil, false
	}
	declaration := &Declaration{semantic: semantic, modules: modules, suspension: generations}
	rule, ok := engine.DeclareRule(composition, engine.RuleSpec[suspension.Value, InitOperand]{
		Semantic:       semantic,
		OperandFamily:  operandFamily,
		OperandContent: operandContent,
		Output:         generations.Output(),
		Inputs:         1,
		Admission:      engine.AdmitRuleByDerivation(evidence, declaration.check),
		Transfer:       declaration.transfer,
	}, func(rule *engine.Rule[suspension.Value, InitOperand]) bool {
		input, inputOK := rule.InputAt(0)
		read, readOK := engine.ReadFrom(rule, input, modules.Read())
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

// NewInstance binds the common Module predecessor and the distinct
// Suspension output key only after the enclosing cold composition seals.
func (declaration *Declaration) NewInstance(operand InitOperand) (*engine.RuleInstance[suspension.Value, InitOperand], bool) {
	if declaration == nil || declaration.rule == nil || declaration.modules == nil || declaration.suspension == nil || !validOperand(operand, declaration.modules.Schema(), declaration.suspension.Schema()) {
		return nil, false
	}
	return engine.NewRuleInstance(declaration.rule, operand, func(binding *engine.RuleBinding[suspension.Value, InitOperand]) bool {
		moduleRef, moduleOK := declaration.modules.Locate(operand.moduleKey)
		suspensionRef, suspensionOK := declaration.suspension.Locate(operand.suspensionKey)
		return moduleOK && suspensionOK && engine.InstanceRead(binding, declaration.read, moduleRef) && engine.InstanceWrite(binding, declaration.write, suspensionRef)
	})
}

func operandContent(operand InitOperand) (InitOperand, [32]byte, bool) {
	if !operand.content.Available() || !operand.moduleKey.Valid() || !operand.suspensionKey.Valid() {
		return InitOperand{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.content), true
}

func validOperand(operand InitOperand, modules module.Schema, generations suspension.Schema) bool {
	if operand.source == nil || !operand.content.Available() || !operand.moduleKey.Valid() || !operand.suspensionKey.Valid() || !modules.Valid() || !generations.Valid() || modules.Link() != operand.source || generations.Link() != operand.source {
		return false
	}
	moduleLink, moduleOK := operand.moduleKey.LinkContentID()
	suspensionLink, suspensionOK := operand.suspensionKey.LinkContentID()
	coordinate, coordinateOK := operand.moduleKey.Coordinate()
	generation, generationKeyOK := operand.suspensionKey.ModuleInitGeneration()
	_, predecessor, _, _, generationTopologyOK := operand.source.Module().Generations().Entry(operand.generation)
	content, contentOK := operand.source.Module().Generations().ID(operand.generation)
	expectedModuleKey, expectedModuleOK := modules.KeyForCoordinate(predecessor)
	expectedSuspensionKey, expectedSuspensionOK := generations.KeyForModuleInitGeneration(operand.generation)
	if !moduleOK || !suspensionOK || !coordinateOK || !generationKeyOK || !generationTopologyOK || !contentOK || !expectedModuleOK || !expectedSuspensionOK ||
		moduleLink != operand.source.ContentID() || suspensionLink != operand.source.ContentID() || coordinate != predecessor || generation != operand.generation || content != operand.content {
		return false
	}
	if expectedModuleKey != operand.moduleKey || expectedSuspensionKey != operand.suspensionKey {
		return false
	}
	live, liveOK := generations.Live(operand.suspensionKey, materialization.Recent)
	return liveOK && generations.Admits(operand.suspensionKey, live)
}

func (declaration *Declaration) transfer(access engine.Access[suspension.Value, InitOperand]) bool {
	operand, operandOK := engine.Operand(access)
	if !operandOK || declaration == nil || declaration.modules == nil || declaration.suspension == nil || !validOperand(operand, declaration.modules.Schema(), declaration.suspension.Schema()) {
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
		live, liveOK := initLive(declaration.modules.Schema(), operand.moduleKey, declaration.suspension.Schema(), operand.suspensionKey, current, present)
		if !liveOK {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, live)
	})
}

func (declaration *Declaration) check(derivation engine.RuleDerivation[suspension.Value, InitOperand]) (engine.RuleEvidence, bool) {
	if declaration == nil || declaration.modules == nil || declaration.suspension == nil || !declaration.matchesSemantic(derivation.Rule()) ||
		derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	input, inputOK := derivation.InputAt(0)
	if !inputOK || input.Guard().Empty() {
		return engine.RuleEvidence{}, false
	}
	operand, ok := derivation.Operand()
	if !ok || !validOperand(operand, declaration.modules.Schema(), declaration.suspension.Schema()) || !derivation.OperandContentMatches([32]byte(operand.content)) {
		return engine.RuleEvidence{}, false
	}
	moduleRef, moduleOK := declaration.modules.Locate(operand.moduleKey)
	suspensionRef, suspensionOK := declaration.suspension.Locate(operand.suspensionKey)
	if !moduleOK || !suspensionOK || !engine.DerivationReadMatchesRef(derivation, declaration.read, moduleRef) {
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
	live, liveOK := initLive(declaration.modules.Schema(), operand.moduleKey, declaration.suspension.Schema(), operand.suspensionKey, current, present)
	if !liveOK {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	value, staged := disposition.Value()
	target, targetOK := disposition.TargetAt(0)
	if !liveOK || disposition.Kind() != engine.RuleDispositionStaged || !staged || disposition.TargetCount() != 1 || !targetOK ||
		!declaration.suspension.Schema().Admits(operand.suspensionKey, value) || !declaration.suspension.Schema().Equal(value, live) || !engine.TargetMatchesRef(target, suspensionRef) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}

// initLive is the one local may-law shared by execution and evidence.  Module
// Top contains every admitted cache alternative, including Cold; dropping it
// here would erase a lawful ModuleInit -> live-generation consequence.  A
// known Pending/Ready-only relation has no Cold alternative and therefore
// remains NoCandidate.
func initLive(modules module.Schema, moduleKey module.Key, generations suspension.Schema, suspensionKey suspension.Key, current module.Value, present bool) (suspension.Value, bool) {
	if !present || !modules.Admits(moduleKey, current) || !current.HasCold() {
		return suspension.Value{}, false
	}
	live, ok := generations.Live(suspensionKey, materialization.Recent)
	return live, ok && generations.Admits(suspensionKey, live)
}

// matchesSemantic prevents a second Rule with the same Module predecessor,
// Suspension output, and operand shape from replaying this Rule's evidence.
func (declaration *Declaration) matchesSemantic(semantic engine.SemanticKey) bool {
	return declaration != nil && declaration.semantic.Available() && declaration.semantic == semantic
}
