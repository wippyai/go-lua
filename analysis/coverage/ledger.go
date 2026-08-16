package coverage

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

// OwnerClass distinguishes Factor-owned conclusions from existing source-side
// structural authorities. Structural requirements may not mint a fake Factor
// just to satisfy coverage.
type OwnerClass uint8

const (
	OwnerInvalid OwnerClass = iota
	OwnerFactor
	OwnerStructural
)

// StructuralAuthorityKind keeps Source, Flow, Module, Target, and Link owner
// boundaries distinct even though all use the same immutable ContentID shape.
// Production plans must name the kind and the exact sealed owner ID together.
type StructuralAuthorityKind uint8

const (
	StructuralAuthorityUnset StructuralAuthorityKind = iota
	StructuralAuthoritySource
	StructuralAuthorityFlow
	StructuralAuthorityModule
	StructuralAuthorityTarget
	StructuralAuthorityLinkModule
	StructuralAuthorityLinkStatic
)

// Requirement is one exact source-relation × domain-conclusion obligation.
// Owner locates the Factor receiving that conclusion (or an existing
// structural authority); Conclusion remains distinct so one source can
// lawfully feed several judgments of the same Factor. Neither string names
// nor runtime facts cross this cut.
type Requirement struct {
	Source     semanticsource.Token
	Class      OwnerClass
	Owner      engine.SemanticKey
	Conclusion engine.SemanticKey
	// Authority is populated only for OwnerStructural requirements. It is the
	// exact sealed owner ContentID which issued the structural treatment.
	// Factor requirements continue to use their opaque engine semantic owner
	// above.
	Authority     keyspace.ContentID
	AuthorityKind StructuralAuthorityKind
}

// CoverageContract is declared by its semantic owner.  A contract is
// intentionally one exact pair: several Factor owners may legitimately claim
// different conclusions for the same source relation, but no owner may claim
// the same pair twice.
type CoverageContract struct {
	Source        semanticsource.Token
	Class         OwnerClass
	Owner         engine.SemanticKey
	Conclusion    engine.SemanticKey
	Authority     keyspace.ContentID
	AuthorityKind StructuralAuthorityKind
}

func (contract CoverageContract) requirement() Requirement {
	return Requirement{Source: contract.Source, Class: contract.Class, Owner: contract.Owner, Conclusion: contract.Conclusion, Authority: contract.Authority, AuthorityKind: contract.AuthorityKind}
}

// RulePlan assigns one or more exact requirements to a sealed Rule semantic
// identity.  Covers must be nonempty, duplicate-free, and canonically sorted.
// A single Rule schema may therefore cover several Factor-owned requirements
// without creating adapters or a second rule authority.
type RulePlan struct {
	Semantic engine.SemanticKey
	Covers   []Requirement
}

// QueryPlan assigns one or more exact requirements to a sealed Query semantic
// identity.  It is separate from RulePlan because queries observe results and
// cannot be silently treated as executable Rules.
type QueryPlan struct {
	Semantic engine.SemanticKey
	Covers   []Requirement
}

// StructuralPlan records source requirements discharged by one exact sealed
// structural authority rather than an executable Rule or Query. Structural
// plans have no engine semantic identity: the owner ContentID and kind are
// the authority.
type StructuralPlan struct {
	Authority     keyspace.ContentID
	AuthorityKind StructuralAuthorityKind
	Covers        []Requirement
}

type treatmentKind uint8

const (
	treatmentInvalid treatmentKind = iota
	treatmentRule
	treatmentQuery
	treatmentStructural
)

type treatment struct {
	kind          treatmentKind
	semantic      engine.SemanticKey
	authority     keyspace.ContentID
	authorityKind StructuralAuthorityKind
	covers        []Requirement
}

// Ledger is a frozen coverage result bound to one exact source catalog and
// sealed receipt Binding. Its contents are private so later caller mutation
// cannot turn a failed denominator into an accepted one.
type Ledger struct {
	catalog       SourceCatalog
	requirements  []Requirement
	treatments    []treatment
	compositionID engine.CompositionID
	valid         bool
}

// IssueKind names one fail-closed semantic coverage failure.  It never names
// a Go package, declaration, source location, fact payload, or topology row.
type IssueKind uint8

const (
	IssueInvalidCatalog IssueKind = iota + 1
	IssueInvalidRequirement
	IssueUnknownRequirementSource
	IssueDuplicateRequirement
	IssueMissingFactorOwner
	IssueUnclaimedCompositionFactor
	IssueInvalidTreatment
	IssueNonCanonicalCovers
	IssueDuplicateTreatmentSemantic
	IssueUnknownTreatmentRequirement
	IssueTreatmentReuse
	IssueMissingRequirement
	IssueMissingCompositionRule
	IssueMissingCompositionQuery
	IssueTreatmentKindMismatch
	IssueUnclaimedCompositionRule
	IssueUnclaimedCompositionQuery
	IssueIncompleteCompositionReport
)

// Issue identifies the opaque semantic item that failed validation.  Source
// Owner, and Conclusion identify requirements; Treatment identifies a planned or
// reported Rule/Query/structural authority.
type Issue struct {
	Kind          IssueKind
	Source        semanticsource.Token
	Class         OwnerClass
	Owner         engine.SemanticKey
	Conclusion    engine.SemanticKey
	Authority     keyspace.ContentID
	AuthorityKind StructuralAuthorityKind
	Treatment     engine.SemanticKey
}

// Result is the complete fail-closed validation outcome.  It succeeds only
// when Issues is empty.
type Result struct{ Issues []Issue }

// Valid reports whether the full source catalog, contracts, treatment plan,
// and sealed composition agree exactly.
func (result Result) Valid() bool { return len(result.Issues) == 0 }

// Freeze validates the entire three-part coverage kernel in one cut:
// generated source catalog, Factor-owned contracts, and treatment plan. It
// reads only semantic identities from the receipt-native Binding report; Rule
// and Query schemas are deliberately not reinterpreted or duplicated here.
func Freeze(
	catalog SourceCatalog,
	contracts []CoverageContract,
	rules []RulePlan,
	queries []QueryPlan,
	structural []StructuralPlan,
	binding *engine.SchemaBinding,
) (Ledger, Result) {
	ledger := Ledger{catalog: cloneCatalog(catalog)}
	result := Result{}
	if !catalog.valid {
		result.Issues = append(result.Issues, Issue{Kind: IssueInvalidCatalog})
	}

	requirements := make(map[Requirement]struct{}, len(contracts))
	for _, contract := range contracts {
		requirement := contract.requirement()
		if !validRequirement(requirement) {
			result.Issues = append(result.Issues, issueForRequirement(IssueInvalidRequirement, requirement))
			continue
		}
		if !catalog.contains(requirement.Source) {
			result.Issues = append(result.Issues, issueForRequirement(IssueUnknownRequirementSource, requirement))
			continue
		}
		if _, duplicate := requirements[requirement]; duplicate {
			result.Issues = append(result.Issues, issueForRequirement(IssueDuplicateRequirement, requirement))
			continue
		}
		requirements[requirement] = struct{}{}
	}

	plans := collectTreatments(rules, queries, structural)
	ledger.requirements = sortedRequirements(requirements)
	ledger.treatments = cloneTreatments(plans)
	validateTreatments(&result, requirements, plans)
	validateBinding(&result, requirements, plans, binding)
	ledger.valid = result.Valid()
	if ledger.valid {
		ledger.compositionID = binding.Schema().ID()
	}
	return ledger, result
}

func collectTreatments(rules []RulePlan, queries []QueryPlan, structural []StructuralPlan) []treatment {
	plans := make([]treatment, 0, len(rules)+len(queries)+len(structural))
	for _, plan := range rules {
		plans = append(plans, treatment{kind: treatmentRule, semantic: plan.Semantic, covers: append([]Requirement(nil), plan.Covers...)})
	}
	for _, plan := range queries {
		plans = append(plans, treatment{kind: treatmentQuery, semantic: plan.Semantic, covers: append([]Requirement(nil), plan.Covers...)})
	}
	for _, plan := range structural {
		plans = append(plans, treatment{kind: treatmentStructural, authority: plan.Authority, authorityKind: plan.AuthorityKind, covers: append([]Requirement(nil), plan.Covers...)})
	}
	return plans
}

func validateTreatments(result *Result, requirements map[Requirement]struct{}, plans []treatment) {
	semantics := make(map[engine.SemanticKey]treatmentKind, len(plans))
	type structuralIdentity struct {
		kind StructuralAuthorityKind
		id   keyspace.ContentID
	}
	authorities := make(map[structuralIdentity]struct{}, len(plans))
	covered := make(map[Requirement]engine.SemanticKey, len(requirements))
	for _, plan := range plans {
		if !validTreatment(plan) {
			result.Issues = append(result.Issues, Issue{Kind: IssueInvalidTreatment, Treatment: plan.semantic})
			continue
		}
		if plan.kind == treatmentStructural {
			identity := structuralIdentity{kind: plan.authorityKind, id: plan.authority}
			if _, duplicate := authorities[identity]; duplicate {
				result.Issues = append(result.Issues, Issue{Kind: IssueDuplicateTreatmentSemantic, Authority: plan.authority, AuthorityKind: plan.authorityKind})
			} else {
				authorities[identity] = struct{}{}
			}
		} else if _, duplicate := semantics[plan.semantic]; duplicate {
			result.Issues = append(result.Issues, Issue{Kind: IssueDuplicateTreatmentSemantic, Treatment: plan.semantic})
		} else {
			semantics[plan.semantic] = plan.kind
		}
		if !canonicalCovers(plan.covers) {
			result.Issues = append(result.Issues, Issue{Kind: IssueNonCanonicalCovers, Treatment: plan.semantic})
		}
		for _, requirement := range plan.covers {
			if !planOwnsRequirement(plan, requirement) {
				result.Issues = append(result.Issues, issueForTreatmentRequirement(IssueTreatmentKindMismatch, plan.semantic, requirement))
			}
			if _, known := requirements[requirement]; !known {
				result.Issues = append(result.Issues, issueForTreatmentRequirement(IssueUnknownTreatmentRequirement, plan.semantic, requirement))
				continue
			}
			if previous, reused := covered[requirement]; reused {
				result.Issues = append(result.Issues, issueForTreatmentRequirement(IssueTreatmentReuse, plan.semantic, requirement))
				_ = previous
				continue
			}
			covered[requirement] = plan.semantic
		}
	}
	for requirement := range requirements {
		if _, found := covered[requirement]; !found {
			result.Issues = append(result.Issues, issueForRequirement(IssueMissingRequirement, requirement))
		}
	}
}

func planOwnsRequirement(plan treatment, requirement Requirement) bool {
	switch plan.kind {
	case treatmentRule:
		return requirement.Class == OwnerFactor || requirement.Class == OwnerStructural && requirement.Owner == plan.semantic
	case treatmentQuery:
		return requirement.Class == OwnerFactor
	case treatmentStructural:
		if requirement.Class != OwnerStructural {
			return false
		}
		return requirement.Authority.Available() && plan.authority.Available() &&
			requirement.Authority == plan.authority &&
			requirement.AuthorityKind != StructuralAuthorityUnset &&
			plan.authorityKind != StructuralAuthorityUnset &&
			requirement.AuthorityKind == plan.authorityKind
	default:
		return false
	}
}

func validateBinding(result *Result, requirements map[Requirement]struct{}, plans []treatment, binding *engine.SchemaBinding) {
	if binding == nil || !binding.Sealed() {
		result.Issues = append(result.Issues, Issue{Kind: IssueIncompleteCompositionReport})
		return
	}
	report, reported := binding.SemanticReport()
	schema := binding.Schema()
	if !reported || schema == nil || !report.ID.Available() || report.ID != schema.ID() {
		result.Issues = append(result.Issues, Issue{Kind: IssueIncompleteCompositionReport})
		return
	}
	factors := make(map[engine.SemanticKey]struct{})
	for _, component := range report.Components {
		for _, factor := range component.Factors {
			if !factor.Available() {
				result.Issues = append(result.Issues, Issue{Kind: IssueIncompleteCompositionReport})
				continue
			}
			factors[factor] = struct{}{}
		}
	}
	rules := make(map[engine.SemanticKey]engine.RuleSchemaReport, len(report.Rules))
	queries := make(map[engine.SemanticKey]engine.QuerySchemaReport, len(report.Queries))
	for _, rule := range report.Rules {
		if !rule.Semantic.Available() {
			result.Issues = append(result.Issues, Issue{Kind: IssueIncompleteCompositionReport})
			continue
		}
		if _, duplicate := rules[rule.Semantic]; duplicate {
			result.Issues = append(result.Issues, Issue{Kind: IssueIncompleteCompositionReport})
			continue
		}
		rules[rule.Semantic] = rule
	}
	for _, query := range report.Queries {
		if !query.Semantic.Available() {
			result.Issues = append(result.Issues, Issue{Kind: IssueIncompleteCompositionReport})
			continue
		}
		if _, duplicate := queries[query.Semantic]; duplicate {
			result.Issues = append(result.Issues, Issue{Kind: IssueIncompleteCompositionReport})
			continue
		}
		queries[query.Semantic] = query
	}
	validateFactorOwners(result, requirements, factors)
	claimedRules := make(map[engine.SemanticKey]struct{}, len(rules))
	claimedQueries := make(map[engine.SemanticKey]struct{}, len(queries))
	for _, plan := range plans {
		if plan.kind != treatmentStructural && !plan.semantic.Available() {
			continue
		}
		if plan.kind == treatmentStructural {
			continue
		}
		switch plan.kind {
		case treatmentRule:
			rule, exists := rules[plan.semantic]
			if !exists {
				if _, query := queries[plan.semantic]; query {
					result.Issues = append(result.Issues, Issue{Kind: IssueTreatmentKindMismatch, Treatment: plan.semantic})
				} else {
					result.Issues = append(result.Issues, Issue{Kind: IssueMissingCompositionRule, Treatment: plan.semantic})
				}
				continue
			}
			validateRuleCovers(result, rule, plan)
			claimedRules[plan.semantic] = struct{}{}
		case treatmentQuery:
			query, exists := queries[plan.semantic]
			if !exists {
				if _, rule := rules[plan.semantic]; rule {
					result.Issues = append(result.Issues, Issue{Kind: IssueTreatmentKindMismatch, Treatment: plan.semantic})
				} else {
					result.Issues = append(result.Issues, Issue{Kind: IssueMissingCompositionQuery, Treatment: plan.semantic})
				}
				continue
			}
			validateQueryCovers(result, query, plan)
			claimedQueries[plan.semantic] = struct{}{}
		case treatmentStructural:
			if _, rule := rules[plan.semantic]; rule {
				result.Issues = append(result.Issues, Issue{Kind: IssueTreatmentKindMismatch, Treatment: plan.semantic})
			}
			if _, query := queries[plan.semantic]; query {
				result.Issues = append(result.Issues, Issue{Kind: IssueTreatmentKindMismatch, Treatment: plan.semantic})
			}
		}
	}
	for semantic := range rules {
		if _, claimed := claimedRules[semantic]; !claimed {
			result.Issues = append(result.Issues, Issue{Kind: IssueUnclaimedCompositionRule, Treatment: semantic})
		}
	}
	for semantic := range queries {
		if _, claimed := claimedQueries[semantic]; !claimed {
			result.Issues = append(result.Issues, Issue{Kind: IssueUnclaimedCompositionQuery, Treatment: semantic})
		}
	}
}

func validateFactorOwners(result *Result, requirements map[Requirement]struct{}, factors map[engine.SemanticKey]struct{}) {
	claimed := make(map[engine.SemanticKey]struct{}, len(factors))
	for requirement := range requirements {
		if requirement.Class != OwnerFactor {
			continue
		}
		if _, exists := factors[requirement.Owner]; !exists {
			result.Issues = append(result.Issues, issueForRequirement(IssueMissingFactorOwner, requirement))
			continue
		}
		claimed[requirement.Owner] = struct{}{}
	}
	for factor := range factors {
		if _, exists := claimed[factor]; !exists {
			result.Issues = append(result.Issues, Issue{Kind: IssueUnclaimedCompositionFactor, Owner: factor})
		}
	}
}

func validateRuleCovers(result *Result, rule engine.RuleSchemaReport, plan treatment) {
	for _, requirement := range plan.covers {
		switch rule.OutputDisposition {
		case engine.RuleOutputDispositionFactor:
			if requirement.Class != OwnerFactor || rule.OutputFactor != requirement.Owner {
				result.Issues = append(result.Issues, issueForTreatmentRequirement(IssueTreatmentKindMismatch, plan.semantic, requirement))
			}
		case engine.RuleOutputDispositionStructural:
			if requirement.Class != OwnerStructural || requirement.Owner != plan.semantic {
				result.Issues = append(result.Issues, issueForTreatmentRequirement(IssueTreatmentKindMismatch, plan.semantic, requirement))
			}
		default:
			result.Issues = append(result.Issues, issueForTreatmentRequirement(IssueTreatmentKindMismatch, plan.semantic, requirement))
		}
	}
}

func validateQueryCovers(result *Result, query engine.QuerySchemaReport, plan treatment) {
	for _, requirement := range plan.covers {
		if requirement.Class != OwnerFactor || !queryProjects(query, requirement.Owner) {
			result.Issues = append(result.Issues, issueForTreatmentRequirement(IssueTreatmentKindMismatch, plan.semantic, requirement))
		}
	}
}

func queryProjects(query engine.QuerySchemaReport, owner engine.SemanticKey) bool {
	for _, projection := range query.Projections {
		if projection.Factor == owner {
			return true
		}
	}
	return false
}

func validRequirement(requirement Requirement) bool {
	if !availableToken(requirement.Source) || requirement.Class <= OwnerInvalid || requirement.Class > OwnerStructural {
		return false
	}
	if requirement.Class == OwnerStructural {
		return requirement.Authority.Available() && requirement.AuthorityKind != StructuralAuthorityUnset && validStructuralSource(requirement.Source, requirement.AuthorityKind)
	}
	return requirement.Owner.Available() && requirement.Conclusion.Available()
}

func validStructuralSource(source semanticsource.Token, kind StructuralAuthorityKind) bool {
	switch kind {
	case StructuralAuthoritySource:
		return source.Origin() == semanticsource.OriginProgramSourceProvenance || source.Origin() == semanticsource.OriginProgramSourceOrder
	case StructuralAuthorityFlow:
		return source.Facet() == 0 && (source.Origin() == semanticsource.OriginProgramFlowControl || source.Origin() == semanticsource.OriginProgramFlowCall)
	case StructuralAuthorityModule:
		return source.Origin() == semanticsource.OriginProgramModuleEntry && source.Facet() == 0
	case StructuralAuthorityTarget:
		switch source.Origin() {
		case semanticsource.OriginTargetContract, semanticsource.OriginTargetOperation, semanticsource.OriginTargetProtocol, semanticsource.OriginTargetBoot, semanticsource.OriginTargetGsub:
			return true
		default:
			return false
		}
	case StructuralAuthorityLinkModule:
		return source.Origin() == semanticsource.OriginLinkModule
	case StructuralAuthorityLinkStatic:
		return source.Origin() == semanticsource.OriginLinkStatic
	default:
		return false
	}
}

func validTreatment(plan treatment) bool {
	if plan.kind == treatmentStructural {
		return plan.authority.Available() && plan.authorityKind != StructuralAuthorityUnset && len(plan.covers) != 0
	}
	return plan.kind > treatmentInvalid && plan.kind <= treatmentStructural && plan.semantic.Available() && len(plan.covers) != 0
}

func canonicalCovers(covers []Requirement) bool {
	if len(covers) == 0 {
		return false
	}
	for index, requirement := range covers {
		if !validRequirement(requirement) || index != 0 && compareRequirement(covers[index-1], requirement) >= 0 {
			return false
		}
	}
	return true
}

func compareRequirement(left, right Requirement) int {
	if compared := compareToken(left.Source, right.Source); compared != 0 {
		return compared
	}
	if left.Class < right.Class {
		return -1
	}
	if left.Class > right.Class {
		return 1
	}
	if compared := compareSemantic(left.Owner, right.Owner); compared != 0 {
		return compared
	}
	if compared := compareAuthority(left.Authority, right.Authority); compared != 0 {
		return compared
	}
	if left.AuthorityKind < right.AuthorityKind {
		return -1
	}
	if left.AuthorityKind > right.AuthorityKind {
		return 1
	}
	return compareSemantic(left.Conclusion, right.Conclusion)
}

func compareAuthority(left, right keyspace.ContentID) int {
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}

func compareSemantic(left, right engine.SemanticKey) int {
	leftDigest, rightDigest := left.Digest(), right.Digest()
	for index := range leftDigest {
		if leftDigest[index] < rightDigest[index] {
			return -1
		}
		if leftDigest[index] > rightDigest[index] {
			return 1
		}
	}
	if left.Version() < right.Version() {
		return -1
	}
	if left.Version() > right.Version() {
		return 1
	}
	return 0
}

func sortedRequirements(requirements map[Requirement]struct{}) []Requirement {
	result := make([]Requirement, 0, len(requirements))
	for requirement := range requirements {
		result = append(result, requirement)
	}
	sort.Slice(result, func(left, right int) bool { return compareRequirement(result[left], result[right]) < 0 })
	return result
}

func cloneTreatments(plans []treatment) []treatment {
	cloned := make([]treatment, len(plans))
	for index, plan := range plans {
		cloned[index] = treatment{kind: plan.kind, semantic: plan.semantic, authority: plan.authority, authorityKind: plan.authorityKind, covers: append([]Requirement(nil), plan.covers...)}
	}
	return cloned
}

func issueForRequirement(kind IssueKind, requirement Requirement) Issue {
	return Issue{Kind: kind, Source: requirement.Source, Class: requirement.Class, Owner: requirement.Owner, Conclusion: requirement.Conclusion, Authority: requirement.Authority, AuthorityKind: requirement.AuthorityKind}
}

func issueForTreatmentRequirement(kind IssueKind, treatment engine.SemanticKey, requirement Requirement) Issue {
	return Issue{Kind: kind, Source: requirement.Source, Class: requirement.Class, Owner: requirement.Owner, Conclusion: requirement.Conclusion, Authority: requirement.Authority, AuthorityKind: requirement.AuthorityKind, Treatment: treatment}
}
