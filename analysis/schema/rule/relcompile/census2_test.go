package relcompile_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	callquery "github.com/wippyai/go-lua/domain/call/query"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	"github.com/wippyai/go-lua/domain/placement"
	placementquery "github.com/wippyai/go-lua/domain/placement/query"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// census2GeometryTypes resolves the canonical geometry authorities from the
// structural declarations themselves. The external test package cannot use
// composite's unexported structureEntries, so it collects the production
// geometry specs directly; this preserves owner-issued TypeIDs without
// creating a second registry or synthetic model identities.
func census2GeometryTypes(t *testing.T) structure.GeometryTypes {
	t.Helper()
	entries, ok := structure.Collect(structure.RelationGeometrySpecs())
	if !ok {
		t.Fatal("canonical relation geometry entries unavailable")
	}
	types, ok := structure.RelationGeometryTypes(entries)
	if !ok {
		t.Fatal("canonical relation geometry types unavailable")
	}
	return types
}

func census2PlacementSummaryTypes(t *testing.T) placementquery.SummaryResultTypes {
	t.Helper()
	var ownerContent identity.ContentID
	ownerContent[0] = 0xe1
	owner, ownerOK := model.IssueOwnerID(ownerContent)
	if !ownerOK {
		t.Fatal("placement-summary census type owner")
	}
	issue := func(seed byte) model.TypeID {
		var content identity.ContentID
		content[0] = seed
		value, ok := model.IssueTypeID(owner, content)
		if !ok {
			t.Fatalf("placement-summary census type %d", seed)
		}
		return value
	}
	return placementquery.SummaryResultTypes{AllocationID: issue(1), Fact: issue(2), Evidence: issue(3), SchemaID: issue(4)}
}

// census2 is the non-rule half of the coverage matrix: the four registered
// query families and the denominator they are keyed by, the four canonical
// diagnostic observation kinds, and the two halves of the diagnostic-code
// vocabulary. Every row is one declared surface lowered through the same
// compiler the rule census uses, so the matrix reports what each declaration
// proves and never what a plan the compiler cannot read might have proved.
//
// The issuance seeds are deliberately not restated here. They are rule
// declarations with authored Programs and are already one row each in the rule
// census; TestEveryIssuanceSeedResolvesThroughTheLandedRegistry reads those
// rows rather than compiling a second copy of them.

// census2Row is one non-rule census row. It carries the rule census's own
// columns and one more: the owner statements the plan names that exist today
// only as Go-side computation. A plan that lowers is not a plan that can be
// mounted, and a matrix that reported only the first would hide the second.
type census2Row struct {
	entry
	Requires []string `json:"requires,omitempty"`
}

// census2Specimen is one non-rule declaration and the plan authored from it.
// Requires names the relations of the plan that no declaration surface states
// today; each is cited against the code that computes it instead.
type census2Specimen struct {
	Family   string
	Plane    string
	Rule     string
	Reason   string
	Requires []schema.Key
	Plan     func(*testing.T) *census2Plane
}

func census2Declared() []census2Specimen {
	return []census2Specimen{
		{
			Family: "query/query-site", Plane: "query", Rule: "query-site",
			Reason: "the selected-point denominator every family is keyed by; two positive recurrences and one product of two complete reads, in place of the hand-staged worklist fixpoint at domain/composite/query_sites.go:227-524",
			Requires: []schema.Key{
				"composite/selected-body", "composite/observed-point", "query/query-site", "query/family",
				"composite/non-callable-body", "composite/body-without-occurrence", "composite/eligible-context",
			},
			Plan: census2QuerySitePlan,
		},
		{
			Family: "query/value-summary", Plane: "query", Rule: string(value.SummaryResultFamily),
			Reason:   "FoldDistributive over one subject: the coordinatewise fold is a complete-span delivery keyed by the query site, closed against the value schema's coordinate plane",
			Requires: []schema.Key{"query/query-site", "query/value-summary", "value/summary-coordinates"},
			Plan:     census2ValueSummaryPlan,
		},
		{
			Family: "query/effect-exact", Plane: "query", Rule: string(factor.ExactResultFamily),
			Reason:   "FoldGeneral admitting no split: scalar delivery and exactly-one cardinality in the sealed signature, in place of the runtime cell-count guard at domain/effect/owner/query.go:110-112",
			Requires: []schema.Key{"query/query-site", "query/effect-exact"},
			Plan:     census2EffectExactPlan,
		},
		{
			Family: "query/placement-summary", Plane: "query", Rule: string(placement.SummaryResultFamily),
			Reason: "three subjects joined through two correspondence relations, with the complete heap vector as the heap read's own authenticated denominator; the bind-time schema equality at domain/placement/query/query.go:96 is the correspondence",
			Requires: []schema.Key{
				"query/query-site", "query/placement-summary",
				"corr/placement-heap", "corr/placement-evidence", "placement/allocation-roots",
			},
			Plan: census2PlacementSummaryPlan,
		},
		{
			Family: "observation/call-callee-set", Plane: "observation", Rule: string(callquery.CalleeSetResultFamily),
			Reason:   "PopulationObservation: the plan is read-only, ends in the relation its consumer reads from the converged snapshot, and publishes no selected-point answer; its population is the hand dedup at domain/composite/call_callee_set_observation.go:60",
			Requires: []schema.Key{"observation/call-callee-set-site"},
			Plan:     census2CalleeSetPlan,
		},
		{
			Family: "observation/branch-condition", Plane: "diagnostic",
			Rule:   string(structure.DiagnosticObservationBranchCondition.Key()),
			Reason: "population is the guarded routes; the guard, the two-armed branch and the scope-insensitive body are joins onto the facts that state them, and evidence points are a child relation; the four exits at analysis/program/artifact/compiler/internal/diagnostic/observation.go:65/68/71/74 are the absence of this population and closure",
			Requires: []schema.Key{
				"program/causal-final-route", "program/route-guard-proof", "program/authored-branch",
				"program/containment-scope-insensitive-body", "observation/branch-condition",
			},
			Plan: census2BranchConditionPlan,
		},
		{
			Family: "observation/type-reference-unresolved", Plane: "diagnostic",
			Rule:   string(structure.DiagnosticObservationTypeReferenceUnresolved.Key()),
			Reason: "population is the declared static types; an unresolved reference is a join onto the resolution relation, and the path components are a child relation",
			Requires: []schema.Key{
				"program/static-reference", "program/static-reference-unresolved", "program/static-type",
				"observation/type-reference-unresolved",
			},
			Plan: census2TypeReferenceUnresolvedPlan,
		},
		{
			Family: "observation/value-reference-unresolved", Plane: "diagnostic",
			Rule:   string(structure.DiagnosticObservationValueReferenceUnresolved.Key()),
			Reason: "population is the implicit reads; the undeclared global cell is a join onto the authored storage fact that states it",
			Requires: []schema.Key{
				"program/authored-storage-read", "program/authored-storage-read-implicit",
				"program/authored-storage-cell-global-undeclared", "observation/value-reference-unresolved",
			},
			Plan: census2ValueReferenceUnresolvedPlan,
		},
		{
			Family: "observation/type-conformance", Plane: "diagnostic",
			Rule:   string(structure.DiagnosticObservationTypeConformance.Key()),
			Reason: "one relation with four alternative derivations discriminated by the observation-site column; the structural arm is a positive recurrence, so the walker's visited set is SCC membership and the closure is separate from the arms",
			Requires: []schema.Key{
				"program/declared-typed-position", "program/declared-structural-target",
				"observation/type-conformance", "observation/type-conformance/evidence",
			},
			Plan: census2TypeConformancePlan,
		},
		{
			Family: "diagnostic/code-composed", Plane: "diagnostic", Rule: "diagnostic/code-composed",
			Reason:   "a code is composed when a sealed lane installs its producer, so the composed half closes the code relation against the producer relation",
			Requires: []schema.Key{"diagnostic/producer"},
			Plan:     census2DiagnosticComposedPlan,
		},
		{
			Family: "diagnostic/code-declared", Plane: "diagnostic", Rule: "diagnostic/code-declared",
			Reason:   "the declared-not-composed register carries the owing surface and the missing observation as columns of its own rows",
			Requires: []schema.Key{"diagnostic/declared-register"},
			Plan:     census2DiagnosticDeclaredPlan,
		},
	}
}

func census2Survey(t *testing.T, specimen census2Specimen) census2Row {
	t.Helper()
	plane := specimen.Plan(t)
	rendered := plane.rendered(schema.Key(specimen.Rule))
	requires := make([]string, 0, len(specimen.Requires))
	for _, required := range specimen.Requires {
		if !plane.installed[required] {
			t.Fatalf("%s requires %s, which its own plan does not name", specimen.Family, required)
		}
		requires = append(requires, string(required))
	}
	sort.Strings(requires)
	return census2Row{
		entry: entry{
			Family: specimen.Family, Plane: specimen.Plane, Rule: specimen.Rule,
			Status: statusCompiles, Sketch: rendered.sketch, Expressions: rendered.count,
			Reason: specimen.Reason,
		},
		Requires: requires,
	}
}

// TestNonRuleDeclarationCensus is the machine-readable matrix of the non-rule
// half. It is pinned for the same reason the rule census is: a change in what
// the query, observation and diagnostic-code declarations prove is a reviewed
// change.
func TestNonRuleDeclarationCensus(t *testing.T) {
	rows := make([]census2Row, 0, 16)
	for _, specimen := range census2Declared() {
		rows = append(rows, census2Survey(t, specimen))
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].Family != rows[right].Family {
			return rows[left].Family < rows[right].Family
		}
		return rows[left].Rule < rows[right].Rule
	})

	rendered, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatalf("render census: %v", err)
	}
	rendered = append(rendered, '\n')
	path := filepath.Join("testdata", "census2.json")
	if os.Getenv("RELCOMPILE_CENSUS_UPDATE") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("create testdata: %v", err)
		}
		if err := os.WriteFile(path, rendered, 0o644); err != nil {
			t.Fatalf("write census: %v", err)
		}
	}
	pinned, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read pinned census: %v", readErr)
	}
	if string(pinned) != string(rendered) {
		t.Fatalf("the non-rule census matrix moved; rerun with RELCOMPILE_CENSUS_UPDATE=1 and review\n%s", rendered)
	}
}

// TestEveryRegisteredQueryFamilyHasOneCensus2Row states the matrix is total
// over the query surface: the analyzer registers exactly four families and each
// one is surveyed once, under the authored key its own domain spells it with.
func TestEveryRegisteredQueryFamilyHasOneCensus2Row(t *testing.T) {
	geometry := census2GeometryTypes(t)
	registered := []queryschema.Spec{
		valueowner.QuerySpec(geometry), effectowner.QuerySpec(geometry),
		placementquery.QuerySpec(geometry, census2PlacementSummaryTypes(t)), callquery.QuerySpec(geometry),
	}
	surveyed := map[string]int{}
	for _, row := range census2Declared() {
		surveyed[row.Rule]++
	}
	for _, spec := range registered {
		if surveyed[string(spec.Family)] != 1 {
			t.Fatalf("query family %s has %d census rows, want one", spec.Family, surveyed[string(spec.Family)])
		}
	}
}

// TestTheObservationFamilyPublishesNoAnswer states the population distinction
// the call declaration makes survives lowering. A selected-point family
// proposes rows into its answer relation; an observation family is read-only
// and its plan ends in the relation its consumer reads.
func TestTheObservationFamilyPublishesNoAnswer(t *testing.T) {
	geometry := census2GeometryTypes(t)
	observation := census2CalleeSetPlan(t)
	if len(observation.rules) != 1 {
		t.Fatalf("callee-set derivations = %d, want one", len(observation.rules))
	}
	if observation.rules[0].Publish != nil {
		t.Fatal("the callee-set observation proposes an answer; its declared population publishes none")
	}
	if callquery.QuerySpec(geometry).Population != queryschema.PopulationObservation {
		t.Fatal("the callee-set declaration is no longer an observation population")
	}
	for _, plan := range []*census2Plane{census2ValueSummaryPlan(t), census2EffectExactPlan(t), census2PlacementSummaryPlan(t)} {
		for index, authored := range plan.rules {
			if authored.Publish == nil {
				t.Fatalf("selected-point derivation %d publishes no answer", index)
			}
		}
	}
}

// TestTheDistributiveFamilyFoldsUnderADeclaredGrouping states where the
// grouping of a query fold is stated. value declares FoldDistributive, which is
// the claim that the family may be answered over disjoint fragments of its
// subject and joined; relationally that is a grouped reduction, and the group
// it reduces under is the complete-span delivery key of its sealed signature.
// An exact family declares no such split and its input is scalar.
func TestTheDistributiveFamilyFoldsUnderADeclaredGrouping(t *testing.T) {
	geometry := census2GeometryTypes(t)
	if valueowner.QuerySpec(geometry).Fold != queryschema.FoldDistributive {
		t.Fatal("the value-summary declaration is no longer distributive")
	}
	summary := census2ValueSummaryPlan(t).compile("query/value-summary")
	grouped := 0
	for _, sealed := range summary.Signatures() {
		for _, input := range sealed.Inputs() {
			if input.Delivery.IsComplete() {
				grouped++
			}
		}
	}
	if grouped != 1 {
		t.Fatalf("value-summary complete-span inputs = %d, want the one grouping its fold reduces under", grouped)
	}

	if effectowner.QuerySpec(geometry).Fold != queryschema.FoldGeneral {
		t.Fatal("the effect-exact declaration is no longer general")
	}
	exact := census2EffectExactPlan(t).compile("query/effect-exact")
	for _, sealed := range exact.Signatures() {
		for _, input := range sealed.Inputs() {
			if !input.Delivery.IsScalar() {
				t.Fatal("the exact family folds a span; its declaration admits no split of its subject")
			}
		}
	}
	assertCensus2ExactlyOne(t, exact.Signatures())
}

// assertCensus2ExactlyOne states the cardinality half of an exact family: one
// site is answered by one row, so the sealed operation may not produce a second
// and may not decline to produce the first.
func assertCensus2ExactlyOne(t *testing.T, sealed []signature.Signature) {
	t.Helper()
	if len(sealed) == 0 {
		t.Fatal("the exact family seals no operation")
	}
	for _, one := range sealed {
		if one.Cardinality().Kind() != model.ExactlyOne {
			t.Fatalf("exact operation cardinality = %s, want ExactlyOne", one.Cardinality().Kind())
		}
	}
}

// TestThePlacementFamilyJoinsItsSubjectsThroughCorrespondence states that the
// three-subject precondition placement checks at bind time is relational. Its
// declaration names three subjects; the schema equality it then proves is two
// correspondence relations, and the complete heap vector its containment
// evidence needs is the heap read's own authenticated denominator.
func TestThePlacementFamilyJoinsItsSubjectsThroughCorrespondence(t *testing.T) {
	spec := placementquery.QuerySpec(census2GeometryTypes(t), census2PlacementSummaryTypes(t))
	if len(spec.Subjects) != 3 {
		t.Fatalf("placement subjects = %d, want the placement class, the heap vector and suspension evidence", len(spec.Subjects))
	}
	plan := census2PlacementSummaryPlan(t)
	if len(plan.rules) != 2 {
		t.Fatalf("placement derivations = %d, want the allocation child and the parent row", len(plan.rules))
	}
	completed := 0
	for _, join := range plan.rules[0].Joins {
		if join.Complete != nil {
			completed++
		}
	}
	if completed != 1 {
		t.Fatalf("placement completed reads = %d, want the complete heap vector its containment depends on", completed)
	}
	if len(plan.rules[0].Joins) != 6 {
		t.Fatalf("placement child joins = %d, want the registration, three subject reads and two correspondences", len(plan.rules[0].Joins))
	}
}

// TestEveryDiagnosticObservationKindHasOneCensus2Row states the matrix is total
// over the observation vocabulary. The inventory declares four kinds and their
// ordinals are ABI-bearing, so a fifth row or a missing row is a change to the
// identity preimage rather than to this test.
func TestEveryDiagnosticObservationKindHasOneCensus2Row(t *testing.T) {
	kinds := []structure.DiagnosticObservationKind{
		structure.DiagnosticObservationBranchCondition,
		structure.DiagnosticObservationTypeReferenceUnresolved,
		structure.DiagnosticObservationValueReferenceUnresolved,
		structure.DiagnosticObservationTypeConformance,
	}
	if len(structure.DiagnosticObservationSpecs()) != len(kinds) {
		t.Fatalf("declared observation kinds = %d, want %d", len(structure.DiagnosticObservationSpecs()), len(kinds))
	}
	surveyed := map[string]int{}
	for _, row := range census2Declared() {
		surveyed[row.Rule]++
	}
	for _, kind := range kinds {
		if surveyed[string(kind.Key())] != 1 {
			t.Fatalf("observation kind %s has %d census rows, want one", kind.Key(), surveyed[string(kind.Key())])
		}
	}
}

// TestTypeConformanceIsOneRelationWithFourArms states the measured shape of
// B4: the four authored sites that mint the kind are four alternative
// derivations of one relation, and closing that relation against the declared
// positions is a separate statement from producing its rows.
func TestTypeConformanceIsOneRelationWithFourArms(t *testing.T) {
	plan := census2TypeConformancePlan(t)
	arms := 0
	closures := 0
	for _, authored := range plan.rules {
		if authored.Complete != nil {
			closures++
			continue
		}
		if authored.Apply.Available() {
			arms++
		}
	}
	if arms != 4 {
		t.Fatalf("type-conformance arms = %d, want the call argument, the two assignment sites and the structural member", arms)
	}
	if closures != 1 {
		t.Fatalf("type-conformance closures = %d, want the one denominator over declared-typed positions", closures)
	}
}

// TestTheDiagnosticCodeRegisterIsTwoHalvesOfOneVocabulary states what the two
// code rows mean. Membership in the sealed table is not the line between them:
// two composed rows are named by the register as well, because their lane
// installs no collector. The line is the producer, which is why the composed
// half is a closure against the producer relation rather than a table lookup.
func TestTheDiagnosticCodeRegisterIsTwoHalvesOfOneVocabulary(t *testing.T) {
	declared := composite.DiagnosticsDeclaredNotComposed()
	if len(declared) == 0 {
		t.Fatal("the declared-not-composed register is empty")
	}
	for _, row := range declared {
		if row.Owner == "" || row.Reason == "" {
			t.Fatalf("declared code %s names no owing surface or missing observation", row.Code)
		}
	}
	composed := census2DiagnosticComposedPlan(t)
	if len(composed.rules) != 1 || composed.rules[0].Publish != nil {
		t.Fatal("the composed half is not a read-only relation of the code vocabulary")
	}
	if composed.rules[0].Joins[0].Complete == nil {
		t.Fatal("the composed half does not close the code relation against its producer")
	}
	registered := census2DiagnosticDeclaredPlan(t)
	if len(registered.rules) != 1 || registered.rules[0].Publish != nil {
		t.Fatal("the declared half is not a read-only relation of the code vocabulary")
	}
}

// TestEveryIssuanceSeedResolvesThroughTheLandedRegistry states the seeds'
// coverage without restating their plans. A seed is a rule declaration with an
// authored Program, so it is already one row of the rule census; what the
// non-rule half asserts is that every one of those rows lowers, and lowers to
// the issuance shape - the seed rows a candidate relation holds, reduced by the
// owner's own reducer and published into its factor.
func TestEveryIssuanceSeedResolvesThroughTheLandedRegistry(t *testing.T) {
	pinned, err := os.ReadFile(filepath.Join("testdata", "census.json"))
	if err != nil {
		t.Fatalf("read pinned rule census: %v", err)
	}
	var rows []entry
	if err := json.Unmarshal(pinned, &rows); err != nil {
		t.Fatalf("decode pinned rule census: %v", err)
	}
	seeds := 0
	for _, row := range rows {
		if row.Plane != "seed" {
			continue
		}
		seeds++
		if row.Status != statusCompiles {
			t.Fatalf("seed %s does not lower: %s", row.Family, row.Status)
		}
		if row.Sketch != "Publish(Apply(Select(Input)))" && row.Sketch != "Publish(Merge(Apply(Select(Input)),Apply(Select(Input))))" {
			t.Fatalf("seed %s lowers to %q, want an issuance publication with or without its carry", row.Family, row.Sketch)
		}
	}
	if seeds == 0 {
		t.Fatal("the rule census holds no issuance seed rows")
	}
}

// TestEveryCensus2RowNamesTheOwnerStatementsItStillNeeds states what a green
// row does and does not prove. Lowering proves the declaration is expressible
// in the frozen grammar; it does not prove the relations the plan names are
// declared. Every requirement a row states must be a relation the plan itself
// names, so the column is a reading of the plan and never a list beside it.
func TestEveryCensus2RowNamesTheOwnerStatementsItStillNeeds(t *testing.T) {
	total := 0
	for _, specimen := range census2Declared() {
		row := census2Survey(t, specimen)
		if len(row.Requires) == 0 {
			t.Fatalf("%s states no owner statement it needs; every non-rule plane reads relations no surface declares today", row.Family)
		}
		total += len(row.Requires)
	}
	if total == 0 {
		t.Fatal("the non-rule half reports no declaration work")
	}
}

// TestEveryCensus2RowLowersOrNamesItsMissingOwnerStatement states the shape of
// the matrix's findings. A row either carries a complete logical plan or names
// the exact declaration site and the owner statement it is waiting on; a row
// that reports neither is a lowering that guessed.
func TestEveryCensus2RowLowersOrNamesItsMissingOwnerStatement(t *testing.T) {
	for _, specimen := range census2Declared() {
		row := census2Survey(t, specimen)
		if row.Reason == "" {
			t.Fatalf("%s states nothing about what its declaration lowers to", row.Family)
		}
		if row.Status == statusCompiles {
			if row.Sketch == "" || row.Expressions == 0 {
				t.Fatalf("%s compiles to no logical plan", row.Family)
			}
			continue
		}
		if row.Site == "" || row.Missing == "" {
			t.Fatalf("%s reports a finding without naming the missing owner statement", row.Family)
		}
	}
}
