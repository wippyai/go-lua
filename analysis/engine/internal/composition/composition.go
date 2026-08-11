// Package composition seals the cold Wave-D domain schema.  It deliberately
// knows no concrete Rule instance, point, unit, target, callback, carrier, or
// schedule.  Equation topology is the later Wave-E owner of those concerns.
package composition

import (
	"context"
	"crypto/sha256"
	"sort"

	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// codecVersion changes whenever a cold semantic term changes meaning. The
// trusted-theorem provenance rename is semantic: previous `Theorem` rows must
// never be mistaken for checked artifacts by a persisted CompositionID.
const codecVersion = 15

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
	WriteSelect
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

// AdmissionKind is the exhaustive transfer-soundness provenance of a Rule.
// Neither value is a runtime mode: it is sealed provenance of the one
// evaluator admission path selected by the owning domain.
type AdmissionKind uint8

const (
	AdmissionInvalid AdmissionKind = iota
	// AdmissionTrustedTheorem is one explicitly named/versioned reviewed TCB
	// obligation. It is not an artifact-verification or exhaustive-proof
	// classification; no such basis exists until its verifier exists.
	AdmissionTrustedTheorem
	AdmissionDerivation
)

// Admission identifies the versioned trusted theorem or local checker that
// licenses a Rule evaluator. Its implementation closure stays outside cold
// identity; its basis and semantic identity are part of the Rule and
// Composition digest.
type Admission struct {
	Kind     AdmissionKind
	Identity Key
}

// Carry is the schema-level whole-output relation.
type Carry struct {
	Input     uint64
	Factor    Key
	Transform Key
}

// Write retains the output Factor, write form, selector-law shape, and the
// ordered prior-read candidate vector for a staged target. Concrete targets
// and their local identities belong to equation instances, but the candidate
// read ordinals are cold semantic identity: E must pair them positionally to
// the presealed target surface.
type Write struct {
	Kind     WriteKind
	Factor   Key
	Semantic Key
	// Route is the one-based ordinal of the preceding ReadSelect consumed by
	// WriteRoute.  It is zero for every other write kind.
	Route        uint64
	Candidates   []uint64
	Dependencies []Dependency
}

type Dependency struct {
	Target bool
	Index  uint64
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
// It names exactly one cold family permission.  Concrete locator membership
// is checked only when equation Topology seals an ActivationBinding.
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
	FactorSelectorWrite
)

// FactorForm records one Factor-owned extension form. Semantic is globally
// authored identity; Kind prevents a summary normalizer from being replayed
// as a selector-write law.
type FactorForm struct {
	Kind     FactorFormKind
	Semantic Key
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
	Admission     Admission
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
func (c *Composition) RuleIndex(key Key) (uint64, bool) {
	if c == nil {
		return 0, false
	}
	value, ok := c.ruleAt[key]
	return value, ok
}
func (c *Composition) QueryIndex(key Key) (uint64, bool) {
	if c == nil {
		return 0, false
	}
	value, ok := c.queryAt[key]
	return value, ok
}
func (c *Composition) Factors() []Factor {
	if c == nil {
		return nil
	}
	return copyFactors(c.factors)
}
func (c *Composition) Rules() []Rule          { return copyRules(c.rules) }
func (c *Composition) Queries() []QueryFamily { return copyQueries(c.queries) }

// RuleAt returns one detached sealed rule without cloning the entire catalog.
// Dense ordinal admission remains owned by RuleIndex; callers cannot retain or
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
	// Operand families, admission evidence, and Factor references are excluded:
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
	if !query.Key.Available() || !query.Freezer.Available() {
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
			if _, ok := factors[projection.Factor]; !ok || !projection.Factor.Available() || !projection.Normalizer.Available() || !hasFactorForm(forms, projection.Factor, projection.Normalizer, FactorSummaryRead) {
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
		if !form.Semantic.Available() || form.Semantic == factor.Key || form.Kind < FactorSummaryRead || form.Kind > FactorSelectorWrite ||
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
	if rule == nil || !rule.Key.Available() || !rule.OperandFamily.Available() || !validAdmission(rule.Admission) {
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

func validAdmission(admission Admission) bool {
	return admission.Identity.Available() && (admission.Kind == AdmissionTrustedTheorem || admission.Kind == AdmissionDerivation)
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
			if !read.Semantic.Available() || read.Normalizer != read.Semantic || len(read.Dependencies) != 0 || !hasFactorForm(forms, read.Factor, read.Normalizer, FactorSummaryRead) {
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
	for i, write := range writes {
		if write.Kind < WriteExact || write.Kind > WriteRoute || write.Factor != output || !validWriteDependencies(write.Dependencies, uint64(len(reads)), uint64(i)) {
			return false
		}
		if _, ok := factors[write.Factor]; !ok {
			return false
		}
		switch write.Kind {
		case WriteExact:
			if write.Semantic.Available() || write.Route != 0 || len(write.Candidates) != 0 || len(write.Dependencies) != 0 {
				return false
			}
		case WriteSelect:
			if !write.Semantic.Available() || write.Route != 0 || !hasFactorForm(forms, write.Factor, write.Semantic, FactorSelectorWrite) || len(write.Candidates) == 0 || !validDependencies(write.Candidates, uint64(len(reads))) || len(write.Dependencies) == 0 {
				return false
			}
		case WriteRoute:
			routeCount++
			if routeCount != 1 || write.Semantic.Available() || write.Route == 0 || write.Route > uint64(len(reads)) || len(write.Candidates) != 0 || len(write.Dependencies) != 0 {
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
func validWriteDependencies(values []Dependency, reads, targets uint64) bool {
	for i, value := range values {
		if value.Target && value.Index >= targets || !value.Target && value.Index >= reads {
			return false
		}
		if i > 0 && (values[i-1].Target && !value.Target || values[i-1].Target == value.Target && values[i-1].Index >= value.Index) {
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
	writeDeps := func(values []Dependency) bool {
		if writer.Count(uint64(len(values))) != nil {
			return false
		}
		for _, value := range values {
			if writer.Bool(value.Target) != nil || writer.Uint(value.Index) != nil {
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
		if !key(rule.Key) || !key(rule.OperandFamily) || writer.Uint(uint64(rule.Admission.Kind)) != nil || !key(rule.Admission.Identity) || writer.Uint(uint64(rule.OutputKind)) != nil || !key(rule.Output) || writer.Uint(rule.Inputs) != nil {
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
			if writer.Uint(uint64(write.Kind)) != nil || !key(write.Factor) || !key(write.Semantic) || !deps(write.Candidates) || !writeDeps(write.Dependencies) {
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
		if !key(query.Key) || !key(query.Freezer) || writer.Count(uint64(len(query.Projections))) != nil {
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
	for index := range result.Writes {
		result.Writes[index].Candidates = append([]uint64(nil), rule.Writes[index].Candidates...)
		result.Writes[index].Dependencies = append([]Dependency(nil), rule.Writes[index].Dependencies...)
	}
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
