package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/generated"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// TestExecutionFormTableIsTotal states the registry's totality law: every
// sealed ordinal of the append-only form table names itself, and an ordinal
// outside the table is not a form at all. A derived row may only ever carry a
// declared ordinal, so nothing can be routed into an anonymous ladder slot.
func TestExecutionFormTableIsTotal(t *testing.T) {
	for form := Form(1); form < formCount; form++ {
		if !form.Declared() {
			t.Fatalf("form %d inside the table is not declared", form)
		}
		if form.Name() == "" {
			t.Fatalf("declared form %d has no name", form)
		}
		if form.claimed() != form {
			t.Fatalf("declared form %q is not claimable by a derived row", form.Name())
		}
	}
	for _, form := range []Form{0, formCount, formCount + 1, 250} {
		if form.Declared() || form.Name() != "" || form.claimed() != 0 {
			t.Fatalf("form %d outside the table reports declared=%t name=%q", form, form.Declared(), form.Name())
		}
	}
}

// TestExecutionFormWithoutImplementationRefusesByName states the other half of
// totality: a declared form the typed implementation column does not cover
// refuses the build and names the form it could not execute. A plan row is
// never silently dropped from the ladder or folded into a neighbouring form.
func TestExecutionFormWithoutImplementationRefusesByName(t *testing.T) {
	fixture := newExecutionFixture(t)
	plane, planeOK := NewFormPlane(fixture.binding, nil, nil, RouteTable{}, nil, nil)
	if !planeOK {
		t.Fatal("form plane")
	}
	builders := formBuilders[uint64, uint64]()
	builders[FormSource] = nil
	rows := []FormRow{{Member: 0, Form: FormSource, Relation: 0, Target: fixture.target}}
	families, addresses, refused, built := buildForms(plane, rows, builders)
	if built || families != nil || addresses != nil {
		t.Fatalf("unimplemented form built %d families / %d addresses", len(families), len(addresses))
	}
	if refused != FormSource || refused.Name() != "source" {
		t.Fatalf("refusal names form %d/%q, want source", refused, refused.Name())
	}
}

// TestExecutionFormsBuildInSealedOrdinalOrder states that the ladder a Factor
// receives is ordered by sealed form ordinal, not by the order rows were
// discovered: the same plan rows in any order produce the same family offsets
// and the same locals.
func TestExecutionFormsBuildInSealedOrdinalOrder(t *testing.T) {
	fixture := newExecutionFixture(t)
	column, columnOK := memberrelation.NewSourceColumn([]uint64{7}, []structure.ReductionOutcome{structure.Concrete})
	if !columnOK {
		t.Fatal("sealed source column")
	}
	plane, planeOK := NewFormPlane(fixture.binding, []memberrelation.SourceColumn[uint64]{column}, []bool{true}, RouteTable{}, nil, nil)
	if !planeOK {
		t.Fatal("form plane")
	}
	source := FormRow{Member: 0, Form: FormSource, Relation: 0, Target: fixture.target}
	exact := FormRow{Member: 1, Form: FormExact, Input: 0, Unit: fixture.unit, Target: fixture.target}
	for _, testCase := range []struct {
		name string
		rows []FormRow
	}{
		{name: "source-first", rows: []FormRow{source, exact}},
		{name: "exact-first", rows: []FormRow{exact, source}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			families, addresses, refused, built := BuildForms(plane, testCase.rows)
			if !built || refused != 0 {
				t.Fatalf("build = %t refused %q", built, refused.Name())
			}
			if len(families) != 2 || families[0].InputCapacity() != 1 || families[1].InputCapacity() != 0 {
				t.Fatalf("ladder = %d families with capacities %v", len(families), families)
			}
			if len(addresses) != 2 {
				t.Fatalf("addresses = %d, want 2", len(addresses))
			}
			if addresses[0] != (FormAddress{Member: 1, FamilyOffset: 0, Local: 0}) {
				t.Fatalf("exact address = %+v", addresses[0])
			}
			if addresses[1] != (FormAddress{Member: 0, FamilyOffset: 1, Local: 0}) {
				t.Fatalf("source address = %+v", addresses[1])
			}
		})
	}
}

// TestExecutionFormIsDerivedFromTheDeclaration states what replaced the
// exclusivity of a classifier column.
//
// A descriptor's form is DERIVED from what it declares - its publication mode,
// its carry disposition and its read vocabulary - so it is single-valued by
// construction and cannot depend on the order independent probes were tried.
// A column of probes could only approximate that, and three of the six forms
// have no generic builder at all, so their probes were refusing geometry that
// nothing behind them implements.
//
// The derivation is also a function of the declaration alone: deriving the
// same descriptor twice answers the same row.
func TestExecutionFormIsDerivedFromTheDeclaration(t *testing.T) {
	for _, testCase := range []struct {
		name string
		rule generated.CompiledRule
		want Form
	}{
		{name: "exact", rule: planCompiledExactRule(t), want: FormExact},
		{name: "exact product", rule: planCompiledExactProductRule(t), want: FormExact},
		{name: "source", rule: planCompiledSourceRule(t), want: FormSource},
		{name: "transformed carry", rule: planCompiledTransformedCarryRule(t), want: FormCarry},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			first, firstOK := DeclaredForm(testCase.rule)
			if !firstOK || first.Form != testCase.want {
				t.Fatalf("derived %q/%t, want %q", first.Form.Name(), firstOK, testCase.want.Name())
			}
			if !first.Form.Declared() {
				t.Fatalf("derived an undeclared ordinal %d", first.Form)
			}
			second, secondOK := DeclaredForm(testCase.rule)
			if !secondOK || second.Form != first.Form || second.Input != first.Input || second.Relation != first.Relation {
				t.Fatalf("two derivations of one declaration answered %+v and %+v", first, second)
			}
		})
	}
	if row, ok := DeclaredForm(generated.CompiledRule{}); ok {
		t.Fatalf("an unavailable descriptor derived %q", row.Form.Name())
	}
}

// TestExecutionFormDerivesTheSealedPlan states that the derivation reads the
// sealed descriptor: a one-join exact-output rule is the E form at its
// declared read port, and a read-free rule over its own candidate relation is
// the Z form at that relation member.
func TestExecutionFormDerivesTheSealedPlan(t *testing.T) {
	exact, ok := DeclaredForm(planCompiledExactRule(t))
	if !ok || exact.Form != FormExact || exact.Input != 0 {
		t.Fatalf("exact plan derived as %q input %d/%t", exact.Form.Name(), exact.Input, ok)
	}
	product, ok := DeclaredForm(planCompiledExactProductRule(t))
	if !ok || product.Form != FormExact || product.Rule.ReadCount() != 2 {
		t.Fatalf("exact product derived as %q reads %d/%t", product.Form.Name(), product.Rule.ReadCount(), ok)
	}
	source, ok := DeclaredForm(planCompiledSourceRule(t))
	if !ok || source.Form != FormSource || source.Relation != 4 {
		t.Fatalf("source plan derived as %q relation %d/%t", source.Form.Name(), source.Relation, ok)
	}
	if row, ok := DeclaredForm(generated.CompiledRule{}); ok {
		t.Fatalf("unavailable descriptor derived as %q", row.Form.Name())
	}
}

// planCompiledExactProductRule seals the common binary-domain shape: two
// owner-issued exact joins on one predecessor port, one exact publication,
// and identity carry of the written Factor.
func planCompiledExactProductRule(t *testing.T) generated.CompiledRule {
	t.Helper()
	contract := ruleplan.ReadContract{Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne}
	rule, ok := generated.NewPlanCompiledRule(generated.CompiledRuleSpec{
		AxisCount: 3, InputCount: 1,
		Candidate: ruleplan.RelationAddr{Axis: 0, Member: 0},
		Reducer:   ruleplan.ReducerAddr{Axis: 2, Member: 0},
		Reads: []generated.ReadPlan{
			{Input: 0, Factor: 1, Axis: 0, Relation: ruleplan.RelationAddr{Axis: 0, Member: 0}, Key: ruleplan.ProjectionAddr{Axis: 0, Member: 0}, Addressing: ruleplan.RelationAddr{Axis: 0, Member: 0}, AddressingPresent: true, Form: ruleprogram.Exact, PointBound: ruleprogram.PointBound, Contract: contract, RowCapacity: 2, CellCapacity: 1},
			{Input: 0, Factor: 1, Axis: 0, Relation: ruleplan.RelationAddr{Axis: 0, Member: 1}, Key: ruleplan.ProjectionAddr{Axis: 0, Member: 1}, Addressing: ruleplan.RelationAddr{Axis: 0, Member: 0}, AddressingPresent: true, Form: ruleprogram.Exact, PointBound: ruleprogram.PointBound, Contract: contract, RowCapacity: 2, CellCapacity: 1},
		},
		Outputs: []generated.OutputPlan{{
			Factor: 2, Axis: 2, Address: ruleplan.OutputAddr{Axis: 2, Frame: 0},
			Destination: ruleplan.ProjectionAddr{Axis: 0, Member: 2}, Mode: ruleprogram.ModeExact, Exact: true, Strong: true,
		}},
		Carry: &generated.CarryPlan{Input: 0, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true},
	})
	if !ok {
		t.Fatal("sealed exact-product plan")
	}
	return rule
}

// planCompiledExactRule seals the smallest one-join exact-output descriptor:
// one read on port 0 publishing the candidate axis it joins.
func planCompiledExactRule(t *testing.T) generated.CompiledRule {
	t.Helper()
	rule, ok := generated.NewPlanCompiledRule(generated.CompiledRuleSpec{
		AxisCount: 3, InputCount: 1,
		Candidate: ruleplan.RelationAddr{Axis: 0, Member: 0},
		Reducer:   ruleplan.ReducerAddr{Axis: 2, Member: 0},
		Reads: []generated.ReadPlan{{
			Input: 0, Factor: 1, Axis: 0,
			Relation: ruleplan.RelationAddr{Axis: 0, Member: 0}, Key: ruleplan.ProjectionAddr{Axis: 0, Member: 0},
			Addressing: ruleplan.RelationAddr{Axis: 0, Member: 0}, AddressingPresent: true,
			Form:        ruleprogram.Exact,
			PointBound:  ruleprogram.PointBound,
			Contract:    ruleplan.ReadContract{Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne},
			RowCapacity: 1, CellCapacity: 1,
		}},
		Outputs: []generated.OutputPlan{{
			Factor: 2, Axis: 2, Address: ruleplan.OutputAddr{Axis: 2, Frame: 0},
			Destination: ruleplan.ProjectionAddr{Axis: 0, Member: 0}, Mode: ruleprogram.ModeExact, Exact: true, Strong: true,
		}},
		Carry: &generated.CarryPlan{Input: 0, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true},
	})
	if !ok {
		t.Fatal("sealed exact plan")
	}
	return rule
}

// planCompiledSourceRule seals the read-free descriptor whose output factor
// publishes the candidate relation it writes.
func planCompiledSourceRule(t *testing.T) generated.CompiledRule {
	t.Helper()
	rule, ok := generated.NewPlanCompiledRule(generated.CompiledRuleSpec{
		AxisCount: 3, InputCount: 0,
		Candidate: ruleplan.RelationAddr{Axis: 0, Member: 4},
		Reducer:   ruleplan.ReducerAddr{Axis: 0, Member: 0},
		Reads:     []generated.ReadPlan{},
		Outputs: []generated.OutputPlan{{
			Factor: 0, Axis: 0, Address: ruleplan.OutputAddr{Axis: 0, Frame: 0},
			Destination: ruleplan.ProjectionAddr{Axis: 0, Member: 0}, Mode: ruleprogram.ModeExact, Exact: true, Strong: true,
		}},
	})
	if !ok {
		t.Fatal("sealed source plan")
	}
	return rule
}

// installedFamilyProvider is a test rule package that installs the family of
// its own sealed rule ordinal, the way a rule authored with concrete types
// does.
type installedFamilyProvider struct {
	rule    uint32
	refuse  bool
	calls   int
	install Family
	// rows are the plan rows the table actually handed this installer, which
	// is what proves WHICH rows its claim covered.
	rows []FormRow
}

// fixtureForeignTable is the Program-wide Factor read table: one entry per
// sealed Factor ordinal, all typed in this fixture's one algebra. A real
// Program's entries differ per Factor; the fence a plane applies to them does
// not.
func fixtureForeignTable(t testing.TB, fixture executionFixture, width int) []ForeignFactor {
	t.Helper()
	read, readOK := NewForeignFactor(fixture.binding, RouteTable{})
	if !readOK {
		t.Fatal("foreign read side")
	}
	table := make([]ForeignFactor, width)
	for index := range table {
		table[index] = read
	}
	return table
}

// lawRuleTableWidth is the sealed rule table these laws claim positions in.
const lawRuleTableWidth = 32

// lawExactRuleOrdinal is the position planCompiledExactRule occupies in that
// table. The descriptor carries no ordinal of its own, so a law that installs
// a family for it names the position it was sealed at.
const lawExactRuleOrdinal uint32 = 7

// newLawFormPlane is the bound plane these laws build forms through: the
// fixture's own typed binding, a Program-wide read table, and one claim table.
func newLawFormPlane(t testing.TB, fixture executionFixture, families *RuleFamilies[uint64, uint64]) FormPlane[uint64, uint64] {
	t.Helper()
	plane, planeOK := NewFormPlane(fixture.binding, nil, nil, RouteTable{}, fixtureForeignTable(t, fixture, 3), families)
	if !planeOK {
		t.Fatal("law form plane")
	}
	return plane
}

func newLawRuleFamilies() *RuleFamilies[uint64, uint64] {
	families, opened := NewRuleFamilies[uint64, uint64](lawRuleTableWidth)
	if !opened {
		panic("law rule family table")
	}
	return families
}

// ruleFamilyTable is the sealed authorship table one provider claims its own
// ordinal in, which is what a Factor hands the plane at bind.
func ruleFamilyTable(provider *installedFamilyProvider) *RuleFamilies[uint64, uint64] {
	families := newLawRuleFamilies()
	families.Install(provider.rule, provider)
	return families
}

func (provider *installedFamilyProvider) InstallRuleFamily(plane FormPlane[uint64, uint64], rule uint32, rows []FormRow) (Family, []FormAddress, bool) {
	if !plane.Valid() {
		return nil, nil, false
	}
	if provider == nil || rule != provider.rule {
		return nil, nil, false
	}
	provider.calls++
	provider.rows = append(provider.rows, rows...)
	if provider.refuse {
		return nil, nil, false
	}
	addresses := make([]FormAddress, 0, len(rows))
	for index, row := range rows {
		addresses = append(addresses, FormAddress{Member: row.Member, Local: uint32(index)})
	}
	return provider.install, addresses, true
}

type installedFamily struct{}

func (installedFamily) NewExecutor(*Run) Executor { return nil }
func (installedFamily) InputCapacity() int        { return 1 }
func (installedFamily) OutputCapacity() int       { return 1 }

// TestAnOwnerInstallsTheFamilyOfItsOwnRule states the rule-level handoff: a
// Factor that authors an execution family for one of its sealed rules with
// concrete types has that family installed for every plan row of that rule,
// while every other row still builds through its form. This is the only way a
// rule whose joins or reducer are typed outside the written Factor reaches
// execution - the engine cannot name those types - and it is the same handoff
// shape a materialized source column already uses.
func TestAnOwnerInstallsTheFamilyOfItsOwnRule(t *testing.T) {
	fixture := newExecutionFixture(t)
	exactRule := planCompiledExactRule(t)
	ordinal := lawExactRuleOrdinal
	generic := FormRow{Member: 0, Form: FormExact, Input: 0, Unit: fixture.unit, Target: fixture.target, Rule: exactRule, RuleOrdinal: lawExactRuleOrdinal}
	owned := func(member int) FormRow {
		return FormRow{Member: member, Form: FormExact, Input: 0, Unit: fixture.unit, Target: fixture.target, Rule: exactRule, RuleOrdinal: lawExactRuleOrdinal}
	}

	t.Run("installed", func(t *testing.T) {
		provider := &installedFamilyProvider{rule: ordinal, install: installedFamily{}}
		plane, planeOK := NewFormPlane(fixture.binding, nil, nil, RouteTable{}, fixtureForeignTable(t, fixture, 3), ruleFamilyTable(provider))
		if !planeOK {
			t.Fatal("form plane")
		}
		families, addresses, refused, built := BuildForms(plane, []FormRow{owned(3), owned(4)})
		if !built || refused != 0 || provider.calls != 1 {
			t.Fatalf("install = %t refused %q calls %d", built, refused.Name(), provider.calls)
		}
		if len(families) != 1 || families[0] != provider.install {
			t.Fatalf("ladder = %d families, want the installed one", len(families))
		}
		if len(addresses) != 2 || addresses[0] != (FormAddress{Member: 3, Local: 0}) || addresses[1] != (FormAddress{Member: 4, Local: 1}) {
			t.Fatalf("installed addresses = %+v", addresses)
		}
	})

	t.Run("not-authored", func(t *testing.T) {
		provider := &installedFamilyProvider{rule: ordinal + 1, install: installedFamily{}}
		plane, planeOK := NewFormPlane(fixture.binding, nil, nil, RouteTable{}, fixtureForeignTable(t, fixture, 3), ruleFamilyTable(provider))
		if !planeOK {
			t.Fatal("form plane")
		}
		families, addresses, refused, built := BuildForms(plane, []FormRow{generic})
		if !built || refused != 0 || provider.calls != 0 {
			t.Fatalf("unauthored rule = %t refused %q calls %d", built, refused.Name(), provider.calls)
		}
		if len(families) != 1 || families[0] == provider.install || len(addresses) != 1 {
			t.Fatalf("unauthored rule did not build through its form: %d families", len(families))
		}
	})

	t.Run("refused-install-is-a-refusal", func(t *testing.T) {
		provider := &installedFamilyProvider{rule: ordinal, refuse: true, install: installedFamily{}}
		plane, planeOK := NewFormPlane(fixture.binding, nil, nil, RouteTable{}, fixtureForeignTable(t, fixture, 3), ruleFamilyTable(provider))
		if !planeOK {
			t.Fatal("form plane")
		}
		families, addresses, refused, built := BuildForms(plane, []FormRow{owned(0)})
		if built || families != nil || addresses != nil {
			t.Fatalf("a refused install still produced %d families", len(families))
		}
		if refused != FormExact {
			t.Fatalf("refusal names %q, want the row's form", refused.Name())
		}
	})
}

// TestAFormRefusesACoordinateOfAnotherFactor states the guarantee that lets a
// generated member bind a read on a Factor other than the one it writes: the
// binding decides. A row whose Unit or Target was minted by another Factor's
// binding is refused where the family is sealed, so a foreign join reaches
// execution only through the family its owner installs and never by being
// silently sealed onto the writing Factor's plane.
func TestAFormRefusesACoordinateOfAnotherFactor(t *testing.T) {
	fixture := newExecutionFixture(t)
	foreign := newExecutionFixture(t)
	plane, planeOK := NewFormPlane(fixture.binding, nil, nil, RouteTable{}, nil, nil)
	if !planeOK {
		t.Fatal("form plane")
	}
	for _, testCase := range []struct {
		name string
		row  FormRow
	}{
		{name: "foreign-unit", row: FormRow{Member: 0, Form: FormExact, Unit: foreign.unit, Target: fixture.target}},
		{name: "foreign-target", row: FormRow{Member: 0, Form: FormExact, Unit: fixture.unit, Target: foreign.target}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if families, addresses, _, built := BuildForms(plane, []FormRow{testCase.row}); built {
				t.Fatalf("a foreign coordinate sealed %d families / %d addresses", len(families), len(addresses))
			}
		})
	}
}

// TestOneRuleOrdinalHasOneFamilyAuthority is why authorship is a table rather
// than a predicate each installer answers for itself. Two installers claiming
// one ordinal is two authorities over one rule's execution, and no order
// between them resolves it - so the second claim is refused where it is made,
// before anything is built and while the claimant can still be named.
func TestOneRuleOrdinalHasOneFamilyAuthority(t *testing.T) {
	first := &installedFamilyProvider{rule: 7, install: installedFamily{}}
	second := &installedFamilyProvider{rule: 7, install: installedFamily{}}
	families := newLawRuleFamilies()
	if !families.Install(first.rule, first) {
		t.Fatal("the first claim on an unclaimed ordinal is refused")
	}
	if families.Install(second.rule, second) {
		t.Fatal("a second installer claimed an ordinal that already has one")
	}
	installer, authored := families.Installer(7)
	if !authored || installer != RuleFamilyInstaller[uint64, uint64](first) {
		t.Fatal("the refused second claim displaced the first")
	}
	if _, authored := families.Installer(8); authored {
		t.Fatal("an unclaimed ordinal reports an authority")
	}
	if families.Count() != 1 {
		t.Fatalf("table holds %d claims after one admitted and one refused", families.Count())
	}
}

// TestAnEmptyFamilyTableAuthorsNothing keeps the common case - a Factor no rule
// installs a family for - reading through the generic form builders rather
// than refusing.
func TestAnEmptyFamilyTableAuthorsNothing(t *testing.T) {
	var absent *RuleFamilies[uint64, uint64]
	if _, authored := absent.Installer(0); authored {
		t.Fatal("a Factor with no installed families authors one")
	}
	if absent.Count() != 0 {
		t.Fatal("an absent table holds claims")
	}
	empty := newLawRuleFamilies()
	if _, authored := empty.Installer(0); authored {
		t.Fatal("an empty table authors an ordinal")
	}
	if empty.Install(0, nil) {
		t.Fatal("a nil installer claimed an ordinal")
	}
}

// foreignFenceProvider records which input axes the plane handed it. It is the
// installer half of the foreign fence: what a rule can seal a read against is
// decided by the plane it is given, not by anything the installer asks for.
type foreignFenceProvider struct {
	rule     uint32
	resolved map[uint32]bool
	width    int
}

func (provider *foreignFenceProvider) InstallRuleFamily(plane FormPlane[uint64, uint64], rule uint32, rows []FormRow) (Family, []FormAddress, bool) {
	if provider == nil || rule != provider.rule || !plane.Valid() {
		return nil, nil, false
	}
	provider.resolved = make(map[uint32]bool, provider.width)
	for factor := 0; factor < provider.width; factor++ {
		_, ok := plane.Foreign(uint32(factor))
		provider.resolved[uint32(factor)] = ok
	}
	addresses := make([]FormAddress, 0, len(rows))
	for index, row := range rows {
		addresses = append(addresses, FormAddress{Member: row.Member, Local: uint32(index)})
	}
	return installedFamily{}, addresses, true
}

// TestAnInstallerReadsOnlyTheInputAxesItsPlanDeclared is the foreign fence.
// A fold's dependencies are the joins its sealed plan states, and the solver
// schedules the rule against exactly those: a read of any other Factor would
// observe a fact nothing waited for, at whatever value that Factor happened to
// hold. The plane an installer receives therefore resolves the input axes its
// own rule declared and refuses every other Factor by name, so the mistake is
// unrepresentable rather than merely unlikely.
func TestAnInstallerReadsOnlyTheInputAxesItsPlanDeclared(t *testing.T) {
	fixture := newExecutionFixture(t)
	exactRule := planCompiledExactRule(t)
	ordinal := lawExactRuleOrdinal
	declared, declaredOK := exactRule.ReadAt(0)
	if !declaredOK {
		t.Fatal("sealed rule read")
	}
	provider := &foreignFenceProvider{rule: ordinal, width: 3}
	families := newLawRuleFamilies()
	if !families.Install(ordinal, provider) {
		t.Fatal("family claim")
	}
	plane, planeOK := NewFormPlane(fixture.binding, nil, nil, RouteTable{}, fixtureForeignTable(t, fixture, 3), families)
	if !planeOK {
		t.Fatal("form plane")
	}
	row := FormRow{Member: 0, Form: FormExact, Input: 0, Unit: fixture.unit, Target: fixture.target, Rule: exactRule, RuleOrdinal: lawExactRuleOrdinal}
	if _, _, _, built := BuildForms(plane, []FormRow{row}); !built {
		t.Fatal("installed rule did not build")
	}
	if len(provider.resolved) != 3 {
		t.Fatalf("installer probed %d factors, want 3", len(provider.resolved))
	}
	for factor, resolved := range provider.resolved {
		want := factor == declared.Factor
		if resolved != want {
			t.Fatalf("factor %d resolved=%t, want %t (the plan declares a join on %d only)", factor, resolved, want, declared.Factor)
		}
	}
}

// TestAPlaneRefusesAnInputAxisTheProgramDoesNotHave states the outer bound of
// the same fence. A rule whose plan names a read Factor the Program's read
// table has no entry for is a plan and a Program that disagree about how many
// Factors exist; nothing downstream can repair that, so the family refuses
// where it would be sealed.
func TestAPlaneRefusesAnInputAxisTheProgramDoesNotHave(t *testing.T) {
	fixture := newExecutionFixture(t)
	exactRule := planCompiledExactRule(t)
	ordinal := lawExactRuleOrdinal
	provider := &foreignFenceProvider{rule: ordinal, width: 1}
	families := newLawRuleFamilies()
	if !families.Install(ordinal, provider) {
		t.Fatal("family claim")
	}
	// The plan reads Factor 1; the Program's table names one Factor.
	plane, planeOK := NewFormPlane(fixture.binding, nil, nil, RouteTable{}, fixtureForeignTable(t, fixture, 1), families)
	if !planeOK {
		t.Fatal("form plane")
	}
	row := FormRow{Member: 0, Form: FormExact, Input: 0, Unit: fixture.unit, Target: fixture.target, Rule: exactRule, RuleOrdinal: lawExactRuleOrdinal}
	if _, _, _, built := BuildForms(plane, []FormRow{row}); built {
		t.Fatal("a rule reading a Factor the Program does not have was sealed")
	}
	if provider.resolved != nil {
		t.Fatal("the installer was reached with an unresolvable read table")
	}
}
