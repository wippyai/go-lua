package engine

// SupportCompletion is the Composition/Solver-owned structural completion
// capability. It names neither a Factor value nor an execution carrier. Its
// one Prune capability is the only declared way a support-only Rule may
// refine that completed structural support.
type SupportCompletion struct{ schema *supportCompletionSchema }

type supportCompletionSchema struct {
	composition *Composition
	semantic    SemanticKey
	prune       *pruneSchema
}

// Prune is the unique structural refinement capability paired with one
// SupportCompletion. It has no Boolean/guard language, coordinate, state, or
// action authority.
type Prune struct{ schema *pruneSchema }

type pruneSchema struct {
	completion *supportCompletionSchema
	semantic   SemanticKey
}

// SupportRuleSpec declares an output-free structural support refinement.
// Inputs is a finite ordered predecessor vector.  Declare may use the same
// typed Factor read and read-selector projection surface as a Factor Rule, but
// SupportRule exposes no Factor write or carry capability. Run is retained as
// typed behavior for E; it is never evaluated, hashed, or used to manufacture
// a second declaration path in D.
type SupportRuleSpec struct {
	Semantic   SemanticKey
	Completion SupportCompletion
	Prune      Prune
	Inputs     int
	Declare    func(*SupportRule) bool
	Admission  RuleAdmission[Support, ruleUnit]
	Run        func(Support) (Support, bool)
}

// SupportRule is the sealed declaration capability for a structural output
// judgment. It exposes typed Input/Read projection only; it has no Carry,
// Write, Output, or Access surface.
type SupportRule struct {
	composition *Composition
	schema      *ruleSchema
	admission   RuleAdmission[Support, ruleUnit]
	run         func(Support) (Support, bool)
	open        bool
}

type supportRuleSchema struct {
	completion *supportCompletionSchema
	prune      *pruneSchema
}

// AdmitSupportByTrustedTheorem names an explicit reviewed theorem for a
// structural support Rule without exposing the engine's private unit operand
// witness type.
func AdmitSupportByTrustedTheorem(identity SemanticKey) RuleAdmission[Support, ruleUnit] {
	return AdmitRuleByTrustedTheorem[Support, ruleUnit](identity)
}

// DeclareSupportCompletion registers the Composition/Solver structural
// completion after all Factor declarations. A Composition can retain at most
// one such projection, and it must be paired with exactly one Prune before
// seal. This registration creates no Factor capability or graph surface.
func DeclareSupportCompletion(composition *Composition, semantic SemanticKey) (SupportCompletion, bool) {
	if composition == nil || composition.completion != nil || !semantic.Available() || !composition.acceptsChild(semantic) {
		if composition != nil {
			composition.poison()
		}
		return SupportCompletion{}, false
	}
	schema := &supportCompletionSchema{composition: composition, semantic: semantic}
	composition.completion = schema
	return SupportCompletion{schema: schema}, true
}

// DeclarePrune declares the one structural refinement semantic for a
// registered Composition completion while child declaration remains open.
func DeclarePrune(completion SupportCompletion, semantic SemanticKey) (Prune, bool) {
	schema := completion.schema
	if schema == nil || schema.composition == nil || schema.composition.completion != schema || schema.composition.phase != compositionChildren || !schema.semantic.Available() || schema.prune != nil || !semantic.Available() || semantic == schema.semantic || !schema.composition.claim(semantic) {
		if schema != nil && schema.composition != nil {
			schema.composition.poison()
		}
		return Prune{}, false
	}
	prune := &pruneSchema{completion: schema, semantic: semantic}
	schema.prune = prune
	return Prune{schema: prune}, true
}

// DeclareSupportRule records one structural output support refinement.
// Support completion and prune ownership are checked structurally; no Solver,
// local index, carrier, guard, Point, action, or runtime adapter is admitted
// here.  A nil Declare is the admitted zero-read structural contradiction.
func DeclareSupportRule(composition *Composition, spec SupportRuleSpec) (*SupportRule, bool) {
	if composition == nil || !validSupportRuleSpec(composition, spec) || !composition.acceptsChild(spec.Semantic) {
		if composition != nil {
			composition.poison()
		}
		return nil, false
	}
	support := &supportRuleSchema{completion: spec.Completion.schema, prune: spec.Prune.schema}
	admission, admitted := spec.Admission.cold()
	if !admitted {
		composition.poison()
		return nil, false
	}
	schema := &ruleSchema{composition: composition, semantic: spec.Semantic, operandFamily: unitOperandFamily, outputKind: ruleStructuralOutput, inputs: spec.Inputs, admission: admission, support: support, open: true}
	rule := &SupportRule{composition: composition, schema: schema, admission: spec.Admission, run: spec.Run, open: true}
	schema.bind = supportRuleBind{owner: rule}
	composition.rules = append(composition.rules, schema)
	declared := true
	if spec.Declare != nil {
		func() {
			defer func() {
				if recover() != nil {
					composition.poison()
					declared = false
				}
				rule.open = false
				schema.open = false
			}()
			declared = spec.Declare(rule)
		}()
	} else {
		rule.open = false
		schema.open = false
	}
	if !declared || !composition.usable() {
		composition.poison()
		return nil, false
	}
	return rule, true
}

func validSupportRuleSpec(composition *Composition, spec SupportRuleSpec) bool {
	completion := spec.Completion.schema
	prune := spec.Prune.schema
	return composition != nil && spec.Semantic.Available() && spec.Inputs >= 0 && spec.Admission.valid() && spec.Run != nil &&
		completion != nil && completion.composition == composition && composition.completion == completion && completion.semantic.Available() &&
		prune != nil && prune.completion == completion && completion.prune == prune && prune.semantic.Available()
}

func validColdSupportCompletion(composition *Composition, completion *supportCompletionSchema) bool {
	if completion == nil {
		return true
	}
	if composition == nil || composition.completion != completion || completion.composition != composition ||
		!completion.semantic.Available() || completion.prune == nil || completion.prune.completion != completion ||
		!completion.prune.semantic.Available() || completion.prune.semantic == completion.semantic {
		return false
	}
	_, completionClaimed := composition.semantics[completion.semantic]
	_, pruneClaimed := composition.semantics[completion.prune.semantic]
	return completionClaimed && pruneClaimed
}

// available remains true after Composition.Seal for Wave-E binding. It gives
// no mutation authority and does not execute the retained callback.
func (rule *SupportRule) available() bool {
	return rule != nil && rule.composition != nil && rule.schema != nil && rule.schema.composition == rule.composition && rule.schema.support != nil && rule.composition.available()
}

// InputAt issues one declared structural predecessor port while the Support
// Rule's declaration callback is open.
func (rule *SupportRule) InputAt(index int) (Input, bool) {
	if !rule.validOpen() || index < 0 || index >= rule.schema.inputs {
		rule.poison()
		return Input{}, false
	}
	return Input{rule: rule.schema, index: index}, true
}

func (rule *SupportRule) readSchema() *ruleSchema {
	if rule == nil {
		return nil
	}
	return rule.schema
}

func (rule *SupportRule) validReadInput(input Input) bool {
	return rule.validOpen() && input.rule == rule.schema && input.index >= 0 && input.index < rule.schema.inputs
}

func (rule *SupportRule) validReadDependencies(dependencies []Dependency) bool {
	if len(dependencies) == 0 {
		return false
	}
	last := -1
	for _, dependency := range dependencies {
		if dependency.rule != rule.schema || dependency.kind != readDependency || dependency.index < 0 || dependency.index >= len(rule.schema.reads) || dependency.index <= last {
			return false
		}
		last = dependency.index
	}
	return true
}

func (rule *SupportRule) validOpen() bool {
	return rule != nil && rule.open && rule.composition != nil && rule.schema != nil && rule.schema.open && rule.schema.composition == rule.composition && rule.composition.usable()
}

func (rule *SupportRule) poison() {
	if rule != nil && rule.composition != nil {
		rule.composition.poison()
	}
}
