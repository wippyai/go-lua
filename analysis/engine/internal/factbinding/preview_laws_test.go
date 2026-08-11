package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestPreviewSerialSameSlotReadsUnpublishedRoot proves the essential postfix
// law: serial groups may write the same Factor twice, with the latter staging
// against the former's temporary root, while no root-store entry is reserved
// or published and Abort revokes every temporary plane.
func TestPreviewSerialSameSlotReadsUnpublishedRoot(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	binding, base, slot, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	work := newWork(t, composition)
	target := fixture.target(t, 0, carrier.StrongTarget)
	// The committed upper bound is deliberately prepared before the preview;
	// preview comparison must use the ordinary exact carrier order, not an
	// ad-hoc temporary comparison.
	upper := writeState(t, work, binding, fixture, base, slot, whole, 2)
	preview, ok := work.BeginPreview(base)
	if !ok {
		t.Fatal("begin preview")
	}

	first := acceptedTransferWrite(t, binding, work, preview.State(), target, whole, 1)
	firstState, _, ok := preview.Commit([]carrier.Patch{first})
	if !ok || firstState.Valid() {
		t.Fatalf("first temporary state = ok:%t normal-valid:%t", ok, firstState.Valid())
	}
	firstRoot, ok := firstState.HandleAt(0)
	if !ok {
		t.Fatal("first temporary root")
	}
	if value, present, valid := observedExactValue(binding, work, firstRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || value != 1 {
		t.Fatalf("first temporary value = %d/%t/%t, want 1/true/true", value, present, valid)
	}

	// This Begin resolves firstRoot as its typed predecessor.  It is the
	// concrete proof that serial duplicate-slot groups see unpublished output.
	second := acceptedTransferWrite(t, binding, work, preview.State(), target, whole, 2)
	secondState, _, ok := preview.Commit([]carrier.Patch{second})
	if !ok {
		t.Fatal("second serial preview commit")
	}
	secondRoot, ok := secondState.HandleAt(0)
	if !ok {
		t.Fatal("second temporary root")
	}
	if value, present, valid := observedExactValue(binding, work, secondRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); !valid || !present || value != 2 {
		t.Fatalf("second temporary value = %d/%t/%t, want 2/true/true", value, present, valid)
	}
	if preview.LessOrEq(base) {
		t.Fatal("preview value 2 was below empty predecessor")
	}
	if !preview.LessOrEq(upper) {
		t.Fatal("preview value 2 was not below matching committed upper bound")
	}
	if !work.LessOrEqUnder(upper, secondState) {
		t.Fatal("matching committed result was not below preview candidate")
	}

	// No ordinary cut may turn a temporary State into a published root vector.
	if _, _, committed := work.Commit(secondState, nil); committed {
		t.Fatal("ordinary commit accepted preview state")
	}
	if view, restricted := preview.Restrict(whole); !restricted {
		t.Fatal("preview restriction")
	} else if _, _, transferred := work.Transfer(secondState, view, nil); transferred {
		t.Fatal("ordinary transfer accepted preview state")
	}
	if _, _, merged := work.Merge3Under(carrier.Join, secondState, secondState, composition.AllMergeScope()); merged {
		t.Fatal("ordinary merge accepted preview state")
	}
	if _, _, replaced := work.Replace(secondState, secondState); replaced {
		t.Fatal("ordinary replace accepted preview state")
	}
	if !preview.Abort() {
		t.Fatal("abort preview")
	}
	if _, _, valid := observedExactValue(binding, work, secondRoot, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false }); valid {
		t.Fatal("aborted preview root remained readable")
	}
}

// TestPreviewObservationReadsTemporaryRootUntilAbort proves that the normal
// SlotWork observation route used by Product can consume a live temporary
// root.  The same escaped row must fail immediately after Preview revokes its
// root, even though the observation generation is still open.
func TestPreviewObservationReadsTemporaryRootUntilAbort(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	binding, base, slot, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	preview, ok := work.BeginPreview(base)
	if !ok {
		t.Fatal("begin preview")
	}
	patch := acceptedTransferWrite(t, binding, work, preview.State(), declared.target[0], whole, 9)
	temporary, _, ok := preview.Commit([]carrier.Patch{patch})
	if !ok {
		t.Fatal("preview write")
	}
	root, ok := temporary.HandleAt(slot)
	if !ok {
		t.Fatal("temporary root")
	}
	slotWork, ok := work.SlotWork(slot)
	if !ok || !slotWork.BeginObservation() {
		t.Fatal("begin observation")
	}
	var escaped carrier.ObservationRow
	if !slotWork.ObserveUnder(root, declared.exact[0], whole, func(row carrier.ObservationRow) bool {
		escaped = row
		observation, resolved := binding.ResolveObservation(slotWork, row)
		if !resolved || observation.Count() != 1 {
			return false
		}
		entry, present := observation.At(0)
		value, stored := entry.Read()
		return present && stored && value == 9
	}) {
		t.Fatal("temporary root observation")
	}
	if !preview.Abort() {
		t.Fatal("abort preview")
	}
	if _, resolved := binding.ResolveObservation(slotWork, escaped); resolved {
		t.Fatal("aborted preview root remained observable")
	}
	if !slotWork.EndObservation() {
		t.Fatal("end observation")
	}
}

// TestPreviewMixedPublishedAndTemporaryNoOp proves that preview admission
// accepts a no-op on a still-published Factor while another Factor in the
// same candidate already carries a temporary root.  This is the mixed-root
// case serial postfix must preserve without widening the normal root law.
func TestPreviewMixedPublishedAndTemporaryNoOp(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	first, firstFixture := newLawBinding(t, manager, true)
	second, _ := newLawBinding(t, manager, true)
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{first, second})
	if !ok {
		t.Fatal("composition")
	}
	base, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	work := newWork(t, composition)
	preview, ok := work.BeginPreview(base)
	if !ok {
		t.Fatal("begin preview")
	}
	write := acceptedTransferWrite(t, first, work, preview.State(), firstFixture.target(t, 0, carrier.StrongTarget), whole, 1)
	if _, _, ok := preview.Commit([]carrier.Patch{write}); !ok {
		t.Fatal("temporary first-factor write")
	}

	// Accept without a write produces the normal published root as `after`.
	// Its exact predecessor State simultaneously contains first's temporary
	// root, so this reaches carrier's mixed retained/temporary root admission.
	staged := second.Begin(work, preview.State())
	if staged == nil {
		t.Fatal("begin published no-op")
	}
	noop, ok := staged.Accept(work)
	if !ok {
		t.Fatal("accept published no-op")
	}
	if _, changes, ok := preview.Commit([]carrier.Patch{noop}); !ok || !changes.Empty() {
		t.Fatalf("mixed no-op commit = ok:%t empty:%t", ok, changes.Empty())
	}
	if !preview.Abort() {
		t.Fatal("abort preview")
	}
}

// TestPreviewSerialTransferMatchesPublishedTransfer proves the real From
// shape: a restricted predecessor is transferred twice through the same
// Factor, and the second callback stages against the first temporary root.
// Its final semantic result is exactly the published Transfer result.
func TestPreviewSerialTransferMatchesPublishedTransfer(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	whole := regions.True()
	on, ok := regions.Literal(1, true)
	if !ok || !regions.Seal() {
		t.Fatal("on support")
	}
	binding, base, _, composition, fixture := bindingState(t, manager, lawInput(true), whole)
	target := fixture.target(t, 0, carrier.StrongTarget)

	publishedWork := newWork(t, composition)
	firstView, ok := base.Restrict(on)
	if !ok {
		t.Fatal("published first view")
	}
	first := acceptedTransferWrite(t, binding, publishedWork, base, target, on, 1)
	publishedFirst, firstChanges, ok := publishedWork.Transfer(base, firstView, []carrier.Patch{first})
	if !ok {
		t.Fatal("published first transfer")
	}
	secondView, ok := publishedFirst.Restrict(on)
	if !ok {
		t.Fatal("published second view")
	}
	second := acceptedTransferWrite(t, binding, publishedWork, publishedFirst, target, on, 2)
	publishedFinal, secondChanges, ok := publishedWork.Transfer(publishedFirst, secondView, []carrier.Patch{second})
	if !ok {
		t.Fatal("published second transfer")
	}

	previewWork := newWork(t, composition)
	preview, ok := previewWork.BeginPreview(base)
	if !ok {
		t.Fatal("begin preview")
	}
	previewFirstView, ok := preview.Restrict(on)
	if !ok {
		t.Fatal("preview first view")
	}
	previewFirst := acceptedTransferWrite(t, binding, previewWork, preview.State(), target, on, 1)
	if _, previewFirstChanges, ok := preview.Transfer(previewFirstView, []carrier.Patch{previewFirst}); !ok || !sameChanges(firstChanges, previewFirstChanges) {
		t.Fatalf("preview first transfer equivalence = ok:%t changes:%t", ok, sameChanges(firstChanges, previewFirstChanges))
	}
	previewSecondView, ok := preview.Restrict(on)
	if !ok {
		t.Fatal("preview second view")
	}
	previewSecond := acceptedTransferWrite(t, binding, previewWork, preview.State(), target, on, 2)
	previewFinal, previewSecondChanges, ok := preview.Transfer(previewSecondView, []carrier.Patch{previewSecond})
	if !ok || !sameChanges(secondChanges, previewSecondChanges) {
		t.Fatalf("preview second transfer equivalence = ok:%t changes:%t", ok, sameChanges(secondChanges, previewSecondChanges))
	}
	if !preview.LessOrEq(publishedFinal) || !previewWork.LessOrEqUnder(publishedFinal, previewFinal) {
		t.Fatal("preview and published serial Transfer results differ")
	}
	if !preview.Abort() {
		t.Fatal("abort preview")
	}
}

func sameChanges(left, right carrier.ChangeSet) bool {
	if !left.Added().Equal(right.Added()) || !left.Removed().Equal(right.Removed()) || left.FactorCount() != right.FactorCount() || left.Count() != right.Count() {
		return false
	}
	for index := 0; index < left.FactorCount(); index++ {
		leftRow, leftOK := left.FactorAt(index)
		rightRow, rightOK := right.FactorAt(index)
		if !leftOK || !rightOK || leftRow.Slot() != rightRow.Slot() || !leftRow.Region().Equal(rightRow.Region()) {
			return false
		}
	}
	for index := 0; index < left.Count(); index++ {
		leftRow, leftOK := left.At(index)
		rightRow, rightOK := right.At(index)
		if !leftOK || !rightOK || !leftRow.Unit().Same(rightRow.Unit()) || !leftRow.Region().Equal(rightRow.Region()) {
			return false
		}
	}
	return true
}
