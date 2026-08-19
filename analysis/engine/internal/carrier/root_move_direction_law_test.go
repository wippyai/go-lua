package carrier

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/change"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// orderedRootOperation is a sealed four-element plane whose roots are the
// subsets of a two-element set: root 1 is the bottom, roots 2 and 3 are the
// two incomparable atoms, and root 4 is the top. It is the smallest universe
// that separates the four answers a root replacement can have -- equal,
// strictly above, strictly below, and unordered -- so a publication law can
// be exhaustive over every ordered pair instead of sampling one of them.
type orderedRootOperation struct {
	*adversarialOperation
	initial uint64
}

func newOrderedRootOperation(t testing.TB, guards *guard.Manager, initial uint64) *orderedRootOperation {
	t.Helper()
	return &orderedRootOperation{adversarialOperation: newAdversarialOperation(t, guards), initial: initial}
}

func (operation *orderedRootOperation) Preflight() (SlotOperation, bool) {
	if operation == nil || operation.adversarialOperation == nil || operation.prepared {
		return nil, false
	}
	operation.prepared = true
	return operation, true
}

func (operation *orderedRootOperation) Attach(owner SlotOwner) RootHandle {
	if operation == nil || !operation.prepared {
		panic("invalid ordered-root attachment")
	}
	operation.issuer.Attach(owner)
	root, ok := operation.issuer.IssueRoot(operation.initial)
	if !ok {
		panic("ordered root")
	}
	return root
}

func (operation *orderedRootOperation) ValidRoot(root RootHandle) bool {
	id, ok := operation.issuer.ResolveRoot(root)
	return ok && id >= 1 && id <= 4
}

func (operation *orderedRootOperation) NewWork() (SlotWork, bool) {
	return orderedRootWork{operation: operation}, operation != nil
}

// orderedRootMembers is the sealed universe's order: root id n stands for the
// subset whose members are the low bits of n-1.
func orderedRootMembers(id uint64) uint64 { return id - 1 }

func orderedRootLessOrEq(left, right uint64) bool {
	return orderedRootMembers(left)&^orderedRootMembers(right) == 0
}

type orderedRootWork struct {
	adversarialWork
	operation *orderedRootOperation
}

func (work orderedRootWork) EqualUnder(left, right RootHandle, _ support.Mask) bool {
	leftID, leftOK := work.operation.issuer.ResolveRoot(left)
	rightID, rightOK := work.operation.issuer.ResolveRoot(right)
	return leftOK && rightOK && leftID == rightID
}

func (work orderedRootWork) LessOrEqUnder(left, right RootHandle, _ support.Mask) bool {
	leftID, leftOK := work.operation.issuer.ResolveRoot(left)
	rightID, rightOK := work.operation.issuer.ResolveRoot(right)
	return leftOK && rightOK && orderedRootLessOrEq(leftID, rightID)
}

// orderedRootPublication runs one publication that replaces the slot's root
// under an unchanged support region, which is the exact cut whose direction
// the publishing operation owns.
func orderedRootPublication(t testing.TB, before, after uint64) change.Set {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	operation := newOrderedRootOperation(t, manager, before)
	composition, ok := attachTestComposition(t, []FactorOperation{operation})
	if !ok {
		t.Fatal("composition")
	}
	state, ok := NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	work, ok := composition.NewWork()
	if !ok {
		t.Fatal("work")
	}
	t.Cleanup(func() { work.Close() })
	predecessor, ok := state.HandleAt(0)
	if !ok {
		t.Fatal("predecessor root")
	}
	successor, ok := operation.issuer.IssueRoot(after)
	if !ok {
		t.Fatal("successor root")
	}
	handle, ok := operation.issuer.IssueChange(predecessor, RootHandle{}, &checkpointPublisher{root: successor}, support.Mask{}, nil, nil, nil)
	if !ok {
		t.Fatal("change")
	}
	patch, ok := work.Accept(state, handle)
	if !ok {
		t.Fatal("accept")
	}
	next, changes, ok := work.Commit(state, []Patch{patch})
	if !ok {
		t.Fatal("commit")
	}
	published, ok := next.HandleAt(0)
	if !ok || !sameRoot(published, successor) {
		t.Fatal("publication did not install the successor root")
	}
	if !next.Support().SameHandle(state.Support()) {
		t.Fatal("publication moved the support region")
	}
	return changes.Evidence()
}

// A root replacement under an unchanged support region is classified by the
// plane that owns the roots, not guessed. The publication ascends when the
// plane orders the successor above its predecessor, descends when it orders
// it below, moves neither axis when the two are order-equal, and carries both
// axes when the plane cannot order them at all.
func TestRootReplacementIsClassifiedByThePlaneOrder(t *testing.T) {
	for before := uint64(1); before <= 4; before++ {
		for after := uint64(1); after <= 4; after++ {
			evidence := orderedRootPublication(t, before, after)
			ascends := orderedRootLessOrEq(before, after)
			descends := orderedRootLessOrEq(after, before)
			want := change.Known
			switch {
			case ascends && descends:
			case ascends:
				want |= change.Ascends
			case descends:
				want |= change.Descends
			default:
				want |= change.Ascends | change.Descends
			}
			if evidence.Direction != want {
				t.Fatalf("root %d -> %d classified %d, want %d", before, after, evidence.Direction, want)
			}
			// The soundness half: a retained accumulator may only be reused
			// across a replacement the plane proved to be an upper bound. An
			// unordered replacement carries the descent half beside its
			// ascent half precisely so that it is refused.
			if evidence.Direction&change.Descends == 0 && !ascends {
				t.Fatalf("root %d -> %d answered an unqualified ascent the plane did not prove", before, after)
			}
			if evidence.Admits() != ascends {
				t.Fatalf("root %d -> %d admits=%v while the plane proves upper bound=%v", before, after, evidence.Admits(), ascends)
			}
		}
	}
}

// A descent must be issued whenever the plane orders the successor strictly
// below its predecessor, and the classification must never silently drop it
// because the support region did not move.
func TestStrictRootDescentUnderUnchangedSupportRefusesReuse(t *testing.T) {
	evidence := orderedRootPublication(t, 4, 2)
	if evidence.Direction&change.Descends == 0 || evidence.Direction&change.Ascends != 0 {
		t.Fatalf("a strict descent classified %d", evidence.Direction)
	}
	if evidence.Admits() {
		t.Fatal("a descending root replacement was admitted for accumulator reuse")
	}
}

// A strict ascent is admissible: the whole point of classifying honestly is
// that the ordinary Kleene step keeps its retained accumulator.
func TestStrictRootAscentUnderUnchangedSupportAdmitsReuse(t *testing.T) {
	evidence := orderedRootPublication(t, 2, 4)
	if evidence.Direction != change.Known|change.Ascends {
		t.Fatalf("a strict ascent classified %d", evidence.Direction)
	}
	if !evidence.Admits() {
		t.Fatal("an ascending root replacement refused accumulator reuse")
	}
}

// An unordered replacement is not an ascent. It carries both axes, so it is
// refused exactly as a descent is.
func TestUnorderedRootReplacementCarriesBothAxes(t *testing.T) {
	evidence := orderedRootPublication(t, 2, 3)
	if evidence.Direction != change.Known|change.Ascends|change.Descends {
		t.Fatalf("an unordered replacement classified %d", evidence.Direction)
	}
	if evidence.Admits() {
		t.Fatal("an unordered root replacement was admitted for accumulator reuse")
	}
}
