package guard

import "testing"

func TestScopeRanksFenceUnownedAndForeignAtoms(t *testing.T) {
	manager := newTestManager(t)
	onlyA, ok := manager.SealScope([]Atom{testA})
	if !ok || !onlyA.Valid() {
		t.Fatal("single-rank scope")
	}
	if onlyA.contains(testB) || onlyA.contains(testC) {
		t.Fatal("scope admitted an unowned presealed atom")
	}
	rootA := sealedGuard(t, manager, func(work *Work) Guard { return literal(t, work, testA) })
	rootB := sealedGuard(t, manager, func(work *Work) Guard { return literal(t, work, testB) })
	if !onlyA.Contains(rootA) {
		t.Fatal("scope rejected its owned atom")
	}
	if onlyA.Contains(rootB) {
		t.Fatal("scope admitted an unowned root")
	}
	foreign := newTestManager(t)
	foreignRoot := sealedGuard(t, foreign, func(work *Work) Guard { return literal(t, work, testA) })
	if onlyA.Contains(foreignRoot) {
		t.Fatal("scope crossed its Manager fence")
	}
}

func TestCompactReindexRowsRemainTotalAndFenceSource(t *testing.T) {
	manager := newTestManager(t)
	source, ok := manager.SealScope([]Atom{testA, testC})
	if !ok {
		t.Fatal("source scope")
	}
	target := manager.AllScope()
	builder, ok := manager.NewReindex(source, target)
	if !ok || !builder.Identity(testA) || !builder.Forget(testC) {
		t.Fatal("source-row reindex construction")
	}
	plan, ok := builder.Seal()
	if !ok || !plan.Valid() {
		t.Fatal("source-row reindex seal")
	}
	for _, atom := range []Atom{testA, testC} {
		if _, ok := plan.Action(atom); !ok {
			t.Fatalf("missing total source action for atom %d", atom)
		}
	}
	if _, ok := plan.Action(testB); ok {
		t.Fatal("reindex exposed an action outside its source scope")
	}
	foreign := newTestManager(t)
	foreignBuilder, ok := foreign.NewReindex(foreign.AllScope(), foreign.AllScope())
	if !ok || !foreignBuilder.Identity(testA) || !foreignBuilder.Identity(testB) || !foreignBuilder.Identity(testC) {
		t.Fatal("foreign reindex construction")
	}
	foreignPlan, ok := foreignBuilder.Seal()
	if !ok || !foreignPlan.Valid() {
		t.Fatal("foreign reindex seal")
	}
	if _, ok := manager.NewReindex(foreignPlan.Source(), foreignPlan.Target()); ok {
		t.Fatal("Manager accepted foreign reindex scopes")
	}
	if _, ok := manager.ComposeReindex(plan, foreignPlan); ok {
		t.Fatal("Manager composed a foreign reindex")
	}
}

func TestCoordinateIdentityDistinguishesPayloadFromIssuedScope(t *testing.T) {
	manager := newTestManager(t)
	source := sealTestScope(t, manager, []Atom{testA, testC})
	target := sealTestScope(t, manager, []Atom{testA, testC})
	if source.Same(target) {
		t.Fatal("separately issued scopes aliased")
	}
	builder, ok := manager.NewReindex(source, target)
	if !ok || !builder.Identity(testA) || !builder.Identity(testC) {
		t.Fatal("coordinate identity construction")
	}
	plan, ok := builder.Seal()
	if !ok || !plan.Valid() {
		t.Fatal("coordinate identity seal")
	}
	if plan.Identity() {
		t.Fatal("coordinate identity claimed exact issued-scope identity")
	}
	if !plan.CoordinateIdentity() {
		t.Fatal("identity coordinate function was not recognized")
	}
}

func TestScopeAndReindexHotReadsDoNotAllocate(t *testing.T) {
	manager := newTestManager(t)
	scope := sealTestScope(t, manager, []Atom{testA})
	root := sealedGuard(t, manager, func(work *Work) Guard { return literal(t, work, testA) })
	builder, ok := manager.NewReindex(scope, manager.AllScope())
	if !ok || !builder.Identity(testA) {
		t.Fatal("identity reindex construction")
	}
	plan, ok := builder.Seal()
	if !ok {
		t.Fatal("identity reindex seal")
	}
	if !scope.Contains(root) || !plan.Valid() {
		t.Fatal("warm hot-read paths")
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		if !scope.Valid() || !scope.contains(testA) || scope.contains(testB) {
			t.Fatal("scope scalar hot read changed")
		}
		if _, ok := plan.Action(testA); !ok {
			t.Fatal("reindex action hot read changed")
		}
	}); allocations != 0 {
		t.Fatalf("scope/reindex hot reads allocated %f times", allocations)
	}
}

func TestSealedScopeAndReindexValidationIsCardinalityIndependentAndAllocationFree(t *testing.T) {
	const count = 4_096
	atoms := make([]Atom, count)
	for index := range atoms {
		atoms[index] = Atom(index + 1)
	}
	manager, err := New(atoms)
	if err != nil {
		t.Fatal(err)
	}
	scope, scopeOK := manager.SealScope(atoms)
	plan, planOK := manager.IdentityReindex(scope)
	if !scopeOK || !planOK || !scope.Valid() || !plan.Valid() {
		t.Fatal("large sealed scope/reindex")
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		if !scope.Valid() || !plan.Valid() {
			t.Fatal("sealed capability validation changed")
		}
	}); allocations != 0 {
		t.Fatalf("sealed scope/reindex validation allocated %f times", allocations)
	}
}

// sealTestScope seals one scope for a fixture and fails the test rather than
// widening the production Manager with a panicking helper.
func sealTestScope(t *testing.T, manager *Manager, atoms []Atom) Scope {
	t.Helper()
	scope, ok := manager.SealScope(atoms)
	if !ok {
		t.Fatal("test scope seal")
	}
	return scope
}

// TestScopeValidIsSealedAtConstruction pins that Scope.Valid reads the
// completeness verdict SealScope/AllScope reached once, rather than
// re-deriving it from manager/ranks on every call.  Detaching those fields
// from an already-issued scope therefore cannot flip the verdict, and the
// read allocates nothing.
func TestScopeValidIsSealedAtConstruction(t *testing.T) {
	manager := newTestManager(t)
	issued := sealTestScope(t, manager, []Atom{testA})
	if !issued.Valid() {
		t.Fatal("issued scope unavailable")
	}
	detached := *issued.value
	detached.manager = nil
	detached.ranks = nil
	view := Scope{value: &detached}
	if !view.Valid() {
		t.Fatal("Valid re-derives from manager/ranks instead of reading the sealed verdict")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = issued.Valid() }); allocs != 0 {
		t.Fatalf("Valid allocates %v per call", allocs)
	}
	if (Scope{}).Valid() {
		t.Fatal("zero scope available")
	}
}

// TestScopeRefusesMalformedConstruction pins that SealScope is the sole
// authenticator: an unowned or unsorted atom set never reaches a published
// Scope, so a caller can never observe a false-verdict scope becoming true
// verdict later.
func TestScopeRefusesMalformedConstruction(t *testing.T) {
	manager := newTestManager(t)
	if scope, ok := manager.SealScope([]Atom{testC, testA}); ok || scope.Valid() {
		t.Fatal("unsorted atom set sealed")
	}
	if scope, ok := manager.SealScope([]Atom{9999}); ok || scope.Valid() {
		t.Fatal("unowned atom sealed")
	}
	var nilManager *Manager
	if scope, ok := nilManager.SealScope([]Atom{testA}); ok || scope.Valid() {
		t.Fatal("nil-manager scope sealed")
	}
}

// TestReindexValidIsSealedAtConstruction pins that Reindex.Valid reads the
// completeness verdict Seal/ComposeReindex reached once over the relation's
// manager, scopes, and entries, rather than re-deriving it. Detaching those
// fields from an already-sealed plan therefore cannot flip the verdict, and
// the read allocates nothing.
func TestReindexValidIsSealedAtConstruction(t *testing.T) {
	manager := newTestManager(t)
	scope := sealTestScope(t, manager, []Atom{testA})
	builder, ok := manager.NewReindex(scope, manager.AllScope())
	if !ok || !builder.Identity(testA) {
		t.Fatal("identity reindex construction")
	}
	plan, ok := builder.Seal()
	if !ok || !plan.Valid() {
		t.Fatal("issued reindex plan unavailable")
	}
	detached := *plan.value
	detached.manager = nil
	detached.entries = nil
	view := Reindex{value: &detached}
	if !view.Valid() {
		t.Fatal("Valid re-derives from manager/entries instead of reading the sealed verdict")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = plan.Valid() }); allocs != 0 {
		t.Fatalf("Valid allocates %v per call", allocs)
	}
	if (Reindex{}).Valid() {
		t.Fatal("zero reindex plan available")
	}
}

// TestReindexRefusesMalformedConstruction pins that Seal is the sole
// authenticator: an incomplete source-coordinate assignment never reaches a
// published Reindex.
func TestReindexRefusesMalformedConstruction(t *testing.T) {
	manager := newTestManager(t)
	scope := sealTestScope(t, manager, []Atom{testA, testC})
	builder, ok := manager.NewReindex(scope, manager.AllScope())
	if !ok || !builder.Identity(testA) {
		t.Fatal("partial reindex construction")
	}
	// testC is left unassigned: Seal must reject an incomplete relation.
	if plan, ok := builder.Seal(); ok || plan.Valid() {
		t.Fatal("incomplete source relation sealed")
	}
}
