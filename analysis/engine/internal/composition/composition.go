// Package composition seals the cold Wave-D domain schema.  It deliberately
// knows no concrete Rule instance, point, unit, target, callback, carrier, or
// schedule.  Equation topology is the later Wave-E owner of those concerns.
package composition

import (
	"context"
	"crypto/sha256"
	"reflect"
	"sort"

	"github.com/wippyai/go-lua/analysis/schema/population"
	"github.com/wippyai/go-lua/internal/canonical"
)

// codecVersion changes whenever a cold semantic term changes meaning. Rule
// admission is no longer a cold schema term, so previous CompositionIDs must
// not be reused for the reduced declaration grammar.
const codecVersion = 18

// ID is a fixed semantic digest.  It is not a Program artifact identity.
type ID [sha256.Size]byte

func (id ID) Available() bool { return id != ID{} }

// Key is a semantic, versioned declaration identity.
type Key struct {
	ID      ID
	Version uint64
}

func (key Key) Available() bool { return key.ID.Available() && key.Version != 0 }

type ReadKind uint8

const (
	ReadExact ReadKind = iota + 1
	ReadSummary
	ReadSelect
)

type WriteKind uint8

const (
	WriteExact WriteKind = iota + 1
	// WriteRoute consumes one preceding ReadSelect.  The concrete exact
	// target comes only from that selected route at execution; it has neither
	// a static candidate vector nor a second target language.
	WriteRoute
)

// OutputKind is the exhaustive owner disposition of one Rule.  A Factor
// output owns one Factor patch (and, if declared, that same Factor's carry).
// A structural output owns no Factor patch or carry; it instead changes
// structural support through a declared prune or activation capability.
//
// Output is meaningful only for FactorOutput.  Keeping the tag separate from
// the payload makes a zero Key an invalid factor result rather than an
// overloaded spelling for a structural Rule.
type OutputKind uint8

const (
	OutputInvalid OutputKind = iota
	FactorOutput
	StructuralOutput
)

// Read retains a dependency form and the preceding read DAG for a staged
// exact selection. A staged selector has no cold candidate vector: it chooses
// only owner-issued exact units from completed predecessor observations at
// runtime. It never contains a resolved Factor unit or local capability.
type Read struct {
	Kind         ReadKind
	Input        uint64
	Factor       Key
	Semantic     Key
	Normalizer   Key
	Dependencies []uint64
}

// Carry is the schema-level whole-output relation.
type Carry struct {
	Input     uint64
	Factor    Key
	Transform Key
}

// Write retains the output Factor and write form. Concrete targets and their
// local identities belong to equation instances.
type Write struct {
	Kind   WriteKind
	Factor Key
	// Route is the one-based ordinal of the preceding ReadSelect consumed by
	// WriteRoute.  It is zero for every other write kind.
	Route uint64
}

// Support and Prune retain only their Composition/Solver structural-law
// identities. Their exact structural coordinates are resolved by the later
// equation compiler; neither row can carry a Factor coordinate.
type Support struct{ Semantic Key }

type Prune struct{ Semantic Key }

// Completion is the optional immutable Composition/Solver support
// completion/prune pair. It is declared independently of a support Rule so
// the cold schema retains the registered structural projection even when no
// Rule currently refines it.
type Completion struct {
	Semantic Key
	Prune    Key
}

// ActivationFamily is the one cold authority for an activation predicate and
// its axis-qualified descriptor schema. Topology supplies independent axis
// values and tuple issuance only; it cannot author a second predicate.
type ActivationFamily struct {
	Semantic Key
}

// ActivationRange is the structural effect declared by one output-free Rule.
// It names exactly one cold family permission. Concrete locator membership is
// checked only when equation Topology seals an owner-issued activation row.
type ActivationRange struct {
	Family Key
}

// FactorFormKind names one optional owner-declared form beyond the Factor's
// intrinsic exact read/write pair.  It is cold semantic schema, not a
// runtime handle: declarations must survive sealing even before a Rule or
// Query happens to consume them.
type FactorFormKind uint8

const (
	FactorSummaryRead FactorFormKind = iota + 1
	// FactorDistributiveSummaryRead is a summary read whose reader folds each
	// declared coordinate independently. The fold belongs to the form because
	// it decides how the carrier partitions the declared vector; it is never a
	// property of a Rule, Query, or observation call.
	FactorDistributiveSummaryRead
)

// FactorForm records one Factor-owned extension form. Semantic is globally
// authored identity; Kind prevents a summary normalizer from being replayed
// as a selector-write law.
type FactorForm struct {
	Kind     FactorFormKind
	Semantic Key
}

// Scalar shape projections are the only child-schema inspection surface used
// by hot binding. They intentionally contain no slices or mutable row storage.
type FactorFormShape struct {
	Kind     FactorFormKind
	Semantic Key
}

type RuleShape struct {
	OperandFamily    Key
	OutputKind       OutputKind
	Output           Key
	Inputs           uint64
	ReadCount        uint64
	CarryCount       uint64
	WriteCount       uint64
	SupportCount     uint64
	PruneCount       uint64
	ActivationCount  uint64
	ActivationFamily Key
}

type RuleReadShape struct {
	Kind            ReadKind
	Input           uint64
	Factor          Key
	Semantic        Key
	Normalizer      Key
	DependencyCount uint64
}

type RuleCarryShape struct {
	Input     uint64
	Factor    Key
	Transform Key
}

type RuleWriteShape struct {
	Kind   WriteKind
	Factor Key
	Route  uint64
}

// RuleSupportShape is the scalar structural-support projection of one
// sealed Rule.  It deliberately exposes no borrowed slice or row storage.
type RuleSupportShape struct {
	Semantic Key
}

// RulePruneShape is the scalar structural-prune projection of one sealed
// Rule.  It deliberately exposes no borrowed slice or row storage.
type RulePruneShape struct {
	Semantic Key
}

// PopulationKind is the resolved query population carried by a sealed
// family. It aliases the schema-neutral leaf so the declaration surface
// remains an ingress vocabulary owner rather than an engine dependency.
type PopulationKind = population.Kind

const (
	PopulationKindInvalid       = population.Invalid
	PopulationKindSelectedPoint = population.SelectedPoint
	PopulationKindObservation   = population.Observation
)

// QueryShape is the scalar, immutable header of one sealed Query family.
// Hot binding uses this projection instead of detaching the cold Query row.
type QueryShape struct {
	Freezer         Key
	Population      population.Kind
	ProjectionCount uint64
}

// QueryProjectionShape is one scalar projection in the sealed Query's
// canonical order. It contains no callback or mutable row storage.
type QueryProjectionShape struct {
	Kind       QueryProjectionKind
	Factor     Key
	Normalizer Key
}

type Factor struct {
	Key   Key
	Forms []FactorForm
}

// Rule is an unanchored domain Rule schema.  Its occurrences and concrete
// read/write capabilities are intentionally absent.
type Rule struct {
	Key           Key
	OperandFamily Key
	OutputKind    OutputKind
	Output        Key
	Inputs        uint64
	Reads         []Read
	Carries       []Carry
	Writes        []Write
	Supports      []Support
	Prunes        []Prune
	Activations   []ActivationRange
}

// QueryFamily is a domain-level observation family.  Freezer is the required
// semantic/versioned Freeze/Clone/Equal/Fingerprint law identity; Point,
// source callback and resolved unit are equation-instance data.
type QueryFamily struct {
	Key         Key
	Freezer     Key
	Population  population.Kind
	Projections []QueryProjection
}

type QueryProjectionKind uint8

const (
	QueryFactorExact QueryProjectionKind = iota + 1
	QueryFactorSummary
	QuerySupport
)

// QueryProjection carries a projection family/law, never a concrete unit.
type QueryProjection struct {
	Kind       QueryProjectionKind
	Factor     Key
	Normalizer Key
}

type Candidate struct {
	Factors            []Factor
	Completion         Completion
	ActivationFamilies []ActivationFamily
	Rules              []Rule
	Queries            []QueryFamily
}

// Incidence is the derived Factor dependency report.
type Incidence struct{ Read, Write Key }

type Component struct {
	Factors    []Key
	Successors []Key
}

// Composition is the one immutable cold authority for domain schemas and
// their derived Factor graph.  It has no execution topology.
type Composition struct {
	id           ID
	factors      []Factor
	rules        []Rule
	queries      []QueryFamily
	completion   Completion
	activations  []ActivationFamily
	activationAt map[Key]ActivationFamily
	factorAt     map[Key]uint64
	ruleAt       map[Key]uint64
	queryAt      map[Key]uint64
	incidence    []Incidence
	reverse      map[Key][]Key
	components   []Component
}

// Same reports exact canonical cold-schema equality. ID equality is required
// but never sufficient on its own: a hot binding must reject a digest
// collision rather than treating it as schema authority. Only the canonical
// semantic rows participate; lookup and incidence indexes are derived.
func Same(left, right *Composition) bool {
	return left != nil && right != nil && left.id.Available() && left.id == right.id &&
		reflect.DeepEqual(left.factors, right.factors) &&
		reflect.DeepEqual(left.completion, right.completion) &&
		reflect.DeepEqual(left.activations, right.activations) &&
		reflect.DeepEqual(left.rules, right.rules) &&
		reflect.DeepEqual(left.queries, right.queries)
}

func (c *Composition) ID() ID {
	if c == nil {
		return ID{}
	}
	return c.id
}
func (c *Composition) FactorIndex(key Key) (uint64, bool) {
	if c == nil {
		return 0, false
	}
	value, ok := c.factorAt[key]
	return value, ok
}
func (c *Composition) FactorKeyAt(index uint64) Key {
	if c == nil || index >= uint64(len(c.factors)) {
		return Key{}
	}
	return c.factors[index].Key
}
func (c *Composition) RuleIndex(key Key) (uint64, bool) {
	if c == nil {
		return 0, false
	}
	value, ok := c.ruleAt[key]
	return value, ok
}
func (c *Composition) RuleKeyAt(index uint64) Key {
	if c == nil || index >= uint64(len(c.rules)) {
		return Key{}
	}
	return c.rules[index].Key
}
func (c *Composition) QueryIndex(key Key) (uint64, bool) {
	if c == nil {
		return 0, false
	}
	value, ok := c.queryAt[key]
	return value, ok
}
func (c *Composition) QueryKeyAt(index uint64) Key {
	if c == nil || index >= uint64(len(c.queries)) {
		return Key{}
	}
	return c.queries[index].Key
}
func (c *Composition) ActivationIndex(key Key) (uint64, bool) {
	if c == nil {
		return 0, false
	}
	for index, family := range c.activations {
		if family.Semantic == key {
			return uint64(index), true
		}
	}
	return 0, false
}
func (c *Composition) Factors() []Factor {
	if c == nil {
		return nil
	}
	return copyFactors(c.factors)
}
func (c *Composition) Rules() []Rule          { return copyRules(c.rules) }
func (c *Composition) Queries() []QueryFamily { return copyQueries(c.queries) }

// ShapeCount returns the immutable cold denominators without detaching any
// semantic row. It is the narrow proof used by the hot Binding planner.
func (c *Composition) ShapeCount() (factors, rules, queries, activations int, ok bool) {
	if c == nil || !c.id.Available() {
		return 0, 0, 0, 0, false
	}
	return len(c.factors), len(c.rules), len(c.queries), len(c.activations), true
}

// FactorFormCount returns only one canonical Factor's extension-form
// denominator. It exposes no row, semantic identity, or mutable storage.
func (c *Composition) FactorFormCount(index uint64) (int, bool) {
	if c == nil || index >= uint64(len(c.factors)) {
		return 0, false
	}
	return len(c.factors[index].Forms), true
}

func (c *Composition) FactorFormShapeAt(factor, form uint64) (FactorFormShape, bool) {
	if c == nil || factor >= uint64(len(c.factors)) || form >= uint64(len(c.factors[factor].Forms)) {
		return FactorFormShape{}, false
	}
	row := c.factors[factor].Forms[form]
	return FactorFormShape{Kind: row.Kind, Semantic: row.Semantic}, true
}

func (c *Composition) RuleShapeAt(index uint64) (RuleShape, bool) {
	if c == nil || index >= uint64(len(c.rules)) {
		return RuleShape{}, false
	}
	row := c.rules[index]
	activationFamily := Key{}
	if len(row.Activations) == 1 {
		activationFamily = row.Activations[0].Family
	}
	return RuleShape{OperandFamily: row.OperandFamily, OutputKind: row.OutputKind, Output: row.Output, Inputs: row.Inputs, ReadCount: uint64(len(row.Reads)), CarryCount: uint64(len(row.Carries)), WriteCount: uint64(len(row.Writes)), SupportCount: uint64(len(row.Supports)), PruneCount: uint64(len(row.Prunes)), ActivationCount: uint64(len(row.Activations)), ActivationFamily: activationFamily}, true
}

func (c *Composition) RuleReadShapeAt(rule, read uint64) (RuleReadShape, bool) {
	if c == nil || rule >= uint64(len(c.rules)) || read >= uint64(len(c.rules[rule].Reads)) {
		return RuleReadShape{}, false
	}
	row := c.rules[rule].Reads[read]
	return RuleReadShape{Kind: row.Kind, Input: row.Input, Factor: row.Factor, Semantic: row.Semantic, Normalizer: row.Normalizer, DependencyCount: uint64(len(row.Dependencies))}, true
}

func (c *Composition) RuleReadDependencyAt(rule, read, dependency uint64) (uint64, bool) {
	if c == nil || rule >= uint64(len(c.rules)) || read >= uint64(len(c.rules[rule].Reads)) || dependency >= uint64(len(c.rules[rule].Reads[read].Dependencies)) {
		return 0, false
	}
	return c.rules[rule].Reads[read].Dependencies[dependency], true
}

func (c *Composition) RuleCarryShapeAt(rule, carry uint64) (RuleCarryShape, bool) {
	if c == nil || rule >= uint64(len(c.rules)) || carry >= uint64(len(c.rules[rule].Carries)) {
		return RuleCarryShape{}, false
	}
	row := c.rules[rule].Carries[carry]
	return RuleCarryShape{Input: row.Input, Factor: row.Factor, Transform: row.Transform}, true
}

func (c *Composition) RuleWriteShapeAt(rule, write uint64) (RuleWriteShape, bool) {
	if c == nil || rule >= uint64(len(c.rules)) || write >= uint64(len(c.rules[rule].Writes)) {
		return RuleWriteShape{}, false
	}
	row := c.rules[rule].Writes[write]
	return RuleWriteShape{Kind: row.Kind, Factor: row.Factor, Route: row.Route}, true
}

func (c *Composition) RuleSupportShapeAt(rule, support uint64) (RuleSupportShape, bool) {
	if c == nil || rule >= uint64(len(c.rules)) || support >= uint64(len(c.rules[rule].Supports)) {
		return RuleSupportShape{}, false
	}
	return RuleSupportShape{Semantic: c.rules[rule].Supports[support].Semantic}, true
}

func (c *Composition) RulePruneShapeAt(rule, prune uint64) (RulePruneShape, bool) {
	if c == nil || rule >= uint64(len(c.rules)) || prune >= uint64(len(c.rules[rule].Prunes)) {
		return RulePruneShape{}, false
	}
	return RulePruneShape{Semantic: c.rules[rule].Prunes[prune].Semantic}, true
}

func (c *Composition) QueryShapeAt(index uint64) (QueryShape, bool) {
	if c == nil || index >= uint64(len(c.queries)) {
		return QueryShape{}, false
	}
	row := c.queries[index]
	return QueryShape{Freezer: row.Freezer, Population: row.Population, ProjectionCount: uint64(len(row.Projections))}, true
}

func (c *Composition) QueryProjectionShapeAt(query, projection uint64) (QueryProjectionShape, bool) {
	if c == nil || query >= uint64(len(c.queries)) || projection >= uint64(len(c.queries[query].Projections)) {
		return QueryProjectionShape{}, false
	}
	row := c.queries[query].Projections[projection]
	return QueryProjectionShape{Kind: row.Kind, Factor: row.Factor, Normalizer: row.Normalizer}, true
}

// RuleAt returns one detached sealed rule without cloning the entire catalog.
// Dense ordinal rule indexing remains owned by RuleIndex; callers cannot retain or
// mutate Composition storage through this projection.
func (c *Composition) RuleAt(index uint64) (Rule, bool) {
	if c == nil || index >= uint64(len(c.rules)) {
		return Rule{}, false
	}
	return copyRule(c.rules[index]), true
}

// ActivationFamily returns one immutable cold activation permission.
func (c *Composition) ActivationFamily(key Key) (ActivationFamily, bool) {
	if c == nil || !key.Available() {
		return ActivationFamily{}, false
	}
	family, ok := c.activationAt[key]
	if !ok {
		return ActivationFamily{}, false
	}
	return copyActivationFamily(family), true
}

func (c *Composition) ActivationFamilies() []ActivationFamily {
	if c == nil {
		return nil
	}
	result := make([]ActivationFamily, len(c.activations))
	for index := range c.activations {
		result[index] = copyActivationFamily(c.activations[index])
	}
	return result
}

// Completion returns the optional registered structural support-completion
// law. It is not a Factor surface or SCC member.
func (c *Composition) Completion() (Completion, bool) {
	if c == nil || !c.completion.Semantic.Available() {
		return Completion{}, false
	}
	return c.completion, true
}
func (c *Composition) Incidence() []Incidence {
	if c == nil {
		return nil
	}
	return append([]Incidence(nil), c.incidence...)
}
func (c *Composition) Reverse(key Key) []Key {
	if c == nil {
		return nil
	}
	return append([]Key(nil), c.reverse[key]...)
}
func (c *Composition) Components() []Component {
	if c == nil {
		return nil
	}
	result := make([]Component, len(c.components))
	for i, component := range c.components {
		result[i] = Component{Factors: append([]Key(nil), component.Factors...), Successors: append([]Key(nil), component.Successors...)}
	}
	return result
}

// Seal validates only the unanchored declaration grammar and derives its
// Factor graph.  Wave-E concrete topology is not admissible here.
func Seal(candidate Candidate) (*Composition, bool) {
	factors := copyFactors(candidate.Factors)
	activations := copyActivationFamilies(candidate.ActivationFamilies)
	rules := copyRules(candidate.Rules)
	queries := copyQueries(candidate.Queries)
	completion := candidate.Completion
	if !validCompletion(completion) {
		return nil, false
	}
	// This exactly mirrors the public Composition.claim authority for cold
	// declaration identities. A persisted Candidate must not admit a form or
	// other declared schema identity that no public Composition could declare.
	// Operand families and Factor references are excluded:
	// they are references under the public grammar and may lawfully repeat.
	if !validGlobalClaims(factors, completion, activations, rules, queries) {
		return nil, false
	}
	sort.Slice(factors, func(i, j int) bool { return lessKey(factors[i].Key, factors[j].Key) })
	factorAt := make(map[Key]uint64, len(factors))
	factorForms := make(map[Key]map[Key]FactorFormKind, len(factors))
	for i, factor := range factors {
		canonicalFactorForms(factor.Forms)
		if !validFactor(factor) || i > 0 && !lessKey(factors[i-1].Key, factor.Key) {
			return nil, false
		}
		factorAt[factor.Key] = uint64(i)
		forms := make(map[Key]FactorFormKind, len(factor.Forms))
		for _, form := range factor.Forms {
			forms[form.Semantic] = form.Kind
		}
		factorForms[factor.Key] = forms
	}
	sort.Slice(activations, func(i, j int) bool { return lessKey(activations[i].Semantic, activations[j].Semantic) })
	activationAt := make(map[Key]ActivationFamily, len(activations))
	for index := range activations {
		canonical, canonicalOK := CanonicalActivationFamily(activations[index])
		if !canonicalOK {
			return nil, false
		}
		activations[index] = canonical
		if !validActivationFamily(&activations[index]) || index > 0 && !lessKey(activations[index-1].Semantic, activations[index].Semantic) {
			return nil, false
		}
		activationAt[activations[index].Semantic] = copyActivationFamily(activations[index])
	}
	sort.Slice(rules, func(i, j int) bool { return lessKey(rules[i].Key, rules[j].Key) })
	ruleAt := make(map[Key]uint64, len(rules))
	for i := range rules {
		if !validRule(&rules[i], factorAt, factorForms, completion, activationAt) || i > 0 && !lessKey(rules[i-1].Key, rules[i].Key) {
			return nil, false
		}
		ruleAt[rules[i].Key] = uint64(i)
	}
	if !allActivationFamiliesReferenced(activations, rules) {
		return nil, false
	}
	sort.Slice(queries, func(i, j int) bool { return lessKey(queries[i].Key, queries[j].Key) })
	queryAt := make(map[Key]uint64, len(queries))
	for i, query := range queries {
		if !validQueryFamily(query, factorAt, factorForms, completion) || i > 0 && !lessKey(queries[i-1].Key, query.Key) {
			return nil, false
		}
		queryAt[query.Key] = uint64(i)
	}
	if len(factors) == 0 && !validFactorFreeStructuralCandidate(completion, activations, rules, queries) {
		return nil, false
	}
	incidence := deriveIncidence(rules)
	components, ok := canonicalComponents(factors, incidence)
	if !ok {
		return nil, false
	}
	id, ok := compositionID(factors, completion, activations, rules, queries)
	if !ok || !id.Available() {
		return nil, false
	}
	reverse := make(map[Key][]Key, len(factors))
	for _, edge := range incidence {
		reverse[edge.Write] = append(reverse[edge.Write], edge.Read)
	}
	for key, reads := range reverse {
		sort.Slice(reads, func(i, j int) bool { return lessKey(reads[i], reads[j]) })
		reverse[key] = reads
	}
	return &Composition{id: id, factors: factors, rules: rules, queries: queries, completion: completion, activations: activations, activationAt: activationAt, factorAt: factorAt, ruleAt: ruleAt, queryAt: queryAt, incidence: incidence, reverse: reverse, components: components}, true
}

func validGlobalClaims(factors []Factor, completion Completion, activations []ActivationFamily, rules []Rule, queries []QueryFamily) bool {
	claimed := make(map[Key]struct{})
	claim := func(key Key) bool {
		if !key.Available() {
			return false
		}
		if _, exists := claimed[key]; exists {
			return false
		}
		claimed[key] = struct{}{}
		return true
	}
	for _, factor := range factors {
		if !claim(factor.Key) {
			return false
		}
		for _, form := range factor.Forms {
			if !claim(form.Semantic) {
				return false
			}
		}
	}
	if completion.Semantic.Available() && (!claim(completion.Semantic) || !claim(completion.Prune)) {
		return false
	}
	for _, family := range activations {
		if !claim(family.Semantic) {
			return false
		}
	}
	for _, rule := range rules {
		if !claim(rule.Key) {
			return false
		}
		for _, carry := range rule.Carries {
			if carry.Transform.Available() && !claim(carry.Transform) {
				return false
			}
		}
	}
	for _, query := range queries {
		if !claim(query.Key) || !claim(query.Freezer) {
			return false
		}
	}
	return true
}

// validFactorFreeStructuralCandidate admits only the support-completion
// relation with no Factor operations.  This is a schema restriction, not a
// second composition form: the caller still seals the same cold identity and
// later materializes the same equation/guard/carrier execution path with an
// empty root vector.
func validFactorFreeStructuralCandidate(completion Completion, activations []ActivationFamily, rules []Rule, queries []QueryFamily) bool {
	if len(activations) != 0 || len(rules) == 0 || len(queries) == 0 || !completion.Semantic.Available() || !completion.Prune.Available() {
		return false
	}
	for _, rule := range rules {
		if rule.OutputKind != StructuralOutput || rule.Output.Available() || len(rule.Reads) != 0 || len(rule.Carries) != 0 || len(rule.Writes) != 0 || len(rule.Activations) != 0 || !validStructuralSupportPrunes(rule.Supports, rule.Prunes, completion) {
			return false
		}
	}
	for _, query := range queries {
		if len(query.Projections) != 1 || query.Projections[0].Kind != QuerySupport || query.Projections[0].Factor.Available() || query.Projections[0].Normalizer.Available() {
			return false
		}
	}
	return true
}

func validQueryFamily(query QueryFamily, factors map[Key]uint64, forms map[Key]map[Key]FactorFormKind, completion Completion) bool {
	if !query.Key.Available() || !query.Freezer.Available() || !query.Population.Available() {
		return false
	}
	if len(query.Projections) == 0 {
		return false
	}
	if len(query.Projections) == 1 && query.Projections[0].Kind == QuerySupport {
		projection := query.Projections[0]
		return completion.Semantic.Available() && !projection.Factor.Available() && !projection.Normalizer.Available()
	}
	for _, projection := range query.Projections {
		switch projection.Kind {
		case QueryFactorExact:
			if _, ok := factors[projection.Factor]; !ok || !projection.Factor.Available() || projection.Normalizer.Available() {
				return false
			}
		case QueryFactorSummary:
			if _, ok := factors[projection.Factor]; !ok || !projection.Factor.Available() || !projection.Normalizer.Available() || !hasSummaryReadForm(forms, projection.Factor, projection.Normalizer) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validFactor(factor Factor) bool {
	if !factor.Key.Available() {
		return false
	}
	for index, form := range factor.Forms {
		if !form.Semantic.Available() || form.Semantic == factor.Key || form.Kind < FactorSummaryRead || form.Kind > FactorDistributiveSummaryRead ||
			index > 0 && !lessFactorForm(factor.Forms[index-1], form) {
			return false
		}
	}
	return true
}

func validCompletion(completion Completion) bool {
	if !completion.Semantic.Available() && !completion.Prune.Available() {
		return true
	}
	return completion.Semantic.Available() && completion.Prune.Available() && completion.Semantic != completion.Prune
}

func validRule(rule *Rule, factors map[Key]uint64, forms map[Key]map[Key]FactorFormKind, completion Completion, activations map[Key]ActivationFamily) bool {
	if rule == nil || !rule.Key.Available() || !rule.OperandFamily.Available() {
		return false
	}
	canonicalSupports(rule.Supports)
	canonicalPrunes(rule.Prunes)
	canonicalActivationRanges(rule.Activations)
	if !validSupports(rule.Supports) || !validPrunes(rule.Prunes) || !validActivationRanges(rule.Activations, activations) {
		return false
	}
	switch rule.OutputKind {
	case FactorOutput:
		if _, ok := factors[rule.Output]; !ok || len(rule.Supports) != 0 || len(rule.Prunes) != 0 || len(rule.Activations) != 0 {
			return false
		}
		// validCarries and validWrites both fence every owned patch/carry to
		// this one Output Factor.  Multiple target forms may contribute to
		// that one Factor patch, but a Rule cannot own a second Factor.
		return len(rule.Carries) <= 1 && validReads(rule.Reads, rule.Inputs, factors, forms) && validCarries(rule.Carries, rule.Inputs, rule.Output, factors, forms) && validWrites(rule.Writes, rule.Output, rule.Reads, factors, forms) && (len(rule.Writes) != 0 || len(rule.Carries) != 0)
	case StructuralOutput:
		// Structural Rules may inspect typed Factor premises, but they never
		// write/carry a Factor.  Their only effect is a declared structural
		// capability.  Activation is introduced by its own cold capability;
		// the currently admitted structural capability is Prune.
		if rule.Output.Available() || len(rule.Carries) != 0 || len(rule.Writes) != 0 || !validReads(rule.Reads, rule.Inputs, factors, forms) {
			return false
		}
		if len(rule.Activations) != 0 {
			return len(rule.Supports) == 0 && len(rule.Prunes) == 0 && len(rule.Activations) == 1
		}
		return len(rule.Prunes) != 0 && validStructuralSupportPrunes(rule.Supports, rule.Prunes, completion)
	default:
		return false
	}
}

func validActivationFamily(family *ActivationFamily) bool {
	if family == nil {
		return false
	}
	canonical, ok := CanonicalActivationFamily(*family)
	return ok && sameActivationFamily(*family, canonical)
}

func allActivationFamiliesReferenced(families []ActivationFamily, rules []Rule) bool {
	if len(families) == 0 {
		return true
	}
	referenced := make(map[Key]struct{}, len(families))
	for _, rule := range rules {
		for _, activation := range rule.Activations {
			referenced[activation.Family] = struct{}{}
		}
	}
	for _, family := range families {
		if _, found := referenced[family.Semantic]; !found {
			return false
		}
	}
	return true
}

func validActivationRanges(ranges []ActivationRange, families map[Key]ActivationFamily) bool {
	for index, activation := range ranges {
		if _, known := families[activation.Family]; !known || !activation.Family.Available() || index > 0 && !lessKey(ranges[index-1].Family, activation.Family) {
			return false
		}
	}
	return true
}

func validStructuralSupportPrunes(supports []Support, prunes []Prune, completion Completion) bool {
	// The current support-completion capability is a single matched input /
	// prune pair.  Keeping this as a structural relation (rather than treating
	// an absent Factor output as the tag) leaves room for the separately-owned
	// activation capability without weakening factor ownership.
	return len(supports) == 1 && len(prunes) == 1 && declaredCompletion(completion, supports[0], prunes[0])
}

func declaredCompletion(completion Completion, support Support, prune Prune) bool {
	return completion.Semantic.Available() && completion.Semantic == support.Semantic && completion.Prune == prune.Semantic
}

func validReads(reads []Read, inputs uint64, factors map[Key]uint64, forms map[Key]map[Key]FactorFormKind) bool {
	for index, read := range reads {
		if read.Kind < ReadExact || read.Kind > ReadSelect || read.Input >= inputs || !read.Factor.Available() || !validDependencies(read.Dependencies, uint64(index)) {
			return false
		}
		if _, ok := factors[read.Factor]; !ok {
			return false
		}
		switch read.Kind {
		case ReadExact:
			if read.Semantic.Available() || read.Normalizer.Available() || len(read.Dependencies) != 0 {
				return false
			}
		case ReadSummary:
			if !read.Semantic.Available() || read.Normalizer != read.Semantic || len(read.Dependencies) != 0 || !hasSummaryReadForm(forms, read.Factor, read.Normalizer) {
				return false
			}
		case ReadSelect:
			if read.Semantic != read.Factor || read.Normalizer.Available() || len(read.Dependencies) == 0 {
				return false
			}
		}
	}
	return true
}
func validCarries(carries []Carry, inputs uint64, output Key, factors map[Key]uint64, forms map[Key]map[Key]FactorFormKind) bool {
	for i, carry := range carries {
		if carry.Input >= inputs || carry.Factor != output || i > 0 && carries[i-1].Input >= carry.Input {
			return false
		}
		if _, ok := factors[carry.Factor]; !ok {
			return false
		}
	}
	return true
}
func validWrites(writes []Write, output Key, reads []Read, factors map[Key]uint64, forms map[Key]map[Key]FactorFormKind) bool {
	routeCount := 0
	for _, write := range writes {
		if write.Kind < WriteExact || write.Kind > WriteRoute || write.Factor != output {
			return false
		}
		if _, ok := factors[write.Factor]; !ok {
			return false
		}
		switch write.Kind {
		case WriteExact:
			if write.Route != 0 {
				return false
			}
		case WriteRoute:
			routeCount++
			if routeCount != 1 || write.Route == 0 || write.Route > uint64(len(reads)) {
				return false
			}
			read := reads[write.Route-1]
			if read.Kind != ReadSelect || read.Factor != output {
				return false
			}
		}
	}
	// A routed atomic batch has one exact selected input and one output
	// disposition.  Carry may coexist to preserve coordinates not selected by
	// the route, but a second static output would introduce a competing path.
	return routeCount == 0 || len(writes) == 1
}

// hasSummaryReadForm accepts either declared summary read fold. The fold
// changes how the carrier partitions the declared vector, never whether the
// Rule or Query may name the form.
func hasSummaryReadForm(forms map[Key]map[Key]FactorFormKind, factor, semantic Key) bool {
	return hasFactorForm(forms, factor, semantic, FactorSummaryRead) || hasFactorForm(forms, factor, semantic, FactorDistributiveSummaryRead)
}

func hasFactorForm(forms map[Key]map[Key]FactorFormKind, factor, semantic Key, kind FactorFormKind) bool {
	owner, present := forms[factor]
	if !present {
		return false
	}
	actual, present := owner[semantic]
	return present && actual == kind
}

func canonicalFactorForms(forms []FactorForm) {
	sort.Slice(forms, func(left, right int) bool { return lessFactorForm(forms[left], forms[right]) })
}

func lessFactorForm(left, right FactorForm) bool {
	if order := compareKey(left.Semantic, right.Semantic); order != 0 {
		return order < 0
	}
	return left.Kind < right.Kind
}
func validSupports(values []Support) bool {
	for i, value := range values {
		if !value.Semantic.Available() {
			return false
		}
		if i > 0 && values[i-1] == value {
			return false
		}
	}
	return true
}
func validPrunes(values []Prune) bool {
	for i, value := range values {
		if !value.Semantic.Available() {
			return false
		}
		if i > 0 && values[i-1] == value {
			return false
		}
	}
	return true
}
func canonicalSupports(values []Support) {
	sort.Slice(values, func(i, j int) bool { return lessKey(values[i].Semantic, values[j].Semantic) })
}
func canonicalPrunes(values []Prune) {
	sort.Slice(values, func(i, j int) bool { return lessKey(values[i].Semantic, values[j].Semantic) })
}
func canonicalActivationRanges(values []ActivationRange) {
	sort.Slice(values, func(left, right int) bool { return lessKey(values[left].Family, values[right].Family) })
}
func validDependencies(values []uint64, limit uint64) bool {
	for i, value := range values {
		if value >= limit || i > 0 && values[i-1] >= value {
			return false
		}
	}
	return true
}
func compositionID(factors []Factor, completion Completion, activations []ActivationFamily, rules []Rule, queries []QueryFamily) (ID, bool) {
	hash := sha256.New()
	var writer canonical.Writer
	if writer.Reset(context.Background(), hash, "analysis/engine/cold-composition", codecVersion) != nil {
		return ID{}, false
	}
	key := func(value Key) bool { return writer.Bytes(value.ID[:]) == nil && writer.Uint(value.Version) == nil }
	deps := func(values []uint64) bool {
		if writer.Count(uint64(len(values))) != nil {
			return false
		}
		for _, value := range values {
			if writer.Uint(value) != nil {
				return false
			}
		}
		return true
	}
	if writer.Count(uint64(len(factors))) != nil {
		return ID{}, false
	}
	for _, factor := range factors {
		if !key(factor.Key) || writer.Count(uint64(len(factor.Forms))) != nil {
			return ID{}, false
		}
		for _, form := range factor.Forms {
			if writer.Uint(uint64(form.Kind)) != nil || !key(form.Semantic) {
				return ID{}, false
			}
		}
	}
	if !key(completion.Semantic) || !key(completion.Prune) {
		return ID{}, false
	}
	if writer.Count(uint64(len(activations))) != nil {
		return ID{}, false
	}
	for _, family := range activations {
		if !key(family.Semantic) {
			return ID{}, false
		}
	}
	if writer.Count(uint64(len(rules))) != nil {
		return ID{}, false
	}
	for _, rule := range rules {
		if !key(rule.Key) || !key(rule.OperandFamily) || writer.Uint(uint64(rule.OutputKind)) != nil || !key(rule.Output) || writer.Uint(rule.Inputs) != nil {
			return ID{}, false
		}
		if writer.Count(uint64(len(rule.Reads))) != nil {
			return ID{}, false
		}
		for _, read := range rule.Reads {
			if writer.Uint(uint64(read.Kind)) != nil || writer.Uint(read.Input) != nil || !key(read.Factor) || !key(read.Semantic) || !key(read.Normalizer) || !deps(read.Dependencies) {
				return ID{}, false
			}
		}
		if writer.Count(uint64(len(rule.Carries))) != nil {
			return ID{}, false
		}
		for _, carry := range rule.Carries {
			if writer.Uint(carry.Input) != nil || !key(carry.Factor) || !key(carry.Transform) {
				return ID{}, false
			}
		}
		if writer.Count(uint64(len(rule.Writes))) != nil {
			return ID{}, false
		}
		for _, write := range rule.Writes {
			if writer.Uint(uint64(write.Kind)) != nil || !key(write.Factor) || writer.Uint(write.Route) != nil {
				return ID{}, false
			}
		}
		if writer.Count(uint64(len(rule.Supports))) != nil {
			return ID{}, false
		}
		for _, value := range rule.Supports {
			if !key(value.Semantic) {
				return ID{}, false
			}
		}
		if writer.Count(uint64(len(rule.Prunes))) != nil {
			return ID{}, false
		}
		for _, value := range rule.Prunes {
			if !key(value.Semantic) {
				return ID{}, false
			}
		}
		if writer.Count(uint64(len(rule.Activations))) != nil {
			return ID{}, false
		}
		for _, activation := range rule.Activations {
			if !key(activation.Family) {
				return ID{}, false
			}
		}
	}
	if writer.Count(uint64(len(queries))) != nil {
		return ID{}, false
	}
	for _, query := range queries {
		if !key(query.Key) || !key(query.Freezer) || writer.Uint(uint64(query.Population)) != nil || writer.Count(uint64(len(query.Projections))) != nil {
			return ID{}, false
		}
		for _, projection := range query.Projections {
			if writer.Uint(uint64(projection.Kind)) != nil || !key(projection.Factor) || !key(projection.Normalizer) {
				return ID{}, false
			}
		}
	}
	if writer.Finish() != nil {
		return ID{}, false
	}
	digest := hash.Sum(nil)
	if len(digest) != len(ID{}) {
		return ID{}, false
	}
	var result ID
	copy(result[:], digest)
	return result, true
}

func copyRules(input []Rule) []Rule {
	result := make([]Rule, len(input))
	for i, rule := range input {
		result[i] = copyRule(rule)
	}
	return result
}

func copyRule(rule Rule) Rule {
	result := rule
	result.Reads = append([]Read(nil), rule.Reads...)
	for index := range result.Reads {
		result.Reads[index].Dependencies = append([]uint64(nil), rule.Reads[index].Dependencies...)
	}
	result.Carries = append([]Carry(nil), rule.Carries...)
	result.Writes = append([]Write(nil), rule.Writes...)
	result.Supports = append([]Support(nil), rule.Supports...)
	result.Prunes = append([]Prune(nil), rule.Prunes...)
	result.Activations = copyActivationRanges(rule.Activations)
	return result
}

func copyActivationFamilies(input []ActivationFamily) []ActivationFamily {
	result := make([]ActivationFamily, len(input))
	for index := range input {
		result[index] = copyActivationFamily(input[index])
	}
	return result
}

func copyActivationFamily(input ActivationFamily) ActivationFamily {
	return ActivationFamily{Semantic: input.Semantic}
}

func copyActivationRanges(input []ActivationRange) []ActivationRange {
	result := make([]ActivationRange, len(input))
	for index := range input {
		result[index] = ActivationRange{Family: input[index].Family}
	}
	return result
}

func copyQueries(input []QueryFamily) []QueryFamily {
	result := make([]QueryFamily, len(input))
	for index, query := range input {
		result[index] = query
		result[index].Projections = append([]QueryProjection(nil), query.Projections...)
	}
	return result
}

func copyFactors(input []Factor) []Factor {
	result := make([]Factor, len(input))
	for index, factor := range input {
		result[index] = factor
		result[index].Forms = append([]FactorForm(nil), factor.Forms...)
	}
	return result
}
func compareKey(left, right Key) int {
	for i := range left.ID {
		if left.ID[i] < right.ID[i] {
			return -1
		}
		if left.ID[i] > right.ID[i] {
			return 1
		}
	}
	if left.Version < right.Version {
		return -1
	}
	if left.Version > right.Version {
		return 1
	}
	return 0
}
func lessKey(left, right Key) bool { return compareKey(left, right) < 0 }
