package support

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func TestMaskNormalizesAndPublishesExactDefinedness(t *testing.T) {
	manager, err := guard.New([]guard.Atom{3, 7, 11})
	if err != nil {
		t.Fatal(err)
	}
	work := New(manager)
	if work == nil {
		t.Fatal("mask work creation failed")
	}

	mask, ok := work.Conjoin(work.True(), 11, true)
	if !ok {
		t.Fatal("first constraint failed")
	}
	mask, ok = work.Conjoin(mask, 3, false)
	if !ok {
		t.Fatal("second constraint failed")
	}
	same, ok := work.Conjoin(mask, 3, false)
	if !ok {
		t.Fatal("idempotent constraint failed")
	}
	// Build the same region in the opposite construction order.  A mask is a
	// BDD, not a textual conjunction; both must become one exact region when
	// published.
	reordered, ok := work.Conjoin(work.True(), 3, false)
	if !ok {
		t.Fatal("reordered first constraint failed")
	}
	reordered, ok = work.Conjoin(reordered, 11, true)
	if !ok {
		t.Fatal("reordered second constraint failed")
	}
	if !work.Seal() || !mask.Valid() || !same.Valid() || !reordered.Valid() {
		t.Fatal("mask publication failed")
	}
	if mask.Manager() != manager || !mask.Equal(same) || !mask.Equal(reordered) {
		t.Fatal("equivalent definedness regions did not canonicalize")
	}

	truth := func(three, eleven bool) func(guard.Atom) bool {
		return func(atom guard.Atom) bool {
			switch atom {
			case 3:
				return three
			case 11:
				return eleven
			default:
				return false
			}
		}
	}
	if !mask.Matches(truth(false, true)) {
		t.Fatal("defined fact did not match its exact support")
	}
	if mask.Matches(truth(true, true)) || mask.Matches(truth(false, false)) {
		t.Fatal("fact support widened beyond its exact branch")
	}
	view, ok := mask.Decompose()
	if !ok || view.Terminal || view.Atom != 3 {
		t.Fatal("published support did not retain fixed guard order")
	}
	work.Discard()
	if !mask.Valid() {
		t.Fatal("published support depended on candidate-work lifetime")
	}
}

func TestFromGuardPreservesSealedRepresentationAndRejectsForeignManager(t *testing.T) {
	manager, err := guard.New([]guard.Atom{3})
	if err != nil {
		t.Fatal(err)
	}
	region, ok := FromGuard(manager, manager.True())
	if !ok || !region.Valid() || !region.Equal(mustTrue(t, manager)) {
		t.Fatal("sealed guard did not retain its exact support representation")
	}
	foreign, err := guard.New([]guard.Atom{3})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := FromGuard(foreign, manager.True()); ok {
		t.Fatal("foreign manager rewrapped a guard")
	}
	handle, ok := region.Guard()
	if !ok {
		t.Fatal("support did not expose sealed guard")
	}
	roundTrip, ok := FromGuard(manager, handle)
	if !ok || !roundTrip.Equal(region) {
		t.Fatal("guard/support round-trip changed representation")
	}
	if _, ok := (Mask{}).Guard(); ok {
		t.Fatal("invalid mask exposed a guard")
	}
}

func TestMaskSameHandleIsPhysicalNotSemanticEquality(t *testing.T) {
	manager, err := guard.New([]guard.Atom{3})
	if err != nil {
		t.Fatal(err)
	}
	firstWork := New(manager)
	first, ok := firstWork.Literal(3, true)
	if !ok || !firstWork.Seal() {
		t.Fatal("first mask setup failed")
	}
	secondWork := New(manager)
	second, ok := secondWork.Literal(3, true)
	if !ok || !secondWork.Seal() {
		t.Fatal("second mask setup failed")
	}
	if !first.SameHandle(first) {
		t.Fatal("same mask lost physical identity")
	}
	if !first.Equal(second) {
		t.Fatal("cross-generation masks lost semantic equality")
	}
}

func TestCoordinateIdentityReindexRetainsExactMaskHandleAcrossIssuedScopes(t *testing.T) {
	manager, err := guard.New([]guard.Atom{3, 7})
	if err != nil {
		t.Fatal(err)
	}
	source, ok := manager.SealScope([]guard.Atom{3, 7})
	if !ok {
		t.Fatal("source scope")
	}
	target, ok := manager.SealScope([]guard.Atom{3, 7})
	if !ok || source.Same(target) {
		t.Fatal("distinct target scope")
	}
	builder, ok := manager.NewReindex(source, target)
	if !ok || !builder.Identity(3) || !builder.Identity(7) {
		t.Fatal("coordinate identity construction")
	}
	plan, ok := builder.Seal()
	if !ok || plan.Identity() || !plan.CoordinateIdentity() {
		t.Fatal("coordinate identity proof")
	}
	work := New(manager)
	mask, ok := work.Literal(3, true)
	if !ok || !work.Seal() {
		t.Fatal("mask setup")
	}
	reindexed, ok := ReindexWithWork(New(manager), mask, plan)
	if !ok || !reindexed.SameHandle(mask) {
		t.Fatal("coordinate identity rebuilt an unchanged Boolean support")
	}
}

func TestReindexRejectsOutOfScopeNonIdentityAndDiscardsCandidate(t *testing.T) {
	manager, err := guard.New([]guard.Atom{3, 7})
	if err != nil {
		t.Fatal(err)
	}
	source, ok := manager.SealScope([]guard.Atom{3})
	if !ok {
		t.Fatal("source scope")
	}
	target, ok := manager.SealScope(nil)
	if !ok {
		t.Fatal("target scope")
	}
	builder, ok := manager.NewReindex(source, target)
	if !ok || !builder.Forget(3) {
		t.Fatal("forget relation")
	}
	plan, ok := builder.Seal()
	if !ok || plan.CoordinateIdentity() {
		t.Fatal("nonidentity relation")
	}
	inputWork := New(manager)
	input, ok := inputWork.Literal(7, true)
	if !ok || !inputWork.Seal() {
		t.Fatal("out-of-scope input")
	}
	transportWork := New(manager)
	if _, ok := ReindexWithWork(transportWork, input, plan); ok {
		t.Fatal("out-of-scope nonidentity transport succeeded")
	}
	if transportWork.Open() || transportWork.Seal() {
		t.Fatal("failed transport published or retained its candidate")
	}
}

func TestReusableSupportBatchReindexPublishesCorrelatedMasksOnce(t *testing.T) {
	manager, err := guard.New([]guard.Atom{3})
	if err != nil {
		t.Fatal(err)
	}
	source, ok := manager.SealScope([]guard.Atom{3})
	if !ok {
		t.Fatal("source scope")
	}
	target, ok := manager.SealScope(nil)
	if !ok {
		t.Fatal("target scope")
	}
	builder, ok := manager.NewReindex(source, target)
	if !ok || !builder.Forget(3) {
		t.Fatal("forget relation")
	}
	plan, ok := builder.Seal()
	if !ok {
		t.Fatal("reindex plan")
	}
	inputBuilder := New(manager)
	input, ok := inputBuilder.Literal(3, true)
	if !ok || !inputBuilder.Seal() {
		t.Fatal("input support")
	}
	batch := New(manager)
	if batch == nil || !batch.BeginTransaction(nil) {
		t.Fatal("batch transaction")
	}
	first, ok := batch.And(input, input)
	if !ok {
		batch.Discard()
		t.Fatal("first candidate")
	}
	second, ok := batch.Reindex(first, plan)
	if !ok {
		batch.Discard()
		t.Fatal("reindex candidate")
	}
	if !batch.Seal() || batch.Open() || !second.Valid() {
		t.Fatal("batch publication")
	}
	whole, ok := True(manager)
	if !ok || !second.Equal(whole) {
		t.Fatal("forget candidate changed exact support")
	}
}

// TestCandidateReindexUsesOwningWorkRatherThanSealedMaskAccessors proves a
// genuinely new batch.And candidate can cross a non-coordinate relation
// before the one enclosing transaction seals. Foreign candidate ownership
// remains rejected rather than being mistaken for an equivalent manager root.
func TestCandidateReindexUsesOwningWorkRatherThanSealedMaskAccessors(t *testing.T) {
	manager, err := guard.New([]guard.Atom{3, 7})
	if err != nil {
		t.Fatal(err)
	}
	source := manager.AllScope()
	target, ok := manager.SealScope([]guard.Atom{3})
	if !ok {
		t.Fatal("target scope")
	}
	builder, ok := manager.NewReindex(source, target)
	if !ok || !builder.Identity(3) || !builder.Forget(7) {
		t.Fatal("non-coordinate relation")
	}
	plan, ok := builder.Seal()
	if !ok {
		t.Fatal("relation seal")
	}
	batch := New(manager)
	if batch == nil || !batch.BeginTransaction(nil) {
		t.Fatal("candidate transaction")
	}
	defer batch.Discard()
	left, ok := batch.Literal(3, true)
	if !ok {
		t.Fatal("candidate left")
	}
	right, ok := batch.Literal(7, true)
	if !ok {
		t.Fatal("candidate right")
	}
	candidate, ok := batch.And(left, right)
	if !ok || candidate.Valid() {
		t.Fatal("genuinely new candidate")
	}
	reindexed, ok := batch.Reindex(candidate, plan)
	if !ok || reindexed.Valid() {
		t.Fatal("candidate non-coordinate reindex")
	}
	foreign := New(manager)
	if foreign == nil || !foreign.BeginTransaction(nil) {
		t.Fatal("foreign transaction")
	}
	defer foreign.Discard()
	if _, ok := foreign.Reindex(candidate, plan); ok {
		t.Fatal("foreign candidate work accepted")
	}
	if !batch.Seal() || !reindexed.Valid() {
		t.Fatal("candidate publication")
	}
}

func TestReindexIdentityAndCoordinateIdentityRetainSourceScopeProof(t *testing.T) {
	manager, err := guard.New([]guard.Atom{3, 7})
	if err != nil {
		t.Fatal(err)
	}
	source, ok := manager.SealScope([]guard.Atom{3})
	if !ok {
		t.Fatal("source scope")
	}
	identityBuilder, ok := manager.NewReindex(source, source)
	if !ok || !identityBuilder.Identity(3) {
		t.Fatal("identity relation")
	}
	identity, ok := identityBuilder.Seal()
	if !ok || !identity.Identity() {
		t.Fatal("identity proof")
	}
	target, ok := manager.SealScope([]guard.Atom{3})
	if !ok || target.Same(source) {
		t.Fatal("coordinate target")
	}
	coordinateBuilder, ok := manager.NewReindex(source, target)
	if !ok || !coordinateBuilder.Identity(3) {
		t.Fatal("coordinate relation")
	}
	coordinate, ok := coordinateBuilder.Seal()
	if !ok || coordinate.Identity() || !coordinate.CoordinateIdentity() {
		t.Fatal("coordinate identity proof")
	}
	inputWork := New(manager)
	input, ok := inputWork.Literal(7, true)
	if !ok || !inputWork.Seal() {
		t.Fatal("out-of-scope input")
	}
	for name, plan := range map[string]guard.Reindex{"identity": identity, "coordinate": coordinate} {
		t.Run(name, func(t *testing.T) {
			if _, ok := ReindexWithWork(nil, input, plan); ok {
				t.Fatal("zero-copy transport admitted an out-of-scope root")
			}
		})
	}
}

func TestReindexNonIdentityPreservesExactSupportSemantics(t *testing.T) {
	manager, err := guard.New([]guard.Atom{3, 7})
	if err != nil {
		t.Fatal(err)
	}
	source := manager.AllScope()
	target, ok := manager.SealScope([]guard.Atom{3})
	if !ok {
		t.Fatal("target scope")
	}
	builder, ok := manager.NewReindex(source, target)
	if !ok || !builder.Identity(3) || !builder.Forget(7) {
		t.Fatal("forget relation")
	}
	plan, ok := builder.Seal()
	if !ok {
		t.Fatal("relation seal")
	}
	inputWork := New(manager)
	input, ok := inputWork.Literal(3, true)
	if !ok {
		t.Fatal("first input literal")
	}
	input, ok = inputWork.And(input, mustSupportLiteral(t, inputWork, 7, true))
	if !ok || !inputWork.Seal() {
		t.Fatal("input support")
	}
	result, ok := Reindex(input, plan)
	if !ok {
		t.Fatal("nonidentity transport")
	}
	expectedWork := New(manager)
	expected, ok := expectedWork.Literal(3, true)
	if !ok || !expectedWork.Seal() {
		t.Fatal("expected support")
	}
	if !result.Equal(expected) {
		t.Fatal("nonidentity transport changed support semantics")
	}
}

func mustSupportLiteral(t testing.TB, work *Work, atom guard.Atom, value bool) Mask {
	t.Helper()
	mask, ok := work.Literal(atom, value)
	if !ok {
		t.Fatalf("literal %d", atom)
	}
	return mask
}

func mustTrue(t testing.TB, manager *guard.Manager) Mask {
	t.Helper()
	value, ok := True(manager)
	if !ok {
		t.Fatal("true support")
	}
	return value
}

func TestMaskRejectsForeignAuthority(t *testing.T) {
	first, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	left, right := New(first), New(second)
	if left == nil || right == nil {
		t.Fatal("mask work creation failed")
	}
	candidate, ok := left.Literal(1, true)
	if !ok {
		t.Fatal("candidate construction failed")
	}
	if right.Valid(candidate) {
		t.Fatal("foreign candidate mask passed authority validation")
	}
	if _, ok := right.Not(candidate); ok {
		t.Fatal("foreign candidate was accepted for Boolean construction")
	}
	if !left.Seal() || !candidate.Valid() {
		t.Fatal("candidate publication failed")
	}
	if right.Valid(candidate) || candidate.Equal(right.True()) {
		t.Fatal("cross-manager masks compared in one valuation universe")
	}
}

func TestTruePublishesColdUnconstrainedMask(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	region, ok := True(manager)
	if !ok || !region.Valid() || region.Manager() != manager {
		t.Fatal("cold true mask did not publish")
	}
	for _, value := range []bool{false, true} {
		if !region.Matches(func(guard.Atom) bool { return value }) {
			t.Fatalf("true mask rejected valuation %t", value)
		}
	}
	if _, ok := True(nil); ok {
		t.Fatal("nil guard manager produced a true mask")
	}
}

func TestIdenticalMaskOperationsAreExactAndAllocationFree(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	work := New(manager)
	mask, ok := work.Literal(1, true)
	if !ok || !work.Seal() {
		t.Fatal("mask setup failed")
	}

	split, ok := Three(mask, mask)
	if !ok || !split.Valid(manager) || split.Left() != mask || split.Right() != mask || split.Overlap() != mask || split.Union() != mask {
		t.Fatal("identical Three did not preserve the input mask")
	}
	falseMask, ok := FromGuard(manager, manager.False())
	if !ok || split.LeftOnly() != falseMask || split.RightOnly() != falseMask {
		t.Fatal("identical Three did not produce empty exclusive regions")
	}
	if (Split{}).Valid(manager) {
		t.Fatal("unconstructed Split passed validation")
	}
	intersection, ok := Intersect(mask, mask)
	if !ok || intersection != mask {
		t.Fatal("identical Intersect did not return its operand")
	}
	union, ok := Union(mask, mask)
	if !ok || union != mask {
		t.Fatal("identical Union did not return its operand")
	}

	// Warm every call site before measuring the semantic fast paths.
	_, _ = Three(mask, mask)
	_, _ = Intersect(mask, mask)
	_, _ = Union(mask, mask)
	if allocations := testing.AllocsPerRun(1_000, func() {
		got, ok := Three(mask, mask)
		if !ok || !got.Valid(manager) || got.Left() != mask || got.LeftOnly() != falseMask || got.RightOnly() != falseMask {
			t.Fatal("identical Three changed")
		}
	}); allocations != 0 {
		t.Fatalf("identical Three allocated %f times", allocations)
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		got, ok := Intersect(mask, mask)
		if !ok || got != mask {
			t.Fatal("identical Intersect changed")
		}
	}); allocations != 0 {
		t.Fatalf("identical Intersect allocated %f times", allocations)
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		got, ok := Union(mask, mask)
		if !ok || got != mask {
			t.Fatal("identical Union changed")
		}
	}); allocations != 0 {
		t.Fatalf("identical Union allocated %f times", allocations)
	}
}

func TestReusableSupportShellKeepsMeaningAndRecoversAfterDiscard(t *testing.T) {
	manager, err := guard.New([]guard.Atom{3, 7})
	if err != nil {
		t.Fatal(err)
	}
	build := New(manager)
	left, ok := build.Conjoin(build.True(), 3, true)
	if !ok {
		t.Fatal("left support construction failed")
	}
	right, ok := build.Conjoin(build.True(), 7, true)
	if !ok || !build.Seal() {
		t.Fatal("right support construction failed")
	}

	shell := New(manager)
	first, ok := ThreeWithWork(shell, nil, left, right)
	if !ok || !first.Valid(manager) {
		t.Fatal("first reusable split failed")
	}
	second, ok := ThreeWithWork(shell, nil, left, right)
	if !ok {
		t.Fatal("repeated reusable split failed")
	}
	if !second.Overlap().Valid() || !second.Overlap().Equal(first.Overlap()) {
		t.Fatal("successful Seal did not preserve valid equivalent support")
	}

	if _, ok := ThreeWithWork(shell, func() bool { return false }, left, right); ok {
		t.Fatal("cancelled reusable split succeeded")
	}
	third, ok := ThreeWithWork(shell, nil, left, right)
	if !ok {
		t.Fatal("shell did not recover after failed transaction")
	}
	if !third.Overlap().Equal(first.Overlap()) {
		t.Fatal("failed transaction poisoned later Boolean result")
	}
}

func TestReusableSupportEntailsUsesReadScratchAfterConstructionSeal(t *testing.T) {
	manager, err := guard.New([]guard.Atom{3, 7})
	if err != nil {
		t.Fatal(err)
	}
	builder := New(manager)
	premise, ok := builder.Conjoin(builder.True(), 3, true)
	if !ok {
		t.Fatal("premise")
	}
	premise, ok = builder.Conjoin(premise, 7, true)
	if !ok || !builder.Seal() {
		t.Fatal("premise seal")
	}
	conclusionBuilder := New(manager)
	conclusion, ok := conclusionBuilder.Literal(3, true)
	if !ok || !conclusionBuilder.Seal() {
		t.Fatal("conclusion seal")
	}

	// The exact same shell crosses from a completed construction transaction to
	// a read transaction; this is the Binding lifecycle exercised in production.
	construction := New(manager)
	if _, ok := ThreeWithWork(construction, nil, premise, conclusion); !ok || construction.Open() {
		t.Fatal("construction shell did not reach its terminal seal")
	}
	reader := construction
	if !reader.EntailsWithCheckpoint(nil, premise, conclusion) || reader.EntailsWithCheckpoint(nil, conclusion, premise) {
		t.Fatal("support entailment law failed after prior construction seal")
	}
	if !reader.EntailsWithCheckpoint(nil, premise, conclusion) {
		t.Fatal("warmed support entailment changed")
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		if !reader.EntailsWithCheckpoint(nil, premise, conclusion) {
			t.Fatal("reused support entailment changed")
		}
	}); allocations != 0 {
		t.Fatalf("reused support entailment allocated %f times", allocations)
	}

	polls := 0
	if reader.EntailsWithCheckpoint(func() bool {
		polls++
		return false
	}, premise, conclusion) || polls == 0 {
		t.Fatalf("cancelled support entailment = %t after %d polls", reader.EntailsWithCheckpoint(nil, premise, conclusion), polls)
	}
	foreignManager, err := guard.New([]guard.Atom{3, 7})
	if err != nil {
		t.Fatal(err)
	}
	foreign, ok := FromGuard(foreignManager, foreignManager.True())
	if !ok {
		t.Fatal("foreign support")
	}
	if reader.EntailsWithCheckpoint(nil, premise, foreign) {
		t.Fatal("foreign support crossed reusable read authority")
	}
}
