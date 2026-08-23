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
// outside the table is not a form at all. A form that carries a classifier
// carries a name, so nothing can be classified into an anonymous ladder slot.
func TestExecutionFormTableIsTotal(t *testing.T) {
	for form := Form(1); form < formCount; form++ {
		if !form.Declared() {
			t.Fatalf("form %d inside the table is not declared", form)
		}
		if form.Name() == "" {
			t.Fatalf("declared form %d has no name", form)
		}
		if formClassifiers[form] == nil {
			t.Fatalf("declared form %q has no classifier", form.Name())
		}
	}
	for _, form := range []Form{0, formCount, formCount + 1, 250} {
		if form.Declared() || form.Name() != "" {
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
	plane, planeOK := NewFormPlane(fixture.binding, nil, nil, nil)
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
	plane, planeOK := NewFormPlane(fixture.binding, []memberrelation.SourceColumn[uint64]{column}, []bool{true}, nil)
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

// TestExecutionFormClassificationIsExclusive states that one sealed descriptor
// belongs to exactly one form. Two forms claiming the same plan is a table
// defect, and the registry refuses it rather than letting probe order decide
// which executor a rule gets.
func TestExecutionFormClassificationIsExclusive(t *testing.T) {
	rule := planCompiledExactRule(t)
	claim := func(form Form) formClassifier {
		return func(generated.CompiledRule) (FormRow, bool) { return FormRow{Form: form}, true }
	}
	var overlapping [formCount]formClassifier
	overlapping[FormExact] = claim(FormExact)
	overlapping[FormSource] = claim(FormSource)
	if row, ok := classifyForm(rule, overlapping); ok {
		t.Fatalf("two claiming forms classified as %q", row.Form.Name())
	}
	var mislabelled [formCount]formClassifier
	mislabelled[FormExact] = claim(FormSource)
	if row, ok := classifyForm(rule, mislabelled); ok {
		t.Fatalf("classifier registered under exact answered %q", row.Form.Name())
	}
	var single [formCount]formClassifier
	single[FormSource] = claim(FormSource)
	row, ok := classifyForm(rule, single)
	if !ok || row.Form != FormSource {
		t.Fatalf("single claiming form classified as %q/%t", row.Form.Name(), ok)
	}
}

// TestExecutionFormClassifiesTheSealedPlan states that the real classifier
// column reads the sealed descriptor: a one-join exact-output rule is the E
// form at its declared read port, and a read-free rule over its own candidate
// relation is the Z form at that relation member.
func TestExecutionFormClassifiesTheSealedPlan(t *testing.T) {
	exact, ok := ClassifyForm(planCompiledExactRule(t))
	if !ok || exact.Form != FormExact || exact.Input != 0 {
		t.Fatalf("exact plan classified as %q input %d/%t", exact.Form.Name(), exact.Input, ok)
	}
	source, ok := ClassifyForm(planCompiledSourceRule(t))
	if !ok || source.Form != FormSource || source.Relation != 4 {
		t.Fatalf("source plan classified as %q relation %d/%t", source.Form.Name(), source.Relation, ok)
	}
	if row, ok := ClassifyForm(generated.CompiledRule{}); ok {
		t.Fatalf("unavailable descriptor classified as %q", row.Form.Name())
	}
}

// planCompiledExactRule seals the smallest one-join exact-output descriptor:
// one read on port 0 publishing the candidate axis it joins.
func planCompiledExactRule(t *testing.T) generated.CompiledRule {
	t.Helper()
	rule, ok := generated.NewPlanCompiledRule(generated.CompiledRuleSpec{
		Ordinal: 7, AxisCount: 3, InputCount: 1,
		Candidate: ruleplan.RelationAddr{Axis: 0, Member: 0},
		Reducer:   ruleplan.ReducerAddr{Axis: 2, Member: 0},
		Reads: []generated.ReadPlan{{
			Input: 0, Factor: 1, Axis: 0,
			Relation: ruleplan.RelationAddr{Axis: 0, Member: 0}, Key: ruleplan.ProjectionAddr{Axis: 0, Member: 0},
			Form:        ruleprogram.Exact,
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
		Ordinal: 8, AxisCount: 3, InputCount: 0,
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

// installedFamilyProvider is a test owner that installs the family of exactly
// one sealed rule ordinal, the way a domain arm authored with concrete types
// does.
type installedFamilyProvider struct {
	rule    uint32
	refuse  bool
	calls   int
	install Family
}

func (provider *installedFamilyProvider) AuthorsRule(rule uint32) bool {
	return provider != nil && rule == provider.rule
}

func (provider *installedFamilyProvider) InstallRuleFamily(rule uint32, rows []FormRow) (Family, []FormAddress, bool) {
	if provider == nil || rule != provider.rule {
		return nil, nil, false
	}
	provider.calls++
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
	ordinal, ordinalOK := exactRule.Ordinal()
	if !ordinalOK {
		t.Fatal("sealed rule ordinal")
	}
	generic := FormRow{Member: 0, Form: FormExact, Input: 0, Unit: fixture.unit, Target: fixture.target, Rule: exactRule}
	owned := func(member int) FormRow {
		return FormRow{Member: member, Form: FormExact, Input: 0, Unit: fixture.unit, Target: fixture.target, Rule: exactRule}
	}

	t.Run("installed", func(t *testing.T) {
		provider := &installedFamilyProvider{rule: ordinal, install: installedFamily{}}
		plane, planeOK := NewFormPlane(fixture.binding, nil, nil, provider)
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
		plane, planeOK := NewFormPlane(fixture.binding, nil, nil, provider)
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
		plane, planeOK := NewFormPlane(fixture.binding, nil, nil, provider)
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
	plane, planeOK := NewFormPlane(fixture.binding, nil, nil, nil)
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
