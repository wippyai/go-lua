package engine

import (
	"sync"
	"sync/atomic"

	coldcomposition "github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// nextSealAuthority issues private live capabilities for successfully sealed
// compositions. It is deliberately not semantic identity: it is neither
// serialized, hashed, reported, nor admitted into runtime State. Its only
// purpose is to prevent opaque Refs crossing between two separately sealed
// but canonically equal Composition objects.
var nextSealAuthority atomic.Uint64

// CompositionID is the opaque canonical digest of one sealed cold
// Composition. It is derived from the complete Factor/Rule/Query schema;
// callers can compare or persist its digest but cannot construct one or
// depend on the internal composition representation.
type CompositionID struct{ digest [32]byte }

// Available reports whether this is a derived sealed-composition identity.
func (id CompositionID) Available() bool { return id.digest != [32]byte{} }

// Digest returns the canonical sealed-composition digest.
func (id CompositionID) Digest() [32]byte { return id.digest }

// RuleAdmissionRecord is one canonical semantic provenance row. Trusted
// theorem rows are explicit TCB obligations: the engine has neither checked
// an artifact nor established an exhaustive proof for them. A Wave-D
// composition gate must review or reject those rows deliberately.
type RuleAdmissionRecord struct {
	Rule     SemanticKey
	Basis    RuleAdmissionBasis
	Identity SemanticKey
}

// RuleAdmissionInventory is an immutable-by-value report over every Rule in
// one sealed Composition. ID binds the report to the exact cold schema from
// which its rows were derived; Rules are in canonical Rule semantic-key order.
type RuleAdmissionInventory struct {
	ID    CompositionID
	Rules []RuleAdmissionRecord
}

// Composition is the single-use cold declaration authority. It owns Factor,
// Rule, Query, and the optional Solver structural-support schema. It has no guard universe, carrier,
// factbinding, scheduler, action, State, or executable callback frame.
//
// Declaration is deliberately fail-closed. A failed capability use poisons
// the entire unsealed Composition; callers must discard it rather than repair
// or retry it. A successfully sealed Composition is immutable input to the
// sole Wave-E template compiler.
type Composition struct {
	phase compositionPhase
	// activeFactor is set only while one Factor owner's declaration callback
	// runs. Child and nested Factor declarations during that callback poison
	// the candidate, preserving the all-Factors-before-children cut.
	activeFactor *factorSchema

	semantics   map[SemanticKey]struct{}
	factors     []*factorSchema
	completion  *supportCompletionSchema
	activations []*activationFamilySchema
	rules       []*ruleSchema
	// rulesByIndex is frozen once from the sealed canonical RuleIndex. It is a
	// dense lookup projection, not another registry or semantic authority.
	rulesByIndex []*ruleSchema
	queries      []coldQuery

	sealed *coldcomposition.Composition
	id     CompositionID
	// sealAuthority is a process-local capability boundary. It is assigned
	// exactly once after semantic sealing succeeds and never contributes to
	// CompositionID or cold-composition content.
	sealAuthority uint64
}

type compositionPhase uint8

const (
	compositionFactors compositionPhase = iota + 1
	compositionChildren
	compositionSealed
	compositionPoisoned
)

// Solver is intentionally opaque at Wave D. Only the Wave-E template
// compiler may construct one after it has bound a sealed Composition to exact
// Program/Link coordinates. There is no public Solver constructor.
type Solver struct {
	// mu owns the installed activation relation, its exact runtime revision,
	// and the monotonically increasing completion serial. A solverRuntime is
	// only an operational realization and cannot retain activation authority.
	mu         sync.Mutex
	runtime    *solverRuntime
	compiler   coldSolverCompiler
	assembly   *Assembly
	accepted   []equation.AcceptedMember
	revision   uint64
	completion uint64
}

// NewComposition opens one cold, empty declaration authority.
func NewComposition() *Composition {
	return &Composition{
		phase:     compositionFactors,
		semantics: make(map[SemanticKey]struct{}),
	}
}

// Sealed reports whether this exact cold composition has completed its
// one-shot seal. It does not imply that a Program body has been compiled or a
// Solver exists.
func (composition *Composition) Sealed() bool {
	return composition != nil && composition.phase == compositionSealed && composition.sealed != nil && composition.id.Available() && composition.sealAuthority != 0
}

// ID returns the opaque canonical identity derived at Seal. The zero value
// means this Composition has not sealed successfully.
func (composition *Composition) ID() CompositionID {
	if composition == nil || !composition.Sealed() {
		return CompositionID{}
	}
	return composition.id
}

// RuleAdmissionInventory returns the complete canonical admission provenance
// of a sealed Composition. It is intentionally derived from the one sealed
// cold schema, never from typed callback closures or declaration order. The
// report is the sole Wave-D gate surface for distinguishing named trusted TCB
// theorems from locally checked derivations.
func (composition *Composition) RuleAdmissionInventory() (RuleAdmissionInventory, bool) {
	if composition == nil || !composition.Sealed() {
		return RuleAdmissionInventory{}, false
	}
	rules := composition.sealed.Rules()
	report := RuleAdmissionInventory{ID: composition.id, Rules: make([]RuleAdmissionRecord, len(rules))}
	for index, rule := range rules {
		basis, known := ruleAdmissionBasisFromCold(rule.Admission.Kind)
		if !known || !rule.Key.Available() || !rule.Admission.Identity.Available() {
			return RuleAdmissionInventory{}, false
		}
		report.Rules[index] = RuleAdmissionRecord{
			Rule:     semanticKeyFromComposition(rule.Key),
			Basis:    basis,
			Identity: semanticKeyFromComposition(rule.Admission.Identity),
		}
	}
	return report, true
}

func ruleAdmissionBasisFromCold(kind coldcomposition.AdmissionKind) (RuleAdmissionBasis, bool) {
	switch kind {
	case coldcomposition.AdmissionTrustedTheorem:
		return RuleAdmissionBasisTrustedTheorem, true
	case coldcomposition.AdmissionDerivation:
		return RuleAdmissionBasisDerivation, true
	default:
		return RuleAdmissionBasisInvalid, false
	}
}

// Seal closes cold declaration only. It validates that all Factors preceded
// every child and that every retained Rule and Query shape closed under the
// same Composition. It deliberately does not allocate carrier storage,
// materialize coordinates, create a guard manager, compile an equation, or
// select a query cone.
func (composition *Composition) Seal() bool {
	if composition == nil || composition.phase == compositionPoisoned || composition.phase == compositionSealed || len(composition.rules) == 0 || len(composition.queries) == 0 {
		if composition != nil && composition.phase != compositionSealed {
			composition.poison()
		}
		return false
	}
	if !validColdSupportCompletion(composition, composition.completion) {
		composition.poison()
		return false
	}
	for _, family := range composition.activations {
		if !validColdActivationFamily(composition, family) {
			composition.poison()
			return false
		}
	}
	for _, factor := range composition.factors {
		if !validColdFactor(composition, factor) {
			composition.poison()
			return false
		}
	}
	for _, rule := range composition.rules {
		if !validColdRule(composition, rule) {
			composition.poison()
			return false
		}
	}
	if !allActivationFamiliesBound(composition) {
		composition.poison()
		return false
	}
	for _, query := range composition.queries {
		if !validColdQuery(composition, query) {
			composition.poison()
			return false
		}
	}
	if len(composition.factors) == 0 && !validFactorFreeStructuralComposition(composition) {
		composition.poison()
		return false
	}
	sealed, ok := composition.deriveColdComposition()
	if !ok || sealed == nil {
		composition.poison()
		return false
	}
	if !composition.bindSchemas(sealed) {
		composition.poison()
		return false
	}
	sealedID := sealed.ID()
	var digest [32]byte
	copy(digest[:], sealedID[:])
	authority := nextSealAuthority.Add(1)
	if authority == 0 || composition.sealAuthority != 0 {
		composition.poison()
		return false
	}
	composition.sealed = sealed
	composition.id = CompositionID{digest: digest}
	composition.sealAuthority = authority
	composition.phase = compositionSealed
	return true
}

// allActivationFamiliesBound rejects cold predicate authorities that no
// structural trigger can invoke. A family is declared for its exact trigger
// set, never as a spare registry entry for later topology repair.
func allActivationFamiliesBound(composition *Composition) bool {
	if composition == nil {
		return false
	}
	bound := make(map[*activationFamilySchema]struct{}, len(composition.activations))
	for _, rule := range composition.rules {
		if rule != nil && rule.activation != nil && rule.activation.family != nil {
			bound[rule.activation.family] = struct{}{}
		}
	}
	for _, family := range composition.activations {
		if _, found := bound[family]; !found {
			return false
		}
	}
	return true
}

// validFactorFreeStructuralComposition admits the one zero-width carrier
// case: a support-only structural relation.  It has no Factor declaration,
// incidence, or slot, but it remains in the same sealed Composition and
// equation pipeline as every other Rule.  Activation is deliberately not
// admitted here: an empty Factor vocabulary must not become a second generic
// structural execution language.
func validFactorFreeStructuralComposition(composition *Composition) bool {
	if composition == nil || len(composition.factors) != 0 || len(composition.activations) != 0 || len(composition.rules) == 0 || len(composition.queries) == 0 || !validColdSupportCompletion(composition, composition.completion) || composition.completion == nil {
		return false
	}
	for _, rule := range composition.rules {
		if !validColdSupportRule(composition, rule) || len(rule.reads) != 0 {
			return false
		}
	}
	for _, query := range composition.queries {
		if query.schema == nil || !query.schema.support || len(query.schema.reads) != 0 {
			return false
		}
	}
	return true
}

// bindSchemas assigns each private typed binder its canonical index. Input
// slices remain declaration-order storage only; every index comes from the
// one sealed semantic authority.
func (composition *Composition) bindSchemas(sealed *coldcomposition.Composition) bool {
	if composition == nil || sealed == nil {
		return false
	}
	factors, rules, queries := sealed.Factors(), sealed.Rules(), sealed.Queries()
	rulesByIndex := make([]*ruleSchema, len(rules))
	for _, factor := range composition.factors {
		if !validFactorBind(factor) || factor.bound {
			return false
		}
		index, ok := sealed.FactorIndex(factor.semantic.compositionKey())
		if !ok || index >= uint64(len(factors)) || factors[index].Key != factor.semantic.compositionKey() {
			return false
		}
		factor.bindIndex, factor.bound = index, true
	}
	for _, rule := range composition.rules {
		if !validRuleBind(rule) || rule.bound {
			return false
		}
		index, ok := sealed.RuleIndex(rule.semantic.compositionKey())
		if !ok || index >= uint64(len(rules)) || rules[index].Key != rule.semantic.compositionKey() || rulesByIndex[index] != nil {
			return false
		}
		rule.bindIndex, rule.bound = index, true
		rulesByIndex[index] = rule
	}
	for _, query := range composition.queries {
		schema := query.schema
		if !validQueryBind(schema) || schema.bound || schema.authority != nil {
			return false
		}
		index, ok := sealed.QueryIndex(schema.semantic.compositionKey())
		if !ok || index >= uint64(len(queries)) || queries[index].Key != schema.semantic.compositionKey() {
			return false
		}
		schema.bindIndex, schema.bound = index, true
		schema.authority = &queryAuthority{schema: schema, index: index}
		if schema.authority.schema != schema || schema.authority.index != schema.bindIndex {
			return false
		}
	}
	for _, rule := range rulesByIndex {
		if rule == nil {
			return false
		}
	}
	composition.rulesByIndex = rulesByIndex
	return true
}

func (composition *Composition) ruleAt(key coldcomposition.Key) (*ruleSchema, bool) {
	if composition == nil || !composition.Sealed() || composition.sealed == nil || !key.Available() {
		return nil, false
	}
	index, ok := composition.sealed.RuleIndex(key)
	if !ok || index >= uint64(len(composition.rulesByIndex)) {
		return nil, false
	}
	rule := composition.rulesByIndex[index]
	return rule, rule != nil && rule.bound && rule.bindIndex == index && rule.semantic.compositionKey() == key && validColdRule(composition, rule)
}

// deriveColdComposition is the sole lowering from typed public declarations
// to the immutable canonical cold schema. It derives Factor incidence, SCC
// reports, and CompositionID inside internal/composition; it never creates a
// Point, Action, Unit, Target, carrier, guard, or equation instance.
func (composition *Composition) deriveColdComposition() (*coldcomposition.Composition, bool) {
	if composition == nil {
		return nil, false
	}
	candidate := coldcomposition.Candidate{
		Factors:            make([]coldcomposition.Factor, len(composition.factors)),
		ActivationFamilies: make([]coldcomposition.ActivationFamily, len(composition.activations)),
		Rules:              make([]coldcomposition.Rule, len(composition.rules)),
		Queries:            make([]coldcomposition.QueryFamily, len(composition.queries)),
	}
	if completion := composition.completion; completion != nil {
		if !validColdSupportCompletion(composition, completion) {
			return nil, false
		}
		candidate.Completion = coldcomposition.Completion{
			Semantic: completion.semantic.compositionKey(),
			Prune:    completion.prune.semantic.compositionKey(),
		}
	}
	for index, factor := range composition.factors {
		if factor == nil || !factor.semantic.Available() {
			return nil, false
		}
		derived, ok := deriveColdFactor(factor)
		if !ok {
			return nil, false
		}
		candidate.Factors[index] = derived
	}
	for index, family := range composition.activations {
		derived, ok := deriveColdActivationFamily(composition, family)
		if !ok {
			return nil, false
		}
		candidate.ActivationFamilies[index] = derived
	}
	for index, rule := range composition.rules {
		derived, ok := deriveColdRule(rule)
		if !ok {
			return nil, false
		}
		candidate.Rules[index] = derived
	}
	for index, query := range composition.queries {
		derived, ok := deriveColdQuery(query)
		if !ok {
			return nil, false
		}
		candidate.Queries[index] = derived
	}
	return coldcomposition.Seal(candidate)
}

func deriveColdFactor(factor *factorSchema) (coldcomposition.Factor, bool) {
	if factor == nil || !factor.semantic.Available() {
		return coldcomposition.Factor{}, false
	}
	derived := coldcomposition.Factor{Key: factor.semantic.compositionKey()}
	for _, form := range factor.forms {
		if form == nil || !form.semantic.Available() || form.factor != factor {
			return coldcomposition.Factor{}, false
		}
		switch {
		case form == factor.exactRead && form.readKind == exactReadForm && form.writeKind == 0 && form.semantic == factor.semantic:
		case form == factor.exactWrite && form.readKind == 0 && form.writeKind == exactWriteForm && form.semantic == factor.semantic:
		case form.readKind == summaryReadForm && form.writeKind == 0 && form.semantic != factor.semantic:
			derived.Forms = append(derived.Forms, coldcomposition.FactorForm{Kind: coldcomposition.FactorSummaryRead, Semantic: form.semantic.compositionKey()})
		case form.readKind == 0 && form.writeKind == selectorWriteForm && form.semantic != factor.semantic:
			derived.Forms = append(derived.Forms, coldcomposition.FactorForm{Kind: coldcomposition.FactorSelectorWrite, Semantic: form.semantic.compositionKey()})
		default:
			return coldcomposition.Factor{}, false
		}
	}
	return derived, true
}

func deriveColdRule(rule *ruleSchema) (coldcomposition.Rule, bool) {
	if rule == nil || !rule.semantic.Available() {
		return coldcomposition.Rule{}, false
	}
	if !rule.admission.valid() {
		return coldcomposition.Rule{}, false
	}
	admission, admitted := deriveColdAdmission(rule.admission)
	if !admitted {
		return coldcomposition.Rule{}, false
	}
	if rule.outputKind == ruleStructuralOutput {
		if validColdActivationRule(rule.composition, rule) {
			reads, ok := deriveColdReads(rule)
			if !ok || rule.activation == nil || rule.activation.family == nil {
				return coldcomposition.Rule{}, false
			}
			return coldcomposition.Rule{
				Key:           rule.semantic.compositionKey(),
				OperandFamily: rule.operandFamily.compositionKey(),
				Admission:     admission,
				OutputKind:    coldcomposition.StructuralOutput,
				Inputs:        uint64(rule.inputs),
				Reads:         reads,
				Activations:   []coldcomposition.ActivationRange{{Family: rule.activation.family.semantic.compositionKey()}},
			}, true
		}
		if !validColdSupportRule(rule.composition, rule) {
			return coldcomposition.Rule{}, false
		}
		reads, ok := deriveColdReads(rule)
		if !ok {
			return coldcomposition.Rule{}, false
		}
		return coldcomposition.Rule{
			Key:           rule.semantic.compositionKey(),
			OperandFamily: rule.operandFamily.compositionKey(),
			Admission:     admission,
			OutputKind:    coldcomposition.StructuralOutput,
			Inputs:        uint64(rule.inputs),
			Reads:         reads,
			Supports: []coldcomposition.Support{{
				Semantic: rule.support.completion.semantic.compositionKey(),
			}},
			Prunes: []coldcomposition.Prune{{
				Semantic: rule.support.prune.semantic.compositionKey(),
			}},
		}, true
	}
	if rule.outputKind != ruleFactorOutput || rule.support != nil || rule.activation != nil || rule.output == nil || !rule.output.semantic.Available() || rule.inputs < 0 {
		return coldcomposition.Rule{}, false
	}
	reads, ok := deriveColdReads(rule)
	if !ok {
		return coldcomposition.Rule{}, false
	}
	derived := coldcomposition.Rule{
		Key:           rule.semantic.compositionKey(),
		OperandFamily: rule.operandFamily.compositionKey(),
		Admission:     admission,
		OutputKind:    coldcomposition.FactorOutput,
		Output:        rule.output.semantic.compositionKey(),
		Inputs:        uint64(rule.inputs),
		Reads:         reads,
		Carries:       make([]coldcomposition.Carry, len(rule.carries)),
		Writes:        make([]coldcomposition.Write, len(rule.writes)),
	}
	writeSelectors := make(map[int]*coldWriteSelector, len(rule.writeSelectors))
	for index := range rule.writeSelectors {
		selector := &rule.writeSelectors[index]
		if _, duplicate := writeSelectors[selector.write]; duplicate {
			return coldcomposition.Rule{}, false
		}
		writeSelectors[selector.write] = selector
	}
	for index, carry := range rule.carries {
		if carry.factor == nil || !carry.factor.semantic.Available() {
			return coldcomposition.Rule{}, false
		}
		derived.Carries[index] = coldcomposition.Carry{Input: uint64(carry.input), Factor: carry.factor.semantic.compositionKey(), Transform: carry.transform.compositionKey()}
	}
	for index, write := range rule.writes {
		if write.form == nil || write.form.factor == nil || !write.form.factor.semantic.Available() {
			return coldcomposition.Rule{}, false
		}
		row := coldcomposition.Write{Factor: write.form.factor.semantic.compositionKey()}
		switch write.form.writeKind {
		case exactWriteForm:
			if write.route != 0 {
				if write.route-1 >= uint64(len(rule.reads)) || len(write.dependencies) != 0 {
					return coldcomposition.Rule{}, false
				}
				row.Kind = coldcomposition.WriteRoute
				row.Route = write.route
				break
			}
			if write.route != 0 || len(write.dependencies) != 0 {
				return coldcomposition.Rule{}, false
			}
			row.Kind = coldcomposition.WriteExact
		case selectorWriteForm:
			selector, exists := writeSelectors[index]
			if write.route != 0 || !exists || !write.form.semantic.Available() || !sameDependencies(write.dependencies, selector.depends) {
				return coldcomposition.Rule{}, false
			}
			candidates, ok := coldReadCandidates(selector.candidates)
			if !ok {
				return coldcomposition.Rule{}, false
			}
			dependencies, ok := coldWriteDependencies(selector.depends)
			if !ok {
				return coldcomposition.Rule{}, false
			}
			row.Kind = coldcomposition.WriteSelect
			row.Semantic = write.form.semantic.compositionKey()
			row.Candidates = candidates
			row.Dependencies = dependencies
		default:
			return coldcomposition.Rule{}, false
		}
		derived.Writes[index] = row
	}
	return derived, true
}

func deriveColdActivationFamily(composition *Composition, family *activationFamilySchema) (coldcomposition.ActivationFamily, bool) {
	if !validColdActivationFamily(composition, family) {
		return coldcomposition.ActivationFamily{}, false
	}
	return family.cold, true
}

func deriveColdAdmission(admission ruleAdmissionSchema) (coldcomposition.Admission, bool) {
	if !admission.identity.Available() {
		return coldcomposition.Admission{}, false
	}
	switch admission.kind {
	case ruleAdmissionTrustedTheorem:
		return coldcomposition.Admission{Kind: coldcomposition.AdmissionTrustedTheorem, Identity: admission.identity.compositionKey()}, true
	case ruleAdmissionDerivation:
		return coldcomposition.Admission{Kind: coldcomposition.AdmissionDerivation, Identity: admission.identity.compositionKey()}, true
	default:
		return coldcomposition.Admission{}, false
	}
}

func deriveColdReads(rule *ruleSchema) ([]coldcomposition.Read, bool) {
	if rule == nil {
		return nil, false
	}
	result := make([]coldcomposition.Read, len(rule.reads))
	readSelectors := make(map[int]*coldReadSelector, len(rule.readSelectors))
	for index := range rule.readSelectors {
		selector := &rule.readSelectors[index]
		if _, duplicate := readSelectors[selector.read]; duplicate {
			return nil, false
		}
		readSelectors[selector.read] = selector
	}
	for index, read := range rule.reads {
		if read.form == nil || read.form.factor == nil || !read.form.factor.semantic.Available() {
			return nil, false
		}
		row := coldcomposition.Read{Input: uint64(read.input), Factor: read.form.factor.semantic.compositionKey()}
		if len(read.depends) != 0 {
			selector, exists := readSelectors[index]
			if !exists || selector.bind == nil || read.form.readKind != exactReadForm || !sameDependencies(read.depends, selector.depends) {
				return nil, false
			}
			dependencies, ok := coldReadDependencies(selector.depends)
			if !ok {
				return nil, false
			}
			row.Kind = coldcomposition.ReadSelect
			row.Semantic = row.Factor
			row.Dependencies = dependencies
			result[index] = row
			continue
		}
		if _, selected := readSelectors[index]; selected {
			return nil, false
		}
		switch read.form.readKind {
		case exactReadForm:
			row.Kind = coldcomposition.ReadExact
		case summaryReadForm:
			row.Kind = coldcomposition.ReadSummary
			row.Semantic = read.form.semantic.compositionKey()
			row.Normalizer = row.Semantic
		default:
			return nil, false
		}
		result[index] = row
	}
	return result, true
}

func deriveColdQuery(query coldQuery) (coldcomposition.QueryFamily, bool) {
	if query.schema == nil || !query.schema.semantic.Available() || !query.schema.freezer.Available() {
		return coldcomposition.QueryFamily{}, false
	}
	derived := coldcomposition.QueryFamily{Key: query.schema.semantic.compositionKey(), Freezer: query.schema.freezer.compositionKey()}
	if query.schema.support {
		if len(query.schema.reads) != 0 {
			return coldcomposition.QueryFamily{}, false
		}
		derived.Projections = []coldcomposition.QueryProjection{{Kind: coldcomposition.QuerySupport}}
		return derived, true
	}
	if len(query.schema.reads) == 0 {
		return coldcomposition.QueryFamily{}, false
	}
	derived.Projections = make([]coldcomposition.QueryProjection, len(query.schema.reads))
	for index, read := range query.schema.reads {
		if read.form == nil || read.form.factor == nil || !read.form.factor.semantic.Available() {
			return coldcomposition.QueryFamily{}, false
		}
		switch read.form.readKind {
		case exactReadForm:
			derived.Projections[index] = coldcomposition.QueryProjection{Kind: coldcomposition.QueryFactorExact, Factor: read.form.factor.semantic.compositionKey()}
		case summaryReadForm:
			if !read.form.semantic.Available() {
				return coldcomposition.QueryFamily{}, false
			}
			derived.Projections[index] = coldcomposition.QueryProjection{Kind: coldcomposition.QueryFactorSummary, Factor: read.form.factor.semantic.compositionKey(), Normalizer: read.form.semantic.compositionKey()}
		default:
			return coldcomposition.QueryFamily{}, false
		}
	}
	return derived, true
}

func coldReadDependencies(dependencies []Dependency) ([]uint64, bool) {
	result := make([]uint64, len(dependencies))
	for index, dependency := range dependencies {
		if dependency.kind != readDependency || dependency.index < 0 {
			return nil, false
		}
		result[index] = uint64(dependency.index)
	}
	return result, true
}

func coldReadCandidates(candidates []int) ([]uint64, bool) {
	result := make([]uint64, len(candidates))
	for index, candidate := range candidates {
		if candidate < 0 {
			return nil, false
		}
		result[index] = uint64(candidate)
	}
	return result, true
}

func coldWriteDependencies(dependencies []Dependency) ([]coldcomposition.Dependency, bool) {
	result := make([]coldcomposition.Dependency, len(dependencies))
	for index, dependency := range dependencies {
		if dependency.index < 0 || dependency.kind != readDependency && dependency.kind != writeDependency {
			return nil, false
		}
		result[index] = coldcomposition.Dependency{Target: dependency.kind == writeDependency, Index: uint64(dependency.index)}
	}
	return result, true
}

// coldComposition returns the derived immutable composition for Wave E. It
// remains package-private so no caller can use internal composition rows as a
// second public declaration language.
func (composition *Composition) coldComposition() *coldcomposition.Composition {
	if !composition.Sealed() {
		return nil
	}
	return composition.sealed
}

func (composition *Composition) usable() bool {
	return composition != nil && (composition.phase == compositionFactors || composition.phase == compositionChildren)
}

// available admits both an open declaration and an already sealed cold
// schema. E consumes capabilities from the latter but must never mutate it.
func (composition *Composition) available() bool {
	return composition != nil && (composition.usable() || composition.phase == compositionSealed)
}

func (composition *Composition) acceptsFactor(key SemanticKey) bool {
	if composition == nil || composition.phase != compositionFactors {
		return false
	}
	if composition.activeFactor != nil {
		composition.poison()
		return false
	}
	return composition.claim(key)
}

func (composition *Composition) acceptsChild(key SemanticKey) bool {
	if composition == nil || composition.phase == compositionPoisoned || composition.phase == compositionSealed {
		return false
	}
	if composition.activeFactor != nil {
		composition.poison()
		return false
	}
	if composition.phase == compositionFactors {
		composition.phase = compositionChildren
	}
	return composition.phase == compositionChildren && composition.claim(key)
}

func (composition *Composition) claim(key SemanticKey) bool {
	if composition == nil || !key.Available() || composition.semantics == nil {
		return false
	}
	if _, duplicate := composition.semantics[key]; duplicate {
		return false
	}
	composition.semantics[key] = struct{}{}
	return true
}

func (composition *Composition) poison() {
	if composition != nil && composition.phase != compositionSealed {
		composition.phase = compositionPoisoned
	}
}

func sameComposition(left, right *Composition) bool {
	return left != nil && left == right && left.available()
}
