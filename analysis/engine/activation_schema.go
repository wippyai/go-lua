package engine

import (
	coldcomposition "github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

// ActivationFamily owns one cold semantic capability. It owns neither axis
// values nor a topology; equation seals constituent membership and issues
// tuples for the declared family only.
type ActivationFamily struct{ schema *activationFamilySchema }

type activationFamilySchema struct {
	composition *Composition
	semantic    SemanticKey
	cold        coldcomposition.ActivationFamily
}

func (family ActivationFamily) available() bool {
	return family.schema != nil && family.schema.composition != nil && family.schema.composition.available() && family.schema.semantic.Available()
}

// DeclareActivationFamily records only a semantic activation permission.  A
// finite constituent axes are Program/Link topology data and are intentionally absent from
// the cold Composition so there is one structural authority.
func DeclareActivationFamily(composition *Composition, semantic SemanticKey) (ActivationFamily, bool) {
	if composition == nil || !semantic.Available() || !composition.acceptsChild(semantic) {
		if composition != nil {
			composition.poison()
		}
		return ActivationFamily{}, false
	}
	cold := coldcomposition.ActivationFamily{Semantic: semantic.compositionKey()}
	canonical, ok := coldcomposition.CanonicalActivationFamily(cold)
	if !ok {
		composition.poison()
		return ActivationFamily{}, false
	}
	family := &activationFamilySchema{composition: composition, semantic: semantic, cold: canonical}
	composition.activations = append(composition.activations, family)
	return ActivationFamily{schema: family}, true
}

// AdmitActivationByTrustedTheorem names an explicit reviewed theorem for a
// structural activation Rule without exposing the engine's private unit
// operand witness type.
func AdmitActivationByTrustedTheorem(identity SemanticKey) RuleAdmission[ActivationResult, ruleUnit] {
	return AdmitRuleByTrustedTheorem[ActivationResult, ruleUnit](identity)
}

// ActivationRuleSpec is an output-free typed trigger declaration. It names
// exactly one cold family permission and may declare typed reads. Run is the
// semantic decision for the current Product row; it receives an opaque
// Activation frame and can submit relation locators, but cannot enumerate a
// activation axes or manufacture a Member.
type ActivationRuleSpec struct {
	Semantic  SemanticKey
	Family    ActivationFamily
	Inputs    int
	Declare   func(*ActivationRule) bool
	Admission RuleAdmission[ActivationResult, ruleUnit]
	Run       func(Activation) bool
}

// ActivationRule exposes only predecessor Input/Read declaration. It has no
// Factor output, Write, Carry, Point, scheduler, Pair, or Member capability.
type ActivationRule struct {
	composition *Composition
	schema      *ruleSchema
	admission   RuleAdmission[ActivationResult, ruleUnit]
	run         func(Activation) bool
	open        bool
}

type activationRuleSchema struct{ family *activationFamilySchema }

// DeclareActivationRule records one structural trigger schema.  Topology
// supplies the only concrete Member at seal; an activation Rule can therefore
// never mint, enumerate, rename, or retain dynamic relation coordinates.
func DeclareActivationRule(composition *Composition, spec ActivationRuleSpec) (*ActivationRule, bool) {
	if composition == nil || !validActivationRuleSpec(composition, spec) || !composition.acceptsChild(spec.Semantic) {
		if composition != nil {
			composition.poison()
		}
		return nil, false
	}
	admission, admitted := spec.Admission.cold()
	if !admitted {
		composition.poison()
		return nil, false
	}
	activation := &activationRuleSchema{family: spec.Family.schema}
	schema := &ruleSchema{composition: composition, semantic: spec.Semantic, operandFamily: unitOperandFamily, outputKind: ruleStructuralOutput, inputs: spec.Inputs, admission: admission, activation: activation, open: true}
	rule := &ActivationRule{composition: composition, schema: schema, admission: spec.Admission, run: spec.Run, open: true}
	schema.bind = activationRuleBind{owner: rule}
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

func validActivationRuleSpec(composition *Composition, spec ActivationRuleSpec) bool {
	family := spec.Family.schema
	return composition != nil && spec.Semantic.Available() && spec.Inputs >= 0 && spec.Admission.valid() && spec.Run != nil && family != nil && family.composition == composition && family.semantic.Available()
}

func (rule *ActivationRule) available() bool {
	return rule != nil && rule.composition != nil && rule.schema != nil && rule.schema.composition == rule.composition && rule.schema.activation != nil && rule.composition.available()
}

func (rule *ActivationRule) InputAt(index int) (Input, bool) {
	if !rule.validOpen() || index < 0 || index >= rule.schema.inputs {
		rule.poison()
		return Input{}, false
	}
	return Input{rule: rule.schema, index: index}, true
}

func (rule *ActivationRule) readSchema() *ruleSchema {
	if rule == nil {
		return nil
	}
	return rule.schema
}

func (rule *ActivationRule) validReadInput(input Input) bool {
	return rule.validOpen() && input.rule == rule.schema && input.index >= 0 && input.index < rule.schema.inputs
}

func (rule *ActivationRule) validReadDependencies(dependencies []Dependency) bool {
	return validStructuralReadDependencies(rule.schema, dependencies)
}

func (rule *ActivationRule) validOpen() bool {
	return rule != nil && rule.open && rule.composition != nil && rule.schema != nil && rule.schema.open && rule.schema.composition == rule.composition && rule.composition.usable()
}

func (rule *ActivationRule) poison() {
	if rule != nil && rule.composition != nil {
		rule.composition.poison()
	}
}

func validColdActivationFamily(composition *Composition, family *activationFamilySchema) bool {
	if composition == nil || family == nil || family.composition != composition || !family.semantic.Available() {
		return false
	}
	canonical, ok := coldcomposition.CanonicalActivationFamily(family.cold)
	return ok && canonical.Semantic == family.semantic.compositionKey()
}

func validColdActivationRule(composition *Composition, rule *ruleSchema) bool {
	if rule == nil || rule.outputKind != ruleStructuralOutput || rule.output != nil || rule.inputs < 0 || len(rule.carries) != 0 || len(rule.writes) != 0 || rule.support != nil || rule.activation == nil || !validColdActivationFamily(composition, rule.activation.family) {
		return false
	}
	for _, read := range rule.reads {
		if read.input < 0 || read.input >= rule.inputs || !validColdReadForm(composition, read.form) {
			return false
		}
	}
	return validStructuralSelectors(rule)
}

func validStructuralSelectors(rule *ruleSchema) bool {
	if rule == nil {
		return false
	}
	readSelectors := make(map[int]struct{}, len(rule.readSelectors))
	for _, selector := range rule.readSelectors {
		if selector.bind == nil || len(selector.depends) == 0 || selector.read < 0 || selector.read >= len(rule.reads) {
			return false
		}
		if _, duplicate := readSelectors[selector.read]; duplicate {
			return false
		}
		readSelectors[selector.read] = struct{}{}
		selected := rule.reads[selector.read]
		if selected.form.readKind != exactReadForm || !sameDependencies(selected.depends, selector.depends) {
			return false
		}
		lastDependency := -1
		for _, dependency := range selector.depends {
			if dependency.rule != rule || dependency.kind != readDependency || dependency.index < 0 || dependency.index >= selector.read || dependency.index <= lastDependency {
				return false
			}
			lastDependency = dependency.index
		}
	}
	for index, read := range rule.reads {
		if len(read.depends) != 0 {
			if _, present := readSelectors[index]; !present {
				return false
			}
		} else if _, present := readSelectors[index]; present {
			return false
		}
	}
	return true
}

func validStructuralReadDependencies(schema *ruleSchema, dependencies []Dependency) bool {
	if schema == nil || len(dependencies) == 0 {
		return false
	}
	last := -1
	for _, dependency := range dependencies {
		if dependency.rule != schema || dependency.kind != readDependency || dependency.index < 0 || dependency.index >= len(schema.reads) || dependency.index <= last {
			return false
		}
		last = dependency.index
	}
	return true
}
