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

// ReadyOperand is one successful ModuleInit completion. Link owns the only
// correspondence between its cache coordinate, generation, and returned
// module subject.  It names no raw Program Value and creates no second module
// result relation.
type ReadyOperand struct {
	source        *link.Link
	outcome       linkmodule.ModuleInitOutcome
	moduleKey     module.Key
	suspensionKey suspension.Key
	subject       linkmodule.ModuleReadySubject
	content       keyspace.ContentID
}

// NewReadyOperand accepts only a Link-owned Normal or Return completion with
// its exact ready subject. A successful source term alone cannot select a
// cache coordinate or a lifecycle generation.
func NewReadyOperand(source *link.Link, modules module.Schema, generations suspension.Schema, outcome linkmodule.ModuleInitOutcome) (ReadyOperand, bool) {
	if source == nil || !modules.Valid() || !generations.Valid() || modules.Link() != source || generations.Link() != source {
		return ReadyOperand{}, false
	}
	generation, _, _, provenanceOK := source.Module().Outcomes().Provenance(outcome)
	_, coordinate, _, _, entryOK := source.Module().Generations().Entry(generation)
	subject, readyOK := source.Module().Outcomes().ReadySubject(outcome)
	content, contentOK := source.Module().Outcomes().ID(outcome)
	moduleKey, moduleOK := modules.KeyForCoordinate(coordinate)
	suspensionKey, suspensionOK := generations.KeyForModuleInitGeneration(generation)
	moduleLink, moduleLinkOK := moduleKey.LinkContentID()
	suspensionLink, suspensionLinkOK := suspensionKey.LinkContentID()
	if !provenanceOK || !entryOK || !readyOK || !contentOK || !content.Available() || !moduleOK || !suspensionOK || !moduleLinkOK || !suspensionLinkOK ||
		moduleLink != source.ContentID() || suspensionLink != source.ContentID() {
		return ReadyOperand{}, false
	}
	return ReadyOperand{source: source, outcome: outcome, moduleKey: moduleKey, suspensionKey: suspensionKey, subject: subject, content: content}, true
}

// OutcomeID identifies the existing Link completion consumed by this Rule.
func (operand ReadyOperand) OutcomeID() keyspace.ContentID { return operand.content }

// ReadyDeclaration is the Module-only successful completion capability. It
// observes the matching Suspension lifecycle but never changes it; lifecycle
// termination is owned by a separate Suspension judgment when one is proven.
type ReadyDeclaration struct {
	semantic   engine.SemanticKey
	rule       *engine.Rule[module.Value, ReadyOperand]
	modules    *moduleowner.Owner
	suspension *suspensionowner.Owner
	moduleRead engine.Read[engine.OrderedCells[module.Value]]
	suspRead   engine.Read[engine.OrderedCells[suspension.Value]]
	write      engine.Write[module.Value]
}

// DeclarePublishReady declares Module's cache publication rule for a
// successful module initialization. The two reads are a semantic conjunction,
// not a scheduler-order convention: the cache Pending and lifecycle Live
// alternatives must describe the very same sealed generation and role.
func DeclarePublishReady(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, modules *moduleowner.Owner, generations *suspensionowner.Owner) (*ReadyDeclaration, bool) {
	if composition == nil || modules == nil || generations == nil || !semantic.Available() || !operandFamily.Available() || !evidence.Available() ||
		semantic == operandFamily || semantic == evidence || operandFamily == evidence || modules.Link() == nil || generations.Link() == nil || modules.Link() != generations.Link() {
		return nil, false
	}
	declaration := &ReadyDeclaration{semantic: semantic, modules: modules, suspension: generations}
	rule, ok := engine.DeclareRule(composition, engine.RuleSpec[module.Value, ReadyOperand]{
		Semantic:       semantic,
		OperandFamily:  operandFamily,
		OperandContent: readyOperandContent,
		Output:         modules.Output(),
		Inputs:         1,
		Admission:      engine.AdmitRuleByDerivation(evidence, declaration.check),
		Transfer:       declaration.transfer,
	}, func(rule *engine.Rule[module.Value, ReadyOperand]) bool {
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

// NewInstance binds both reads and the output to the exact Link-derived
// coordinate/generation pair. No caller supplies a bare cache key, result, or
// continuation reference.
func (declaration *ReadyDeclaration) NewInstance(operand ReadyOperand) (*engine.RuleInstance[module.Value, ReadyOperand], bool) {
	if declaration == nil || declaration.rule == nil || declaration.modules == nil || declaration.suspension == nil || !validReadyOperand(operand, declaration.modules.Schema(), declaration.suspension.Schema()) {
		return nil, false
	}
	return engine.NewRuleInstance(declaration.rule, operand, func(binding *engine.RuleBinding[module.Value, ReadyOperand]) bool {
		moduleRef, moduleOK := declaration.modules.Locate(operand.moduleKey)
		suspensionRef, suspensionOK := declaration.suspension.Locate(operand.suspensionKey)
		return moduleOK && suspensionOK && engine.InstanceRead(binding, declaration.moduleRead, moduleRef) &&
			engine.InstanceRead(binding, declaration.suspRead, suspensionRef) && engine.InstanceWrite(binding, declaration.write, moduleRef)
	})
}

func readyOperandContent(operand ReadyOperand) (ReadyOperand, [32]byte, bool) {
	if !operand.content.Available() || !operand.moduleKey.Valid() || !operand.suspensionKey.Valid() {
		return ReadyOperand{}, [32]byte{}, false
	}
	return operand, [32]byte(operand.content), true
}

func validReadyOperand(operand ReadyOperand, modules module.Schema, generations suspension.Schema) bool {
	if operand.source == nil || !operand.content.Available() || !operand.moduleKey.Valid() || !operand.suspensionKey.Valid() || !modules.Valid() || !generations.Valid() || modules.Link() != operand.source || generations.Link() != operand.source {
		return false
	}
	moduleLink, moduleLinkOK := modules.LinkContentID()
	keyLink, keyLinkOK := operand.moduleKey.LinkContentID()
	suspensionLink, suspensionLinkOK := operand.suspensionKey.LinkContentID()
	generation, _, _, provenanceOK := operand.source.Module().Outcomes().Provenance(operand.outcome)
	_, coordinate, _, _, entryOK := operand.source.Module().Generations().Entry(generation)
	subject, readyOK := operand.source.Module().Outcomes().ReadySubject(operand.outcome)
	content, contentOK := operand.source.Module().Outcomes().ID(operand.outcome)
	expectedModuleKey, expectedModuleOK := modules.KeyForCoordinate(coordinate)
	expectedSuspensionKey, expectedSuspensionOK := generations.KeyForModuleInitGeneration(generation)
	keyCoordinate, coordinateOK := operand.moduleKey.Coordinate()
	keyGeneration, generationOK := operand.suspensionKey.ModuleInitGeneration()
	return moduleLinkOK && keyLinkOK && suspensionLinkOK && provenanceOK && entryOK && readyOK && contentOK && expectedModuleOK && expectedSuspensionOK && coordinateOK && generationOK &&
		moduleLink == operand.source.ContentID() && keyLink == moduleLink && suspensionLink == moduleLink && expectedModuleKey == operand.moduleKey && expectedSuspensionKey == operand.suspensionKey && coordinate == keyCoordinate && generation == keyGeneration && subject == operand.subject && content == operand.content
}

func (declaration *ReadyDeclaration) transfer(access engine.Access[module.Value, ReadyOperand]) bool {
	operand, operandOK := engine.Operand(access)
	if !operandOK || declaration == nil || declaration.modules == nil || declaration.suspension == nil || !validReadyOperand(operand, declaration.modules.Schema(), declaration.suspension.Schema()) {
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
		next, published := publishReady(declaration.modules.Schema(), current, operand, lifecycle)
		if !published {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, next)
	})
}

// publishReady preserves every unrelated cache alternative. It publishes only
// a pending alternative whose exact generation and materialization role has a
// matching live lifecycle premise.  In particular a consumed generation, a
// different generation at the same cache coordinate, or a same-generation
// mismatched age cannot be promoted to Ready.
func publishReady(schema module.Schema, current module.Value, operand ReadyOperand, lifecycle suspension.Value) (module.Value, bool) {
	if !schema.Admits(operand.moduleKey, current) || current.IsTop() || !lifecycle.Valid() {
		return module.Value{}, false
	}
	generation, _, _, generationOK := operand.source.Module().Outcomes().Provenance(operand.outcome)
	if !generationOK {
		return module.Value{}, false
	}
	var result module.Value
	published := false
	for index := 0; index < current.PendingCount(); index++ {
		candidate, role, pendingOK := current.PendingAt(index)
		if !pendingOK || candidate != generation || !liveAt(lifecycle, role) {
			continue
		}
		next, reduced := schema.PublishReady(current, operand.moduleKey, generation, role, operand.subject)
		if !reduced {
			return module.Value{}, false
		}
		if !published {
			result, published = next, true
			continue
		}
		joined, joinedOK := schema.Join(result, next)
		if !joinedOK {
			return module.Value{}, false
		}
		result = joined
	}
	return result, published
}

func liveAt(value suspension.Value, wanted materialization.Role) bool {
	if !wanted.Valid() {
		return false
	}
	if value.IsTop() {
		return true
	}
	for index := 0; index < value.LifecycleCount(); index++ {
		role, live, _, _, ok := value.LifecycleAt(index)
		if !ok {
			return false
		}
		if role == wanted && live {
			return true
		}
	}
	return false
}

func (declaration *ReadyDeclaration) check(derivation engine.RuleDerivation[module.Value, ReadyOperand]) (engine.RuleEvidence, bool) {
	if declaration == nil || declaration.modules == nil || declaration.suspension == nil || derivation.Rule() != declaration.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 2 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	input, inputOK := derivation.InputAt(0)
	operand, operandOK := derivation.Operand()
	if !inputOK || input.Guard().Empty() || !operandOK || !validReadyOperand(operand, declaration.modules.Schema(), declaration.suspension.Schema()) || !derivation.OperandContentMatches([32]byte(operand.content)) {
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
	next, published := module.Value{}, false
	if modulePresent && generationPresent {
		next, published = publishReady(declaration.modules.Schema(), current, operand, lifecycle)
	}
	if !published {
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
