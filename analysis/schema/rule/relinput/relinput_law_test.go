package relinput

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// TestEveryRuleOrdinalTakesOneBundleRow states the totality law: Seal
// produces exactly catalog.Count() rows, indexed by rule ordinal, and every
// present rule's row states Present()==true with a port count equal to the
// rule plan's own declared input width.
func TestEveryRuleOrdinalTakesOneBundleRow(t *testing.T) {
	catalog := relinputPlanCatalog(t)
	owner := relinputOwner(t, "totality")

	candidateSingle := relinputScope(t, owner, "totality/candidate-single")
	portSingle := relinputScope(t, owner, "totality/port-single")
	candidateDouble := relinputScope(t, owner, "totality/candidate-double")
	portDoubleA := relinputScope(t, owner, "totality/port-double-a")
	portDoubleB := relinputScope(t, owner, "totality/port-double-b")

	composition := newRelinputComposition().
		place(0, Placement{Candidate: candidateSingle, Ports: []model.ScopeID{portSingle}}).
		place(1, Placement{Candidate: candidateDouble, Ports: []model.ScopeID{portDoubleA, portDoubleB}}).
		region(candidateSingle, relinputEvidence(t, "totality/candidate-single")).
		region(portSingle, relinputEvidence(t, "totality/port-single")).
		region(candidateDouble, relinputEvidence(t, "totality/candidate-double")).
		region(portDoubleA, relinputEvidence(t, "totality/port-double-a")).
		region(portDoubleB, relinputEvidence(t, "totality/port-double-b"))

	bundle, refusal := Seal(catalog, owner, composition)
	if refusal != nil {
		t.Fatalf("seal refused: %v", refusal)
	}
	if !bundle.Available() {
		t.Fatal("sealed bundle unavailable")
	}
	if bundle.Count() != catalog.Count() {
		t.Fatalf("bundle.Count() = %d, want catalog.Count() = %d", bundle.Count(), catalog.Count())
	}

	for ordinal := 0; ordinal < catalog.Count(); ordinal++ {
		plan, held := catalog.At(ordinal)
		if !held {
			t.Fatalf("catalog missing ordinal %d", ordinal)
		}
		row, rowHeld := bundle.At(ordinal)
		if !rowHeld {
			t.Fatalf("bundle missing ordinal %d", ordinal)
		}
		if row.Present() != plan.Present() {
			t.Fatalf("ordinal %d: row.Present() = %t, plan.Present() = %t", ordinal, row.Present(), plan.Present())
		}
		if row.PortCount() != plan.InputCount() {
			t.Fatalf("ordinal %d: row.PortCount() = %d, plan.InputCount() = %d", ordinal, row.PortCount(), plan.InputCount())
		}
		if _, candidateHeld := bundle.CandidateScope(ordinal); candidateHeld != plan.Present() {
			t.Fatalf("ordinal %d: CandidateScope held = %t, want %t", ordinal, candidateHeld, plan.Present())
		}
	}
}

// relinputRefusalComposition builds a composition that places the single-port
// rule (ordinal 0) with a valid candidate and port, then lets the caller
// mutate exactly one aspect of it before sealing.
func relinputRefusalComposition(t *testing.T, owner model.OwnerID) (*relinputComposition, model.ScopeID, model.ScopeID) {
	t.Helper()
	candidate := relinputScope(t, owner, "refusal/candidate")
	port := relinputScope(t, owner, "refusal/port")
	composition := newRelinputComposition().
		place(0, Placement{Candidate: candidate, Ports: []model.ScopeID{port}}).
		region(candidate, relinputEvidence(t, "refusal/candidate")).
		region(port, relinputEvidence(t, "refusal/port"))
	return composition, candidate, port
}

func TestSealRefusesAMountedRuleTheCompositionCannotPlace(t *testing.T) {
	catalog := relinputPlanCatalog(t)
	owner := relinputOwner(t, "unplaced")
	composition := newRelinputComposition() // ordinal 0 has no placement

	bundle, refusal := Seal(catalog, owner, composition)
	if bundle != nil {
		t.Fatal("seal admitted an unplaceable rule")
	}
	if refusal.Reason() != ReasonUnplaced {
		t.Fatalf("reason = %v, want ReasonUnplaced", refusal.Reason())
	}
	ordinal, ordinalHeld := refusal.Ordinal()
	if !ordinalHeld || ordinal != 0 {
		t.Fatalf("ordinal = %d/%t, want 0/true", ordinal, ordinalHeld)
	}
}

func TestSealRefusesAPlacementWhosePortWidthDisagreesWithThePlan(t *testing.T) {
	catalog := relinputPlanCatalog(t)
	owner := relinputOwner(t, "port-width")
	candidate := relinputScope(t, owner, "port-width/candidate")
	portA := relinputScope(t, owner, "port-width/port-a")
	portB := relinputScope(t, owner, "port-width/port-b")
	// Ordinal 0 declares one input port; this placement states two.
	composition := newRelinputComposition().
		place(0, Placement{Candidate: candidate, Ports: []model.ScopeID{portA, portB}}).
		region(candidate, relinputEvidence(t, "port-width/candidate")).
		region(portA, relinputEvidence(t, "port-width/port-a")).
		region(portB, relinputEvidence(t, "port-width/port-b"))

	bundle, refusal := Seal(catalog, owner, composition)
	if bundle != nil {
		t.Fatal("seal admitted a placement with the wrong port width")
	}
	if refusal.Reason() != ReasonPortWidth {
		t.Fatalf("reason = %v, want ReasonPortWidth", refusal.Reason())
	}
	ordinal, ordinalHeld := refusal.Ordinal()
	if !ordinalHeld || ordinal != 0 {
		t.Fatalf("ordinal = %d/%t, want 0/true", ordinal, ordinalHeld)
	}
}

func TestSealRefusesAPlacementNamingAnUnavailableScope(t *testing.T) {
	catalog := relinputPlanCatalog(t)
	owner := relinputOwner(t, "unavailable-scope")
	composition, _, port := relinputRefusalComposition(t, owner)
	composition.place(0, Placement{Candidate: model.ScopeID{}, Ports: []model.ScopeID{port}})

	bundle, refusal := Seal(catalog, owner, composition)
	if bundle != nil {
		t.Fatal("seal admitted a placement naming an unavailable scope")
	}
	if refusal.Reason() != ReasonScope {
		t.Fatalf("reason = %v, want ReasonScope", refusal.Reason())
	}
	ordinal, ordinalHeld := refusal.Ordinal()
	if !ordinalHeld || ordinal != 0 {
		t.Fatalf("ordinal = %d/%t, want 0/true", ordinal, ordinalHeld)
	}
}

func TestSealRefusesAPlacementNamingAForeignOwnersScope(t *testing.T) {
	catalog := relinputPlanCatalog(t)
	owner := relinputOwner(t, "foreign-owner")
	foreignOwner := relinputOwner(t, "foreign-owner/other")
	foreignCandidate := relinputScope(t, foreignOwner, "foreign-owner/candidate")
	port := relinputScope(t, owner, "foreign-owner/port")
	composition := newRelinputComposition().
		place(0, Placement{Candidate: foreignCandidate, Ports: []model.ScopeID{port}}).
		region(foreignCandidate, relinputEvidence(t, "foreign-owner/candidate")).
		region(port, relinputEvidence(t, "foreign-owner/port"))

	bundle, refusal := Seal(catalog, owner, composition)
	if bundle != nil {
		t.Fatal("seal admitted a placement naming a foreign owner's scope")
	}
	if refusal.Reason() != ReasonForeignOwner {
		t.Fatalf("reason = %v, want ReasonForeignOwner", refusal.Reason())
	}
	ordinal, ordinalHeld := refusal.Ordinal()
	if !ordinalHeld || ordinal != 0 {
		t.Fatalf("ordinal = %d/%t, want 0/true", ordinal, ordinalHeld)
	}
}

func TestSealRefusesAScopeTheCompositionSuppliesNoRegionEvidenceFor(t *testing.T) {
	catalog := relinputPlanCatalog(t)
	owner := relinputOwner(t, "no-region")
	candidate := relinputScope(t, owner, "no-region/candidate")
	port := relinputScope(t, owner, "no-region/port")
	// No region() call for candidate: the composition names the scope in a
	// placement but supplies no evidence for it.
	composition := newRelinputComposition().
		place(0, Placement{Candidate: candidate, Ports: []model.ScopeID{port}}).
		region(port, relinputEvidence(t, "no-region/port"))

	bundle, refusal := Seal(catalog, owner, composition)
	if bundle != nil {
		t.Fatal("seal admitted a scope with no region evidence")
	}
	if refusal.Reason() != ReasonRegion {
		t.Fatalf("reason = %v, want ReasonRegion", refusal.Reason())
	}
	ordinal, ordinalHeld := refusal.Ordinal()
	if !ordinalHeld || ordinal != 0 {
		t.Fatalf("ordinal = %d/%t, want 0/true", ordinal, ordinalHeld)
	}
}

func TestSealRefusesANilComposition(t *testing.T) {
	catalog := relinputPlanCatalog(t)
	owner := relinputOwner(t, "nil-composition")

	bundle, refusal := Seal(catalog, owner, nil)
	if bundle != nil {
		t.Fatal("seal admitted a nil composition")
	}
	if refusal.Reason() != ReasonComposition {
		t.Fatalf("reason = %v, want ReasonComposition", refusal.Reason())
	}
	if _, ordinalHeld := refusal.Ordinal(); ordinalHeld {
		t.Fatal("a composition refusal names an ordinal")
	}
}

func TestSealRefusesAZeroOwner(t *testing.T) {
	catalog := relinputPlanCatalog(t)
	// The composition is never consulted: a zero owner refuses before Seal
	// reads a single placement or region.
	composition, _, _ := relinputRefusalComposition(t, relinputOwner(t, "zero-owner/unused"))

	bundle, refusal := Seal(catalog, model.OwnerID{}, composition)
	if bundle != nil {
		t.Fatal("seal admitted a zero owner")
	}
	if refusal.Reason() != ReasonOwner {
		t.Fatalf("reason = %v, want ReasonOwner", refusal.Reason())
	}
	if _, ordinalHeld := refusal.Ordinal(); ordinalHeld {
		t.Fatal("an owner refusal names an ordinal")
	}
}

// TestPortScopeAnswersPortsInCompositionOrder states the port order law: for
// a rule with two ports placed at two distinct scopes, PortScope(ordinal, i)
// answers exactly the i-th scope the composition supplied, and an
// out-of-range port index answers false.
func TestPortScopeAnswersPortsInCompositionOrder(t *testing.T) {
	catalog := relinputPlanCatalog(t)
	owner := relinputOwner(t, "port-order")

	candidateSingle := relinputScope(t, owner, "port-order/candidate-single")
	portSingle := relinputScope(t, owner, "port-order/port-single")
	candidateDouble := relinputScope(t, owner, "port-order/candidate-double")
	portFirst := relinputScope(t, owner, "port-order/port-first")
	portSecond := relinputScope(t, owner, "port-order/port-second")

	composition := newRelinputComposition().
		place(0, Placement{Candidate: candidateSingle, Ports: []model.ScopeID{portSingle}}).
		place(1, Placement{Candidate: candidateDouble, Ports: []model.ScopeID{portFirst, portSecond}}).
		region(candidateSingle, relinputEvidence(t, "port-order/candidate-single")).
		region(portSingle, relinputEvidence(t, "port-order/port-single")).
		region(candidateDouble, relinputEvidence(t, "port-order/candidate-double")).
		region(portFirst, relinputEvidence(t, "port-order/port-first")).
		region(portSecond, relinputEvidence(t, "port-order/port-second"))

	bundle, refusal := Seal(catalog, owner, composition)
	if refusal != nil {
		t.Fatalf("seal refused: %v", refusal)
	}

	if count, held := bundle.PortCount(1); !held || count != 2 {
		t.Fatalf("PortCount(1) = %d/%t, want 2/true", count, held)
	}
	if scope, held := bundle.PortScope(1, 0); !held || scope != portFirst {
		t.Fatalf("PortScope(1,0) = %v/%t, want %v/true", scope, held, portFirst)
	}
	if scope, held := bundle.PortScope(1, 1); !held || scope != portSecond {
		t.Fatalf("PortScope(1,1) = %v/%t, want %v/true", scope, held, portSecond)
	}
	if _, held := bundle.PortScope(1, 2); held {
		t.Fatal("PortScope answered a port index past the rule's declared width")
	}
	if _, held := bundle.PortScope(1, -1); held {
		t.Fatal("PortScope answered a negative port index")
	}
}

// TestAFrozenViewAnswersExactlyWhatTheBundleSealed states the freeze
// round-trip law: every accessor a View exposes answers identically to the
// Bundle it was frozen from, for every ordinal, every port, and every
// region; and Open refuses a wrong catalog digest or a wrong owner.
func TestAFrozenViewAnswersExactlyWhatTheBundleSealed(t *testing.T) {
	catalog := relinputPlanCatalog(t)
	owner := relinputOwner(t, "freeze")

	candidateSingle := relinputScope(t, owner, "freeze/candidate-single")
	portSingle := relinputScope(t, owner, "freeze/port-single")
	candidateDouble := relinputScope(t, owner, "freeze/candidate-double")
	portFirst := relinputScope(t, owner, "freeze/port-first")
	portSecond := relinputScope(t, owner, "freeze/port-second")

	composition := newRelinputComposition().
		place(0, Placement{Candidate: candidateSingle, Ports: []model.ScopeID{portSingle}}).
		place(1, Placement{Candidate: candidateDouble, Ports: []model.ScopeID{portFirst, portSecond}}).
		region(candidateSingle, relinputEvidence(t, "freeze/candidate-single")).
		region(portSingle, relinputEvidence(t, "freeze/port-single")).
		region(candidateDouble, relinputEvidence(t, "freeze/candidate-double")).
		region(portFirst, relinputEvidence(t, "freeze/port-first")).
		region(portSecond, relinputEvidence(t, "freeze/port-second"))

	bundle, refusal := Seal(catalog, owner, composition)
	if refusal != nil {
		t.Fatalf("seal refused: %v", refusal)
	}

	store, storeIssued := identity.IssueStore()
	if !storeIssued {
		t.Fatal("store not issued")
	}
	frozen, frozeOK := bundle.Freeze(store)
	if !frozeOK {
		t.Fatal("bundle did not freeze")
	}
	view, opened := Open(&frozen, bundle.Catalog(), bundle.Owner())
	if !opened {
		t.Fatal("frozen publication did not open")
	}

	if view.Count() != bundle.Count() {
		t.Fatalf("view.Count() = %d, want bundle.Count() = %d", view.Count(), bundle.Count())
	}
	for ordinal := 0; ordinal < bundle.Count(); ordinal++ {
		bundleRow, bundleHeld := bundle.At(ordinal)
		viewRow, viewHeld := view.At(ordinal)
		if bundleHeld != viewHeld || bundleRow != viewRow {
			t.Fatalf("ordinal %d: bundle.At = %+v/%t, view.At = %+v/%t", ordinal, bundleRow, bundleHeld, viewRow, viewHeld)
		}

		bundleCandidate, bundleCandidateHeld := bundle.CandidateScope(ordinal)
		viewCandidate, viewCandidateHeld := view.CandidateScope(ordinal)
		if bundleCandidateHeld != viewCandidateHeld || bundleCandidate != viewCandidate {
			t.Fatalf("ordinal %d: bundle.CandidateScope = %v/%t, view.CandidateScope = %v/%t", ordinal, bundleCandidate, bundleCandidateHeld, viewCandidate, viewCandidateHeld)
		}

		bundlePortCount, bundlePortCountHeld := bundle.PortCount(ordinal)
		viewPortCount, viewPortCountHeld := view.PortCount(ordinal)
		if bundlePortCountHeld != viewPortCountHeld || bundlePortCount != viewPortCount {
			t.Fatalf("ordinal %d: bundle.PortCount = %d/%t, view.PortCount = %d/%t", ordinal, bundlePortCount, bundlePortCountHeld, viewPortCount, viewPortCountHeld)
		}

		for port := 0; port < bundlePortCount+1; port++ {
			bundleScope, bundleScopeHeld := bundle.PortScope(ordinal, port)
			viewScope, viewScopeHeld := view.PortScope(ordinal, port)
			if bundleScopeHeld != viewScopeHeld || bundleScope != viewScope {
				t.Fatalf("ordinal %d port %d: bundle.PortScope = %v/%t, view.PortScope = %v/%t", ordinal, port, bundleScope, bundleScopeHeld, viewScope, viewScopeHeld)
			}
		}
	}

	if view.RegionCount() != bundle.RegionCount() {
		t.Fatalf("view.RegionCount() = %d, want bundle.RegionCount() = %d", view.RegionCount(), bundle.RegionCount())
	}
	for index := 0; index < bundle.RegionCount(); index++ {
		bundleRegion, bundleRegionHeld := bundle.RegionAt(index)
		viewRegion, viewRegionHeld := view.RegionAt(index)
		if bundleRegionHeld != viewRegionHeld || bundleRegion != viewRegion {
			t.Fatalf("region %d: bundle.RegionAt = %+v/%t, view.RegionAt = %+v/%t", index, bundleRegion, bundleRegionHeld, viewRegion, viewRegionHeld)
		}
		bundleEvidence, bundleEvidenceHeld := bundle.ScopeRegion(bundleRegion.Scope())
		viewEvidence, viewEvidenceHeld := view.ScopeRegion(bundleRegion.Scope())
		if bundleEvidenceHeld != viewEvidenceHeld || bundleEvidence != viewEvidence {
			t.Fatalf("scope %v: bundle.ScopeRegion = %v/%t, view.ScopeRegion = %v/%t", bundleRegion.Scope(), bundleEvidence, bundleEvidenceHeld, viewEvidence, viewEvidenceHeld)
		}
	}

	wrongCatalog, derived := identity.DeriveContentID("relinput-law/freeze/wrong-catalog", nil)
	if !derived {
		t.Fatal("wrong catalog digest undeliverable")
	}
	if _, opened := Open(&frozen, wrongCatalog, bundle.Owner()); opened {
		t.Fatal("Open admitted a wrong catalog digest")
	}
	wrongOwner := relinputOwner(t, "freeze/wrong-owner")
	if _, opened := Open(&frozen, bundle.Catalog(), wrongOwner); opened {
		t.Fatal("Open admitted a wrong owner")
	}
}

// TestTwoRulesAtTheSameScopeShareOneRegionRow states the region
// de-duplication law: when two rules are placed at the same candidate scope,
// the bundle carries exactly one region row for it, and the region column
// otherwise holds one row per remaining distinct scope in first-named order.
func TestTwoRulesAtTheSameScopeShareOneRegionRow(t *testing.T) {
	catalog := relinputPlanCatalog(t)
	owner := relinputOwner(t, "shared-scope")

	shared := relinputScope(t, owner, "shared-scope/candidate")
	portSingle := relinputScope(t, owner, "shared-scope/port-single")
	portFirst := relinputScope(t, owner, "shared-scope/port-first")
	portSecond := relinputScope(t, owner, "shared-scope/port-second")

	composition := newRelinputComposition().
		place(0, Placement{Candidate: shared, Ports: []model.ScopeID{portSingle}}).
		place(1, Placement{Candidate: shared, Ports: []model.ScopeID{portFirst, portSecond}}).
		region(shared, relinputEvidence(t, "shared-scope/candidate")).
		region(portSingle, relinputEvidence(t, "shared-scope/port-single")).
		region(portFirst, relinputEvidence(t, "shared-scope/port-first")).
		region(portSecond, relinputEvidence(t, "shared-scope/port-second"))

	bundle, refusal := Seal(catalog, owner, composition)
	if refusal != nil {
		t.Fatalf("seal refused: %v", refusal)
	}

	const wantRegions = 4 // shared candidate + port-single + port-first + port-second
	if bundle.RegionCount() != wantRegions {
		t.Fatalf("RegionCount() = %d, want %d", bundle.RegionCount(), wantRegions)
	}

	sharedRows := 0
	wantOrder := []model.ScopeID{shared, portSingle, portFirst, portSecond}
	for index := 0; index < bundle.RegionCount(); index++ {
		row, held := bundle.RegionAt(index)
		if !held {
			t.Fatalf("RegionAt(%d) not held", index)
		}
		if row.Scope() != wantOrder[index] {
			t.Fatalf("RegionAt(%d).Scope() = %v, want %v (first-named order)", index, row.Scope(), wantOrder[index])
		}
		if row.Scope() == shared {
			sharedRows++
		}
	}
	if sharedRows != 1 {
		t.Fatalf("shared candidate scope has %d region rows, want exactly 1", sharedRows)
	}
}
