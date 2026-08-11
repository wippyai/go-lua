package guard

import "testing"

func TestIdentityIsCanonicalAcrossManagerGenerationsAndConstructionOrder(t *testing.T) {
	first := newTestManager(t)
	firstWork := first.NewWork()
	firstA := literal(t, firstWork, testA)
	firstB := literal(t, firstWork, testB)
	firstC := literal(t, firstWork, testC)
	firstFormula := firstWork.Or(firstWork.And(firstA, firstWork.Not(firstB)), firstC)
	firstWork.Seal()

	second := newTestManager(t)
	secondWork := second.NewWork()
	secondC := literal(t, secondWork, testC)
	secondB := literal(t, secondWork, testB)
	secondA := literal(t, secondWork, testA)
	secondFormula := secondWork.Or(secondC, secondWork.And(secondA, secondWork.Not(secondB)))
	secondWork.Seal()

	firstID, firstOK := Identity(firstFormula)
	secondID, secondOK := Identity(secondFormula)
	if !firstOK || !secondOK || !firstID.Available() || firstID != secondID {
		t.Fatalf("cross-generation canonical identity = %x/%t %x/%t", firstID, firstOK, secondID, secondOK)
	}
}

func TestIdentityDistinguishesFormulaAndAtom(t *testing.T) {
	manager := newTestManager(t)
	work := manager.NewWork()
	a := literal(t, work, testA)
	b := literal(t, work, testB)
	notA := work.Not(a)
	work.Seal()

	aID, aOK := Identity(a)
	bID, bOK := Identity(b)
	notAID, notAOK := Identity(notA)
	if !aOK || !bOK || !notAOK || aID == bID || aID == notAID || bID == notAID {
		t.Fatalf("formula identities collapsed: a=%x b=%x not-a=%x", aID, bID, notAID)
	}
}

func TestIdentityCanonicalizesTerminalsIndependentOfManagerUniverse(t *testing.T) {
	empty, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	populated := newTestManager(t)

	emptyFalse, emptyFalseOK := Identity(empty.False())
	populatedFalse, populatedFalseOK := Identity(populated.False())
	emptyTrue, emptyTrueOK := Identity(empty.True())
	populatedTrue, populatedTrueOK := Identity(populated.True())
	if !emptyFalseOK || !populatedFalseOK || !emptyTrueOK || !populatedTrueOK || emptyFalse != populatedFalse || emptyTrue != populatedTrue || emptyFalse == emptyTrue {
		t.Fatalf("terminal identities = false:%x/%x true:%x/%x", emptyFalse, populatedFalse, emptyTrue, populatedTrue)
	}
}

func TestIdentityRejectsUnsealedAndForeignlessGuard(t *testing.T) {
	if _, ok := Identity(Guard{}); ok {
		t.Fatal("zero guard received an identity")
	}
	manager := newTestManager(t)
	work := manager.NewWork()
	unsealed := literal(t, work, testA)
	if _, ok := Identity(unsealed); ok {
		t.Fatal("unsealed guard received an identity")
	}
	work.Discard()
}

func TestIdentityTraversesDeepFormulaIteratively(t *testing.T) {
	const count = 100_000
	order := ascending(count)
	manager, err := New(order)
	if err != nil {
		t.Fatal(err)
	}
	work := manager.NewWork()
	root := manager.True()
	for index := len(order) - 1; index >= 0; index-- {
		root = work.And(literal(t, work, order[index]), root)
	}
	work.Seal()

	first, firstOK := Identity(root)
	second, secondOK := Identity(root)
	if !firstOK || !secondOK || !first.Available() || first != second {
		t.Fatalf("deep identity = %x/%t %x/%t", first, firstOK, second, secondOK)
	}
}

func TestIdentityWithCheckpointCancelsWithoutPartialIdentity(t *testing.T) {
	const count = 512
	order := ascending(count)
	manager, err := New(order)
	if err != nil {
		t.Fatal(err)
	}
	work := manager.NewWork()
	root := manager.True()
	for index := len(order) - 1; index >= 0; index-- {
		root = work.And(literal(t, work, order[index]), root)
	}
	work.Seal()

	polls := 0
	cancelled, cancelledOK := IdentityWithCheckpoint(root, func() bool {
		polls++
		return polls < 80
	})
	if cancelledOK || cancelled.Available() || polls < 80 {
		t.Fatalf("cancelled identity = %x/%t after %d polls", cancelled, cancelledOK, polls)
	}
	complete, completeOK := Identity(root)
	if !completeOK || !complete.Available() {
		t.Fatal("cancelled traversal changed the sealed formula")
	}
}
