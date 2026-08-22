package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

// This file pins the seal-availability-at-construction law for the equation
// types whose Available() accessor now reads a verdict its sole constructor
// already proved, instead of re-walking the fields that proved it. Each pair
// of tests below mirrors linkexecutionplan's BoundEdge law: one proves the
// accessor is a pure, allocation-free read that cannot be flipped by
// detaching the fields the old body used to walk, and one proves the old
// negative case now refuses at construction rather than at the accessor.

// TestScopeAvailabilityIsSealedAtConstruction pins that Scope.Available reads
// the sealed row pointer NewScope issues, not a re-derived key check: a copy
// pointed at a row whose key was cleared stays available, because NewScope is
// the sole issuer of a non-nil row and never issues one with an unavailable
// key.
func TestScopeAvailabilityIsSealedAtConstruction(t *testing.T) {
	decision := boundaryDecision(t, 1)
	scope, ok := NewScope(decision)
	if !ok || !scope.Available() {
		t.Fatal("sealed scope unavailable")
	}
	detached := scope
	corrupted := *scope.row
	corrupted.key = composition.Key{}
	detached.row = &corrupted
	if detached.Available() != scope.Available() {
		t.Fatal("Available re-derives the row key instead of reading the sealed row pointer")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = scope.Available() }); allocs != 0 {
		t.Fatalf("Available allocates %v per call", allocs)
	}
	if (Scope{}).Available() {
		t.Fatal("zero scope available")
	}
}

// TestScopeRefusesMalformedConstruction pins that a duplicate or unavailable
// decision is refused by NewScope itself.
func TestScopeRefusesMalformedConstruction(t *testing.T) {
	decision := boundaryDecision(t, 2)
	if scope, ok := NewScope(decision, decision); ok || scope.Available() {
		t.Fatal("a duplicate decision scope was sealed")
	}
	if scope, ok := NewScope(Decision{}); ok || scope.Available() {
		t.Fatal("an unavailable decision scope was sealed")
	}
}

// TestExprAvailabilityIsSealedAtConstruction pins that Expr.Available reads
// the sealed valid bit rather than re-checking the root/node bound: clearing
// nodes on a copy cannot flip the verdict, because every constructor proves
// the bound before it ever sets valid.
func TestExprAvailabilityIsSealedAtConstruction(t *testing.T) {
	decision := boundaryDecision(t, 3)
	expr, ok := DecisionExpr(decision)
	if !ok || !expr.Available() {
		t.Fatal("sealed expr unavailable")
	}
	detached := expr
	detached.nodes = nil
	if !detached.Available() {
		t.Fatal("Available re-derives the node bound instead of reading the sealed valid bit")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = expr.Available() }); allocs != 0 {
		t.Fatalf("Available allocates %v per call", allocs)
	}
	if (Expr{}).Available() {
		t.Fatal("zero expr available")
	}
}

// TestExprRefusesMalformedConstruction pins that an unavailable decision node
// and an out-of-range root are refused by NewExprDAGWithCheckpoint itself.
func TestExprRefusesMalformedConstruction(t *testing.T) {
	if expr, ok := NewExprDAGWithCheckpoint([]ExprNode{{Decision: Decision{}, Low: 0, High: 1}}, 2, nil); ok || expr.Available() {
		t.Fatal("an unavailable decision node was sealed")
	}
	if expr, ok := NewExprDAGWithCheckpoint(nil, 5, nil); ok || expr.Available() {
		t.Fatal("an out-of-range root was sealed")
	}
}

// TestRelationAvailabilityIsSealedAtConstruction pins that Relation.Available
// reads the owner Publish and sealInitialRelation set, not a re-check of
// generation/digest: both are already proven available before either
// constructor ever sets owner, so clearing them on a copy cannot flip the
// verdict.
func TestRelationAvailabilityIsSealedAtConstruction(t *testing.T) {
	topology, _ := relationLawTopology(t)
	base, baseOK := topology.InitialRelation()
	if !baseOK || !base.Available() {
		t.Fatal("sealed relation unavailable")
	}
	detached := base
	detached.generation = 0
	detached.digest = composition.Key{}
	if !detached.Available() {
		t.Fatal("Available re-derives generation/digest instead of reading the sealed owner")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = base.Available() }); allocs != 0 {
		t.Fatalf("Available allocates %v per call", allocs)
	}
	if (Relation{}).Available() {
		t.Fatal("zero relation available")
	}
}

// TestRelationRefusesMalformedConstruction pins that Publish refuses a foreign
// or unstamped predecessor rather than issuing an unavailable Relation to be
// caught later.
func TestRelationRefusesMalformedConstruction(t *testing.T) {
	topology, accepted := relationLawTopology(t)
	foreign, _ := relationLawTopology(t)
	foreignBase, foreignOK := foreign.InitialRelation()
	if !foreignOK {
		t.Fatal("foreign relation law topology")
	}
	if relation, published := topology.Publish(foreignBase, []AcceptedMember{accepted}); published || relation.Available() {
		t.Fatal("publish accepted a foreign predecessor")
	}
	if relation, published := topology.Publish(Relation{}, []AcceptedMember{accepted}); published || relation.Available() {
		t.Fatal("publish accepted an unstamped predecessor")
	}
}

// TestTemplateBindingAvailabilityIsSealedAtConstruction pins that
// TemplateBinding.Available reads the verdict SealTemplateBinding sealed onto
// its data pointer, not a re-authentication of formals/actuals/key/authority
// on every call: a copy of the underlying data with those fields cleared
// stays available because the sealed bit already carries the verdict.
func TestTemplateBindingAvailabilityIsSealedAtConstruction(t *testing.T) {
	fixture := newActivationRowFixture(t)
	binding := fixture.binding
	if !binding.Available() {
		t.Fatal("sealed binding unavailable")
	}
	detached := binding
	corrupted := *binding.data
	corrupted.formals, corrupted.actuals, corrupted.authority = nil, nil, nil
	corrupted.key, corrupted.rows, corrupted.bySite = composition.Key{}, nil, nil
	detached.data = &corrupted
	if detached.Available() != binding.Available() {
		t.Fatal("Available re-authenticates the underlying data instead of reading the sealed verdict")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = binding.Available() }); allocs != 0 {
		t.Fatalf("Available allocates %v per call", allocs)
	}
	if (TemplateBinding{}).Available() {
		t.Fatal("zero template binding available")
	}
}

// TestTemplateBindingRefusesMalformedConstruction pins that identical
// formal/actual batches and an incomplete assignment set are refused by
// SealTemplateBinding itself.
func TestTemplateBindingRefusesMalformedConstruction(t *testing.T) {
	fixture := newActivationRowFixture(t)
	if binding, ok := SealTemplateBinding(fixture.formals, fixture.formals, nil); ok || binding.Available() {
		t.Fatal("a binding was sealed across one identical batch")
	}
	if binding, ok := SealTemplateBinding(fixture.formals, fixture.actuals, nil); ok || binding.Available() {
		t.Fatal("a binding was sealed with no assignments")
	}
}

// TestMemberAvailabilityIsSealedAtConstruction pins that Member.Available
// reads the verdict SelectActivationMember(ForContext) sealed, not a
// re-check of binding/locator availability: both are already proven available
// before either constructor ever sets this field.
func TestMemberAvailabilityIsSealedAtConstruction(t *testing.T) {
	_, accepted := relationLawTopology(t)
	member := accepted.Member()
	if !member.Available() {
		t.Fatal("sealed member unavailable")
	}
	detached := member
	detached.binding = composition.Key{}
	detached.locator = PairLocator{}
	if !detached.Available() {
		t.Fatal("Available re-derives binding/locator instead of reading the sealed verdict")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = member.Available() }); allocs != 0 {
		t.Fatalf("Available allocates %v per call", allocs)
	}
	if (Member{}).Available() {
		t.Fatal("zero member available")
	}
}

// TestMemberRefusesMalformedConstruction pins that an unavailable
// trigger/locator and an unknown tuple are refused by SelectActivationMember
// itself.
func TestMemberRefusesMalformedConstruction(t *testing.T) {
	topology, _ := relationLawTopology(t)
	if member, ok := topology.SelectActivationMember(composition.Key{}, PairLocator{}); ok || member.Available() {
		t.Fatal("a member was selected for an unavailable trigger/locator")
	}
	unknown := PairLocator{Application: boundaryKey(251), Target: boundaryKey(252), Endpoint: boundaryKey(253)}
	if member, ok := topology.SelectActivationMember(boundaryKey(250), unknown); ok || member.Available() {
		t.Fatal("a member was selected for an unknown tuple")
	}
}

// TestAcceptedMemberAvailabilityIsSealedAtConstruction pins that
// AcceptedMember.Available reads the verdict Accept/MergeAccepted sealed, not
// a re-check of member/premise/evidence availability.
func TestAcceptedMemberAvailabilityIsSealedAtConstruction(t *testing.T) {
	_, accepted := relationLawTopology(t)
	if !accepted.Available() {
		t.Fatal("sealed accepted member unavailable")
	}
	detached := accepted
	detached.member = Member{}
	detached.premise = Expr{}
	detached.evidence = composition.Key{}
	if !detached.Available() {
		t.Fatal("Available re-derives member/premise/evidence instead of reading the sealed verdict")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = accepted.Available() }); allocs != 0 {
		t.Fatalf("Available allocates %v per call", allocs)
	}
	if (AcceptedMember{}).Available() {
		t.Fatal("zero accepted member available")
	}
}

// TestAcceptedMemberRefusesMalformedConstruction pins that an unowned member
// and an unavailable premise are refused by Accept itself.
func TestAcceptedMemberRefusesMalformedConstruction(t *testing.T) {
	topology, accepted := relationLawTopology(t)
	if value, ok := topology.Accept(Member{}, TrueExpr()); ok || value.Available() {
		t.Fatal("accept sealed an unowned member")
	}
	if value, ok := topology.Accept(accepted.Member(), Expr{}); ok || value.Available() {
		t.Fatal("accept sealed an unavailable premise")
	}
}

// TestInputAvailabilityIsSealedAtConstruction pins that Input.Available reads
// the verdict BoundaryInput sealed, not a re-walk of the source/target Sites,
// Reindex, and pre/post formulas on every call.
func TestInputAvailabilityIsSealedAtConstruction(t *testing.T) {
	fixture := newActivationRowFixture(t)
	input := fixture.inputs[0]
	if !input.Available() {
		t.Fatal("sealed input unavailable")
	}
	detached := input
	detached.source, detached.target = Site{}, Site{}
	detached.omega = Reindex{}
	detached.pre, detached.post = Expr{}, Expr{}
	if !detached.Available() {
		t.Fatal("Available re-walks the boundary instead of reading the sealed verdict")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = input.Available() }); allocs != 0 {
		t.Fatalf("Available allocates %v per call", allocs)
	}
	if (Input{}).Available() {
		t.Fatal("zero input available")
	}
}

// TestInputRefusesMalformedConstruction pins that an Input spanning two
// distinct Batches is refused by BoundaryInput itself.
func TestInputRefusesMalformedConstruction(t *testing.T) {
	fixture := newActivationRowFixture(t)
	crossBatch := BoundaryInput(fixture.local, fixture.actualInput, boundaryKey(240), TrueExpr(), IdentityReindex(fixture.local.Scope()), TrueExpr())
	if crossBatch.Available() {
		t.Fatal("an input spanning two distinct batches was sealed")
	}
}

// TestActivationTargetRowsAvailabilityIsSealedAtConstruction pins that
// activationTargetRows.Available reads the verdict lowerActivationTargetRows
// sealed onto its data pointer, not a re-derivation from source/binding/batch
// on every call.
func TestActivationTargetRowsAvailabilityIsSealedAtConstruction(t *testing.T) {
	fixture := newActivationRowFixture(t)
	sites := []Site{fixture.input.Site(), fixture.local, fixture.output.Site()}
	value, ok := lowerActivationTargetRows(fixture.source, fixture.binding, sites, fixture.inputs)
	if !ok || !value.Available() {
		t.Fatal("sealed activation target rows unavailable")
	}
	detached := value
	corrupted := *value.data
	corrupted.source, corrupted.binding, corrupted.batch = nil, nil, nil
	corrupted.key = composition.Key{}
	detached.data = &corrupted
	if detached.Available() != value.Available() {
		t.Fatal("Available re-derives source/binding/batch instead of reading the sealed verdict")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = value.Available() }); allocs != 0 {
		t.Fatalf("Available allocates %v per call", allocs)
	}
	if (activationTargetRows{}).Available() {
		t.Fatal("zero activation target rows available")
	}
}

// TestActivationTargetRowsRefusesMalformedConstruction pins that a missing
// source Composition and an incomplete formal Site denominator are refused by
// lowerActivationTargetRows itself.
func TestActivationTargetRowsRefusesMalformedConstruction(t *testing.T) {
	fixture := newActivationRowFixture(t)
	sites := []Site{fixture.input.Site(), fixture.local, fixture.output.Site()}
	if value, ok := lowerActivationTargetRows(nil, fixture.binding, sites, fixture.inputs); ok || value.Available() {
		t.Fatal("lowering was sealed with no source composition")
	}
	if value, ok := lowerActivationTargetRows(fixture.source, fixture.binding, sites[:1], fixture.inputs); ok || value.Available() {
		t.Fatal("lowering was sealed with an incomplete formal site denominator")
	}
}
