package rule

import (
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/module"
	moduleowner "github.com/wippyai/go-lua/analysis/domain/module/owner"
	"github.com/wippyai/go-lua/analysis/domain/suspension"
	suspensionowner "github.com/wippyai/go-lua/analysis/domain/suspension/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
)

// RestoreOperand is one exact failed-initialization terminal.  Link fixes the
// terminal outcome, cache coordinate, and ModuleInit generation; Suspension
// remains the sole owner of the observed live/consumed lifecycle premise.
type RestoreOperand struct {
	source        *link.Link
	terminal      linkmodule.ModuleInitTerminal
	moduleKey     module.Key
	suspensionKey suspension.Key
	content       keyspace.ContentID
}

// NewRestoreOperand admits only Link's canonical Throw/Cancel terminal view.
// It cannot pair an arbitrary cache coordinate with a generation or turn a
// Link diagnostic (including an import cycle) into Module state.
func NewRestoreOperand(source *link.Link, modules module.Schema, generations suspension.Schema, terminal linkmodule.ModuleInitTerminal) (RestoreOperand, bool) {
	if source == nil || !modules.Valid() || !generations.Valid() || modules.Link() != source || generations.Link() != source {
		return RestoreOperand{}, false
	}
	moduleLink, moduleLinkOK := modules.LinkContentID()
	outcome, generation, coordinate, _, provenanceOK := source.Module().Terminals().Provenance(terminal)
	content, contentOK := source.Module().Terminals().ID(terminal)
	moduleKey, moduleOK := modules.KeyForCoordinate(coordinate)
	suspensionKey, suspensionOK := generations.KeyForModuleInitGeneration(generation)
	suspensionLink, suspensionLinkOK := suspensionKey.LinkContentID()
	if !moduleLinkOK || !provenanceOK || !contentOK || !content.Available() || !moduleOK || !suspensionOK || !suspensionLinkOK ||
		moduleLink != source.ContentID() || suspensionLink != source.ContentID() {
		return RestoreOperand{}, false
	}
	if projected, ok := source.Module().Terminals().Outcome(terminal); !ok || projected != outcome {
		return RestoreOperand{}, false
	}
	return RestoreOperand{source: source, terminal: terminal, moduleKey: moduleKey, suspensionKey: suspensionKey, content: content}, true
}

// TerminalID identifies the existing Link terminal correspondence used by the
// cache-only Rule.  It is not a Module state or a second continuation key.
func (operand RestoreOperand) TerminalID() keyspace.ContentID { return operand.content }

// RestoreDeclaration is Module's terminal cache-writing capability.  It reads
// Module and Suspension at the same committed predecessor and writes only the
// Module cache coordinate.
type RestoreDeclaration struct {
	semantic   engine.SemanticKey
	rule       *engine.Rule[module.Value, RestoreOperand]
	modules    *moduleowner.Owner
	suspension *suspensionowner.Owner
	moduleRead engine.Read[engine.OrderedCells[module.Value]]
	suspRead   engine.Read[engine.OrderedCells[suspension.Value]]
	write      engine.Write[module.Value]
}

// DeclareRestoreCold declares the Module half of a failed module-init
// terminal.  A Throw reads the matching committed live generation; a Cancel
// reads that generation after Suspension's independent consume transition.
// Neither case writes Suspension.
func DeclareRestoreCold(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, modules *moduleowner.Owner, generations *suspensionowner.Owner) (*RestoreDeclaration, bool) {
	if composition == nil || modules == nil || generations == nil || !semantic.Available() || !operandFamily.Available() || !evidence.Available() || modules.Link() == nil || generations.Link() == nil || modules.Link() != generations.Link() {
		return nil, false
	}
	declaration := &RestoreDeclaration{semantic: semantic, modules: modules, suspension: generations}
	rule, ok := engine.DeclareRule(composition, engine.RuleSpec[module.Value, RestoreOperand]{
		Semantic:       semantic,
		OperandFamily:  operandFamily,
		OperandContent: restoreOperandContent,
		Output:         modules.Output(),
		Inputs:         1,
		Admission:      engine.AdmitRuleByDerivation(evidence, declaration.check),
		Transfer:       declaration.transfer,
	}, func(rule *engine.Rule[module.Value, RestoreOperand]) bool {
		input, inputOK := rule.InputAt(0)
		moduleRead, moduleReadOK := engine.ReadFrom(rule, input, modules.Read())
		suspRead, suspReadOK := engine.ReadFrom(rule, input, generations.Read())
		write, writeOK := engine.WriteTo(rule, modules.Write())
		if !inputOK || !moduleReadOK || !suspReadOK || !writeOK {
			return false
		}
		declaration.rule, declaration.moduleRead, declaration.suspRead, declaration.write = rule, moduleRead, suspRead, write
		return true
	})
	if !ok || rule == nil || declaration.rule != rule {
		return nil, false
	}
	return declaration, true
}

// NewInstance binds both exact predecessor reads and the one Module cache
// output after composition seal.  It never creates a Suspension write.
func (declaration *RestoreDeclaration) NewInstance(operand RestoreOperand) (*engine.RuleInstance[module.Value, RestoreOperand], bool) {
	if declaration == nil || declaration.rule == nil || declaration.modules == nil || declaration.suspension == nil || !validRestoreOperand(operand, declaration.modules.Schema(), declaration.suspension.Schema()) {
		return nil, false
	}
	return engine.NewRuleInstance(declaration.rule, operand, func(binding *engine.RuleBinding[module.Value, RestoreOperand]) bool {
		moduleRef, moduleOK := declaration.modules.Locate(operand.moduleKey)
		suspensionRef, suspensionOK := declaration.suspension.Locate(operand.suspensionKey)
		return moduleOK && suspensionOK && engine.InstanceRead(binding, declaration.moduleRead, moduleRef) &&
			engine.InstanceRead(binding, declaration.suspRead, suspensionRef) && engine.InstanceWrite(binding, declaration.write, moduleRef)
	})
}

func restoreOperandContent(operand RestoreOperand) (RestoreOperand, [32]byte, bool) {
	if !operand.content.Available() || !operand.moduleKey.Valid() || !operand.suspensionKey.Valid() {
		return RestoreOperand{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.content), true
}

func validRestoreOperand(operand RestoreOperand, modules module.Schema, generations suspension.Schema) bool {
	if operand.source == nil || !operand.content.Available() || !operand.moduleKey.Valid() || !operand.suspensionKey.Valid() || !modules.Valid() || !generations.Valid() || modules.Link() != operand.source || generations.Link() != operand.source {
		return false
	}
	moduleLink, moduleLinkOK := modules.LinkContentID()
	keyLink, keyLinkOK := operand.moduleKey.LinkContentID()
	suspensionLink, suspensionLinkOK := operand.suspensionKey.LinkContentID()
	outcome, generation, coordinate, _, provenanceOK := operand.source.Module().Terminals().Provenance(operand.terminal)
	content, contentOK := operand.source.Module().Terminals().ID(operand.terminal)
	keyCoordinate, coordinateOK := operand.moduleKey.Coordinate()
	keyGeneration, generationOK := operand.suspensionKey.ModuleInitGeneration()
	expectedModuleKey, expectedModuleOK := modules.KeyForCoordinate(coordinate)
	expectedSuspensionKey, expectedSuspensionOK := generations.KeyForModuleInitGeneration(generation)
	if !moduleLinkOK || !keyLinkOK || !suspensionLinkOK || !provenanceOK || !contentOK || !coordinateOK || !generationOK || !expectedModuleOK || !expectedSuspensionOK ||
		moduleLink != operand.source.ContentID() || keyLink != moduleLink || suspensionLink != moduleLink || coordinate != keyCoordinate || generation != keyGeneration || content != operand.content {
		return false
	}
	if expectedModuleKey != operand.moduleKey || expectedSuspensionKey != operand.suspensionKey {
		return false
	}
	projected, projectedOK := operand.source.Module().Terminals().Outcome(operand.terminal)
	return projectedOK && projected == outcome
}

func (declaration *RestoreDeclaration) transfer(access engine.Access[module.Value, RestoreOperand]) bool {
	operand, operandOK := engine.Operand(access)
	if !operandOK || declaration == nil || declaration.modules == nil || declaration.suspension == nil || !validRestoreOperand(operand, declaration.modules.Schema(), declaration.suspension.Schema()) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		modules, moduleReadOK := engine.ReadValue(access, row, declaration.moduleRead)
		generations, generationReadOK := engine.ReadValue(access, row, declaration.suspRead)
		if !moduleReadOK || !generationReadOK || modules.Count() != 1 || generations.Count() != 1 {
			return false
		}
		current, modulePresent, moduleOK := modules.At(0)
		lifecycle, generationPresent, generationOK := generations.At(0)
		if !moduleOK || !generationOK || !modulePresent || !generationPresent {
			return engine.NoCandidate(access, row)
		}
		next, restored := restoreTerminal(declaration.modules.Schema(), current, operand, lifecycle)
		if !restored {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, next)
	})
}

func restoreTerminal(schema module.Schema, current module.Value, operand RestoreOperand, lifecycle suspension.Value) (module.Value, bool) {
	_, _, _, kind, terminalOK := operand.source.Module().Terminals().Provenance(operand.terminal)
	if !terminalOK || !schema.Admits(operand.moduleKey, current) || current.IsTop() || !lifecycleMatchesTerminal(lifecycle, kind) {
		return module.Value{}, false
	}
	var result module.Value
	restored := false
	for index := 0; index < current.PendingCount(); index++ {
		generation, role, pendingOK := current.PendingAt(index)
		if !pendingOK || generation != moduleGeneration(operand) || !matchingLifecycle(lifecycle, kind, role) {
			continue
		}
		next, reduced := schema.RestoreCold(current, operand.moduleKey, generation, role)
		if !reduced {
			return module.Value{}, false
		}
		if !restored {
			result, restored = next, true
			continue
		}
		joined, joinedOK := schema.Join(result, next)
		if !joinedOK {
			return module.Value{}, false
		}
		result = joined
	}
	return result, restored
}

func moduleGeneration(operand RestoreOperand) linkmodule.ModuleInitGeneration {
	_, generation, _, _, ok := operand.source.Module().Terminals().Provenance(operand.terminal)
	if !ok {
		return linkmodule.ModuleInitGeneration{}
	}
	return generation
}

func lifecycleMatchesTerminal(value suspension.Value, kind flowkind.OutcomeKind) bool {
	if value.IsTop() {
		return true
	}
	for index := 0; index < value.LifecycleCount(); index++ {
		_, live, consumed, _, ok := value.LifecycleAt(index)
		if !ok {
			return false
		}
		if kind == flowkind.OutcomeThrow && live || kind == flowkind.OutcomeCancel && consumed {
			return true
		}
	}
	return false
}

func matchingLifecycle(value suspension.Value, kind flowkind.OutcomeKind, role materialization.Role) bool {
	if !role.Valid() {
		return false
	}
	if value.IsTop() {
		return true
	}
	for index := 0; index < value.LifecycleCount(); index++ {
		candidate, live, consumed, _, ok := value.LifecycleAt(index)
		if !ok || candidate != role {
			continue
		}
		return kind == flowkind.OutcomeThrow && live || kind == flowkind.OutcomeCancel && consumed
	}
	return false
}

func (declaration *RestoreDeclaration) check(derivation engine.RuleDerivation[module.Value, RestoreOperand]) (engine.RuleEvidence, bool) {
	if declaration == nil || declaration.modules == nil || declaration.suspension == nil || derivation.Rule() != declaration.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 2 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	input, inputOK := derivation.InputAt(0)
	operand, operandOK := derivation.Operand()
	if !inputOK || input.Guard().Empty() || !operandOK || !validRestoreOperand(operand, declaration.modules.Schema(), declaration.suspension.Schema()) || !derivation.OperandContentMatches([32]byte(operand.content)) {
		return engine.RuleEvidence{}, false
	}
	moduleRef, moduleRefOK := declaration.modules.Locate(operand.moduleKey)
	suspensionRef, suspensionRefOK := declaration.suspension.Locate(operand.suspensionKey)
	if !moduleRefOK || !suspensionRefOK || !engine.DerivationReadMatchesRef(derivation, declaration.moduleRead, moduleRef) || !engine.DerivationReadMatchesRef(derivation, declaration.suspRead, suspensionRef) {
		return engine.RuleEvidence{}, false
	}
	disposition, dispositionOK := derivation.DispositionAt(0)
	modules, moduleReadOK := engine.DerivationDispositionReadValue(derivation, disposition, declaration.moduleRead)
	generations, generationReadOK := engine.DerivationDispositionReadValue(derivation, disposition, declaration.suspRead)
	if !dispositionOK || !moduleReadOK || !generationReadOK || modules.Count() != 1 || generations.Count() != 1 || !disposition.Guard().Same(input.Guard()) {
		return engine.RuleEvidence{}, false
	}
	current, modulePresent, moduleCellOK := modules.At(0)
	lifecycle, generationPresent, generationCellOK := generations.At(0)
	if !moduleCellOK || !generationCellOK {
		return engine.RuleEvidence{}, false
	}
	next, restored := module.Value{}, false
	if modulePresent && generationPresent {
		next, restored = restoreTerminal(declaration.modules.Schema(), current, operand, lifecycle)
	}
	if !restored {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	value, staged := disposition.Value()
	target, targetOK := disposition.TargetAt(0)
	if disposition.Kind() != engine.RuleDispositionStaged || !staged || disposition.TargetCount() != 1 || !targetOK || !declaration.modules.Schema().Admits(operand.moduleKey, value) || !declaration.modules.Schema().Equal(value, next) || !engine.TargetMatchesRef(target, moduleRef) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}
