package diagram

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

type testFactor uint64
type testKey uint64

const (
	factorFirst  testFactor = 9
	factorSecond testFactor = 2
)

type diagramFixture struct {
	diagram    *Diagram[testFactor, testKey, uint8]
	manager    *guard.Manager
	trueAtOne  support.Mask
	falseAtOne support.Mask
	trueAtTwo  support.Mask
	values     [3]terminal.ID[uint8]
}

func newDiagramFixture(t testing.TB) diagramFixture {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	masks := support.New(manager)
	if masks == nil {
		t.Fatal("support work creation failed")
	}
	trueAtOne, ok := masks.Literal(1, true)
	if !ok {
		t.Fatal("first literal failed")
	}
	falseAtOne, ok := masks.Not(trueAtOne)
	if !ok {
		t.Fatal("first complement failed")
	}
	trueAtTwo, ok := masks.Literal(2, true)
	if !ok {
		t.Fatal("second literal failed")
	}
	if !masks.Seal() {
		t.Fatal("support seal failed")
	}
	values, ok := terminal.New(terminal.Config[uint8]{
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
	})
	if !ok {
		t.Fatal("terminal arena creation failed")
	}
	var ids [3]terminal.ID[uint8]
	for index, value := range []uint8{10, 20, 30} {
		ids[index], ok = values.Admit(value)
		if !ok {
			t.Fatalf("terminal %d admission failed", value)
		}
	}
	if !values.Seal() {
		t.Fatal("terminal seal failed")
	}
	diagram, ok := New(Config[testFactor, testKey, uint8]{
		// The intentionally nonnumeric order is the schema order tested below.
		Factors:   []testFactor{factorFirst, factorSecond},
		Terminals: values,
		Guards:    manager,
	})
	if !ok {
		t.Fatal("diagram creation failed")
	}
	return diagramFixture{diagram: diagram, manager: manager, trueAtOne: trueAtOne, falseAtOne: falseAtOne, trueAtTwo: trueAtTwo, values: ids}
}

func valuation(one, two bool) func(guard.Atom) bool {
	return func(atom guard.Atom) bool {
		switch atom {
		case 1:
			return one
		case 2:
			return two
		default:
			return false
		}
	}
}

func (fixture diagramFixture) assertAt(t testing.TB, root Root[testFactor, testKey, uint8], factor testFactor, key testKey, when func(guard.Atom) bool, want terminal.ID[uint8], present bool) {
	t.Helper()
	got, gotPresent, valid := fixture.diagram.At(root, factor, key, when)
	if !valid || gotPresent != present || got != want {
		t.Fatalf("At(%d,%d) = %v/%t/%t, want %v/%t/true", factor, key, got, gotPresent, valid, want, present)
	}
}

func TestPartitionValueTerminalsTraversesDeepPublicDecisionChainIteratively(t *testing.T) {
	// Build the deep symbolic input through the public fact boundary. Every
	// literal contributes one disjoint stored region; the remaining valuation
	// is sparse. Partitioning must visit the exact terminal regions without
	// enumerating valuations or consuming the Go call stack.
	const depth = 256
	atoms := make([]guard.Atom, depth)
	for index := range atoms {
		atoms[index] = guard.Atom(index + 1)
	}
	manager, err := guard.New(atoms)
	if err != nil {
		t.Fatal(err)
	}
	values, ok := terminal.New(terminal.Config[uint8]{
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
	})
	if !ok {
		t.Fatal("terminal arena")
	}
	stored, ok := values.Admit(1)
	if !ok || !values.Seal() {
		t.Fatal("terminal setup")
	}
	diagram, ok := New(Config[testFactor, testKey, uint8]{
		Factors:   []testFactor{factorFirst},
		Terminals: values,
		Guards:    manager,
	})
	if !ok {
		t.Fatal("diagram")
	}
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("support work")
	}
	literals := make([]support.Mask, depth)
	for index := range literals {
		literals[index], ok = regions.Literal(atoms[index], true)
		if !ok {
			t.Fatalf("literal %d", index)
		}
	}
	if !regions.Seal() {
		t.Fatal("support seal")
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	builder := diagram.Begin()
	root := diagram.Empty()
	for index, literal := range literals {
		root, ok = builder.Set(root, factorFirst, 1, literal, stored)
		if !ok {
			t.Fatalf("deep write %d", index)
		}
	}
	root, ok = builder.Seal(root)
	if !ok || !diagram.Valid(root) {
		t.Fatal("deep root seal")
	}
	value, present, valid := diagram.Get(root, factorFirst, 1)
	if !valid || !present {
		t.Fatal("deep value unavailable")
	}
	storedLeaves, absentLeaves := 0, 0
	var storedRegion support.Mask
	completed, valid := diagram.PartitionValueTerminals(value, whole, nil, func(id terminal.ID[uint8], region support.Mask) bool {
		if id == stored {
			storedLeaves++
			storedRegion = region
		} else if id == (terminal.ID[uint8]{}) {
			absentLeaves++
		} else {
			t.Fatal("foreign terminal")
		}
		return true
	})
	// The depth literals all carry the one stored terminal, so the partition
	// publishes their union as one piece and sparse absence as the other.
	if !completed || !valid || storedLeaves != 1 || absentLeaves != 1 {
		t.Fatalf("deep partition = completed:%t valid:%t stored:%d absent:%d", completed, valid, storedLeaves, absentLeaves)
	}
	// Coalescing keeps every cell the chain wrote: each literal still selects
	// the stored piece.
	for index, literal := range literals {
		if !literal.Entails(storedRegion) {
			t.Fatalf("literal %d escaped the stored piece", index)
		}
	}
	if _, present, valid := diagram.At(root, factorFirst, 1, func(guard.Atom) bool { return false }); !valid || present {
		t.Fatal("all-false valuation must select sparse absence")
	}
	if got, present, valid := diagram.At(root, factorFirst, 1, func(atom guard.Atom) bool { return atom == 1 }); !valid || !present || got != stored {
		t.Fatal("stored literal valuation was not preserved")
	}
}

func TestColumnCarriesDistinctTerminalsOnOppositeBranches(t *testing.T) {
	fixture := newDiagramFixture(t)
	builder := fixture.diagram.Begin()
	root, ok := builder.Set(fixture.diagram.Empty(), factorFirst, 4, fixture.trueAtOne, fixture.values[0])
	if !ok {
		t.Fatal("true-branch fact write failed")
	}
	root, ok = builder.Set(root, factorFirst, 4, fixture.falseAtOne, fixture.values[1])
	if !ok {
		t.Fatal("false-branch fact write failed")
	}
	root, ok = builder.Seal(root)
	if !ok || !fixture.diagram.Valid(root) {
		t.Fatal("branch-valued root did not publish")
	}
	if count, ok := fixture.diagram.Count(root); !ok || count != 1 {
		t.Fatalf("branch-valued column count = %d/%t, want 1/true", count, ok)
	}
	fixture.assertAt(t, root, factorFirst, 4, valuation(true, false), fixture.values[0], true)
	fixture.assertAt(t, root, factorFirst, 4, valuation(false, false), fixture.values[1], true)
	fixture.assertAt(t, root, factorFirst, 5, valuation(true, false), terminal.ID[uint8]{}, false)
}

func TestSetDeleteArePersistentExactITEs(t *testing.T) {
	fixture := newDiagramFixture(t)
	firstBuilder := fixture.diagram.Begin()
	first, ok := firstBuilder.Set(fixture.diagram.Empty(), factorFirst, 4, fixture.trueAtOne, fixture.values[0])
	if !ok {
		t.Fatal("first write failed")
	}
	first, ok = firstBuilder.Seal(first)
	if !ok {
		t.Fatal("first root seal failed")
	}

	secondBuilder := fixture.diagram.Begin()
	second, ok := secondBuilder.Set(first, factorFirst, 4, fixture.falseAtOne, fixture.values[1])
	if !ok {
		t.Fatal("second write failed")
	}
	second, ok = secondBuilder.Seal(second)
	if !ok {
		t.Fatal("second root seal failed")
	}
	// Updating the successor cannot mutate its predecessor.
	fixture.assertAt(t, first, factorFirst, 4, valuation(true, false), fixture.values[0], true)
	fixture.assertAt(t, first, factorFirst, 4, valuation(false, false), terminal.ID[uint8]{}, false)
	fixture.assertAt(t, second, factorFirst, 4, valuation(true, false), fixture.values[0], true)
	fixture.assertAt(t, second, factorFirst, 4, valuation(false, false), fixture.values[1], true)

	thirdBuilder := fixture.diagram.Begin()
	third, ok := thirdBuilder.Set(second, factorFirst, 4, fixture.trueAtOne, fixture.values[2])
	if !ok {
		t.Fatal("ITE replacement failed")
	}
	third, ok = thirdBuilder.Seal(third)
	if !ok {
		t.Fatal("ITE replacement seal failed")
	}
	fixture.assertAt(t, third, factorFirst, 4, valuation(true, false), fixture.values[2], true)
	fixture.assertAt(t, third, factorFirst, 4, valuation(false, false), fixture.values[1], true)

	deleteBuilder := fixture.diagram.Begin()
	deleted, ok := deleteBuilder.Delete(third, factorFirst, 4, fixture.trueAtOne)
	if !ok {
		t.Fatal("branch deletion failed")
	}
	deleted, ok = deleteBuilder.Seal(deleted)
	if !ok {
		t.Fatal("branch deletion seal failed")
	}
	fixture.assertAt(t, deleted, factorFirst, 4, valuation(true, false), terminal.ID[uint8]{}, false)
	fixture.assertAt(t, deleted, factorFirst, 4, valuation(false, false), fixture.values[1], true)

	removeBuilder := fixture.diagram.Begin()
	removed, ok := removeBuilder.Delete(deleted, factorFirst, 4, fixture.falseAtOne)
	if !ok {
		t.Fatal("final branch deletion failed")
	}
	removed, ok = removeBuilder.Seal(removed)
	if !ok {
		t.Fatal("final branch deletion seal failed")
	}
	if count, ok := fixture.diagram.Count(removed); !ok || count != 0 {
		t.Fatalf("fully undefined column count = %d/%t, want 0/true", count, ok)
	}
	fixture.assertAt(t, removed, factorFirst, 4, valuation(true, false), terminal.ID[uint8]{}, false)
	fixture.assertAt(t, removed, factorFirst, 4, valuation(false, false), terminal.ID[uint8]{}, false)
}

func TestColumnsPreserveSharedGuardCorrelationAndSchemaOrder(t *testing.T) {
	fixture := newDiagramFixture(t)
	builder := fixture.diagram.Begin()
	root, ok := builder.Set(fixture.diagram.Empty(), factorSecond, 7, fixture.trueAtOne, fixture.values[2])
	if !ok {
		t.Fatal("second-factor true branch failed")
	}
	root, ok = builder.Set(root, factorSecond, 7, fixture.falseAtOne, fixture.values[0])
	if !ok {
		t.Fatal("second-factor false branch failed")
	}
	root, ok = builder.Set(root, factorFirst, 9, fixture.trueAtOne, fixture.values[0])
	if !ok {
		t.Fatal("first-factor high key failed")
	}
	root, ok = builder.Set(root, factorFirst, 1, fixture.trueAtTwo, fixture.values[1])
	if !ok {
		t.Fatal("first-factor low key failed")
	}
	root, ok = builder.Seal(root)
	if !ok {
		t.Fatal("correlated root seal failed")
	}

	// Both columns branch on atom 1.  The two complete valuations must select
	// the paired values, not an independently mixed combination.
	fixture.assertAt(t, root, factorSecond, 7, valuation(true, false), fixture.values[2], true)
	fixture.assertAt(t, root, factorSecond, 7, valuation(false, false), fixture.values[0], true)
	fixture.assertAt(t, root, factorFirst, 9, valuation(true, false), fixture.values[0], true)
	fixture.assertAt(t, root, factorFirst, 9, valuation(false, false), terminal.ID[uint8]{}, false)

	seen := make([][2]uint64, 0, 3)
	completed, valid := fixture.diagram.ForEach(root, func(fact Fact[testFactor, testKey, uint8]) bool {
		seen = append(seen, [2]uint64{uint64(fact.Factor), uint64(fact.Key)})
		return true
	})
	if !completed || !valid {
		t.Fatal("ordered fact stream failed")
	}
	want := [][2]uint64{{uint64(factorFirst), 1}, {uint64(factorFirst), 9}, {uint64(factorSecond), 7}}
	if len(seen) != len(want) {
		t.Fatalf("streamed %v, want %v", seen, want)
	}
	for index := range want {
		if seen[index] != want[index] {
			t.Fatalf("fact order %v, want %v", seen, want)
		}
	}
}

func TestBuilderMasksZipsAndExistsWithoutBreakingGuardCorrelation(t *testing.T) {
	fixture := newDiagramFixture(t)
	seed := fixture.diagram.Begin()
	root, ok := seed.Set(fixture.diagram.Empty(), factorFirst, 4, fixture.trueAtOne, fixture.values[0])
	if !ok {
		t.Fatal("true branch seed failed")
	}
	root, ok = seed.Set(root, factorFirst, 4, fixture.falseAtOne, fixture.values[1])
	if !ok {
		t.Fatal("false branch seed failed")
	}
	root, ok = seed.Seal(root)
	if !ok {
		t.Fatal("seed seal failed")
	}

	work := fixture.diagram.Begin()
	value, present, valid := work.Get(root, factorFirst, 4)
	if !valid || !present {
		t.Fatal("seed value unavailable")
	}
	masked, ok := work.Mask(value, fixture.trueAtOne)
	if !ok {
		t.Fatal("mask failed")
	}
	maskedRoot, ok := work.Put(fixture.diagram.Empty(), factorFirst, 4, masked)
	if !ok {
		t.Fatal("masked put failed")
	}

	// Zip retains exact low/high pairing.  Its operation handles undefined as
	// the sparse identity and otherwise selects the greater terminal value.
	joined, ok := work.Zip(value, masked, func(left, right terminal.ID[uint8]) (terminal.ID[uint8], bool) {
		if left == (terminal.ID[uint8]{}) {
			return right, true
		}
		if right == (terminal.ID[uint8]{}) {
			return left, true
		}
		leftValue, leftValid := fixture.diagram.Terminals().Value(left)
		rightValue, rightValid := fixture.diagram.Terminals().Value(right)
		if !leftValid || !rightValid {
			return terminal.ID[uint8]{}, false
		}
		if leftValue >= rightValue {
			return left, true
		}
		return right, true
	})
	if !ok {
		t.Fatal("zip failed")
	}
	exists, ok := work.Exists(joined, 1, func(left, right terminal.ID[uint8]) (terminal.ID[uint8], bool) {
		if left == (terminal.ID[uint8]{}) {
			return right, true
		}
		if right == (terminal.ID[uint8]{}) {
			return left, true
		}
		leftValue, leftValid := fixture.diagram.Terminals().Value(left)
		rightValue, rightValid := fixture.diagram.Terminals().Value(right)
		if !leftValid || !rightValid {
			return terminal.ID[uint8]{}, false
		}
		if leftValue >= rightValue {
			return left, true
		}
		return right, true
	})
	if !ok {
		t.Fatal("exists failed")
	}
	result, ok := work.Put(maskedRoot, factorFirst, 9, exists)
	if !ok {
		t.Fatal("exists put failed")
	}
	result, ok = work.Seal(result)
	if !ok {
		t.Fatal("semantic primitive seal failed")
	}
	fixture.assertAt(t, result, factorFirst, 4, valuation(true, false), fixture.values[0], true)
	fixture.assertAt(t, result, factorFirst, 4, valuation(false, false), terminal.ID[uint8]{}, false)
	// ∃ atom1 joins the two original branches: max(10,20) = 20 under every
	// remaining valuation, so the discharged atom cannot leak back in.
	fixture.assertAt(t, result, factorFirst, 9, valuation(true, false), fixture.values[1], true)
	fixture.assertAt(t, result, factorFirst, 9, valuation(false, true), fixture.values[1], true)
}

func TestCandidateTerminalPublishesOnlyWithItsFactRoot(t *testing.T) {
	fixture := newDiagramFixture(t)
	terminals := fixture.diagram.Terminals().Begin()
	if terminals == nil {
		t.Fatal("candidate terminal work creation failed")
	}
	created, ok := terminals.Admit(99)
	if !ok {
		t.Fatal("dynamic terminal admission failed")
	}
	if fixture.diagram.Terminals().Valid(created) {
		t.Fatal("base arena exposed a candidate terminal before fact publication")
	}
	builder := fixture.diagram.BeginWithTerminals(terminals)
	if builder == nil {
		t.Fatal("candidate fact builder creation failed")
	}
	root, ok := builder.Set(fixture.diagram.Empty(), factorFirst, 99, fixture.trueAtOne, created)
	if !ok {
		t.Fatal("candidate terminal write was rejected")
	}
	if fixture.diagram.Valid(root) {
		t.Fatal("candidate root escaped before coordinated seal")
	}
	root, ok = builder.Seal(root)
	if !ok || !fixture.diagram.Valid(root) {
		t.Fatal("candidate fact root did not publish")
	}
	if value, valid := fixture.diagram.Terminals().Value(created); !valid || value != 99 {
		t.Fatalf("published dynamic terminal = %d/%t, want 99/true", value, valid)
	}
	fixture.assertAt(t, root, factorFirst, 99, valuation(true, false), created, true)
	fixture.assertAt(t, root, factorFirst, 99, valuation(false, false), terminal.ID[uint8]{}, false)

	// Importing a root that contains a terminal published from Work must retain
	// its exact meaning under the base Arena's authority.
	reuse := fixture.diagram.Begin()
	noop, ok := reuse.Set(root, factorFirst, 99, fixture.trueAtOne, created)
	if !ok || fixture.diagram.Valid(noop) {
		t.Fatal("published candidate terminal was not safely accepted as a private no-op")
	}
	published, ok := reuse.Seal(noop)
	if !ok || !fixture.diagram.Equal(published, root) {
		t.Fatal("candidate-terminal no-op did not preserve the sealed relation")
	}
}

func TestDiagramRejectsForeignTerminalAndGuardUniverses(t *testing.T) {
	fixture := newDiagramFixture(t)
	foreignTerminals, ok := terminal.New(terminal.Config[uint8]{
		Equal:       func(left, right uint8) bool { return left == right },
		Fingerprint: func(value uint8) uint64 { return uint64(value) },
	})
	if !ok {
		t.Fatal("foreign terminal arena creation failed")
	}
	foreignValue, ok := foreignTerminals.Admit(10)
	if !ok || !foreignTerminals.Seal() {
		t.Fatal("foreign terminal publication failed")
	}
	foreignManager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	foreignMasks := support.New(foreignManager)
	foreignMask, ok := foreignMasks.Literal(1, true)
	if !ok || !foreignMasks.Seal() {
		t.Fatal("foreign support publication failed")
	}
	builder := fixture.diagram.Begin()
	if _, ok := builder.Set(fixture.diagram.Empty(), factorFirst, 1, fixture.trueAtOne, foreignValue); ok {
		t.Fatal("foreign terminal identity entered fact diagram")
	}
	if _, ok := builder.Set(fixture.diagram.Empty(), factorFirst, 1, foreignMask, fixture.values[0]); ok {
		t.Fatal("foreign guard universe entered fact diagram")
	}
	if root, ok := builder.Seal(fixture.diagram.Empty()); !ok || !fixture.diagram.Valid(root) {
		t.Fatal("rejected candidate left builder unable to publish valid predecessor")
	}
}

func TestCandidateColumnsCannotEscapePublication(t *testing.T) {
	fixture := newDiagramFixture(t)
	builder := fixture.diagram.Begin()
	candidate, ok := builder.Set(fixture.diagram.Empty(), factorFirst, 1, fixture.trueAtOne, fixture.values[0])
	if !ok {
		t.Fatal("candidate write failed")
	}
	// A candidate is inspectable only by its owning Builder.  The immutable
	// Diagram boundary must never make a half-built fact visible as State.
	if _, _, valid := fixture.diagram.At(candidate, factorFirst, 1, valuation(true, false)); valid {
		t.Fatal("candidate fact escaped through immutable Diagram reader")
	}
	if _, present, valid := builder.Get(candidate, factorFirst, 1); !valid || !present {
		t.Fatal("owning Builder could not inspect its candidate fact")
	}
	builder.Discard()
	if _, _, valid := builder.Get(candidate, factorFirst, 1); valid {
		t.Fatal("discarded candidate remained readable")
	}
}

func TestPersistentNoOpsPreserveMeaningWithoutPublishingCandidate(t *testing.T) {
	fixture := newDiagramFixture(t)
	seed := fixture.diagram.Begin()
	base, ok := seed.Set(fixture.diagram.Empty(), factorFirst, 4, fixture.trueAtOne, fixture.values[0])
	if !ok {
		t.Fatal("seed write failed")
	}
	base, ok = seed.Seal(base)
	if !ok {
		t.Fatal("seed seal failed")
	}

	builder := fixture.diagram.Begin()
	if _, ok := builder.Constant(terminal.ID[uint8]{}); !ok {
		t.Fatal("candidate undefined terminal creation failed")
	}
	if _, ok := builder.Constant(fixture.values[0]); !ok {
		t.Fatal("candidate terminal creation failed")
	}
	setNoop, ok := builder.Set(base, factorFirst, 4, fixture.trueAtOne, fixture.values[0])
	if !ok {
		t.Fatal("exact no-op Set failed")
	}
	if fixture.diagram.Valid(setNoop) {
		t.Fatal("shared no-op candidate escaped before seal")
	}
	deleteNoop, ok := builder.Delete(setNoop, factorFirst, 4, fixture.falseAtOne)
	if !ok {
		t.Fatal("exact no-op Delete failed")
	}
	value, present, valid := builder.Get(deleteNoop, factorFirst, 4)
	if !valid || !present {
		t.Fatal("seed value unavailable to transform")
	}
	weakNoop, ok := builder.Transform(value, fixture.trueAtOne, func(value terminal.ID[uint8]) (terminal.ID[uint8], bool) {
		return value, true
	})
	if !ok {
		t.Fatal("identity transform failed")
	}
	transformNoop, ok := builder.Put(deleteNoop, factorFirst, 4, weakNoop)
	if !ok {
		t.Fatal("weak-style no-op transform failed")
	}
	published, ok := builder.Seal(transformNoop)
	if !ok || !fixture.diagram.Valid(published) || !fixture.diagram.Equal(published, base) {
		t.Fatal("no-op sequence did not publish the original fact relation")
	}
}

// balanceKey and balanceFactor are rebalancing operators, not copiers. Every
// caller hands them a node it has just constructed from the children it wants,
// so an already-balanced node is republished as it stands. Copying it would
// allocate a second identical node on the persistent tree's hottest write path,
// and the AVL rotations below still publish the new roots they compute.
func TestBalanceRepublishesTheBalancedNodeItReceives(t *testing.T) {
	left := makeKey[testKey, uint8](1, nil, nil, nil)
	right := makeKey[testKey, uint8](3, nil, nil, nil)
	balanced := makeKey[testKey, uint8](2, nil, left, right)
	if got := balanceKey(balanced); got != balanced {
		t.Fatal("balanceKey copied a key node it had no rotation to perform")
	}
	if got := balanceKey(left); got != left {
		t.Fatal("balanceKey copied a leaf key node")
	}
	heavy := makeKey[testKey, uint8](2, nil, makeKey[testKey, uint8](1, nil, makeKey[testKey, uint8](0, nil, nil, nil), nil), nil)
	rotated := balanceKey(heavy)
	if rotated == heavy || rotated.key != 1 || keyHeight(rotated) != 2 || rotated.left == nil || rotated.right == nil {
		t.Fatalf("left-heavy balanceKey = key %d height %d, want the rotated root", rotated.key, keyHeight(rotated))
	}

	firstFactor := makeFactor[testFactor, testKey, uint8](factorFirst, 0, nil, nil, nil)
	secondFactor := makeFactor[testFactor, testKey, uint8](factorSecond, 2, nil, nil, nil)
	balancedFactor := makeFactor[testFactor, testKey, uint8](factorFirst, 1, nil, firstFactor, secondFactor)
	if got := balanceFactor(balancedFactor); got != balancedFactor {
		t.Fatal("balanceFactor copied a factor node it had no rotation to perform")
	}
	heavyFactor := makeFactor[testFactor, testKey, uint8](factorFirst, 2, nil,
		makeFactor[testFactor, testKey, uint8](factorFirst, 1, nil, makeFactor[testFactor, testKey, uint8](factorFirst, 0, nil, nil, nil), nil), nil)
	rotatedFactor := balanceFactor(heavyFactor)
	if rotatedFactor == heavyFactor || rotatedFactor.rank != 1 || factorHeight(rotatedFactor) != 2 {
		t.Fatalf("left-heavy balanceFactor = rank %d height %d, want the rotated root", rotatedFactor.rank, factorHeight(rotatedFactor))
	}
}

// PartitionValueTerminals refines a region by one FDD value, and that
// refinement is Boolean construction: it needs a support transaction. The
// transaction is a cost of the read, not of the value, so the caller lends the
// shell. The engine's one-key read runs millions of times against the same
// Diagram and must not mint a private shell per read.
//
// Lending changes no identity. Each borrow opens a fresh candidate whose Seal
// publishes exactly the cells that one read constructed, so a lent read reports
// the same partition as an unlent one and every lent read is repeatable.
func TestPartitionValueTerminalsRefinesThroughALentSupportShell(t *testing.T) {
	fixture := newDiagramFixture(t)
	builder := fixture.diagram.Begin()
	root, ok := builder.Set(fixture.diagram.Empty(), factorFirst, 1, fixture.trueAtOne, fixture.values[0])
	if !ok {
		t.Fatal("first branch write")
	}
	root, ok = builder.Set(root, factorFirst, 1, fixture.trueAtTwo, fixture.values[1])
	if !ok {
		t.Fatal("second branch write")
	}
	root, ok = builder.Seal(root)
	if !ok {
		t.Fatal("branched root seal")
	}
	value, present, valid := fixture.diagram.Get(root, factorFirst, 1)
	if !valid || !present {
		t.Fatal("branched value unavailable")
	}
	whole, ok := support.True(fixture.manager)
	if !ok {
		t.Fatal("whole support")
	}

	type cell struct {
		id     terminal.ID[uint8]
		region support.Mask
	}
	read := func(scratch *support.Work) []cell {
		var cells []cell
		completed, valid := fixture.diagram.PartitionValueTerminals(value, whole, scratch, func(id terminal.ID[uint8], region support.Mask) bool {
			cells = append(cells, cell{id: id, region: region})
			return true
		})
		if !completed || !valid {
			t.Fatalf("partition = completed:%t valid:%t", completed, valid)
		}
		return cells
	}

	unlent := read(nil)
	if len(unlent) < 2 {
		t.Fatalf("branched partition emitted %d cells, want a refined partition", len(unlent))
	}
	lent := support.New(fixture.manager)
	if lent == nil {
		t.Fatal("lent shell")
	}
	// A lent shell serves consecutive reads: the second borrow reopens it after
	// the first read's Seal, and publishes the same partition again.
	for pass := 0; pass < 3; pass++ {
		borrowed := read(lent)
		if len(borrowed) != len(unlent) {
			t.Fatalf("pass %d emitted %d cells, the unlent read emitted %d", pass, len(borrowed), len(unlent))
		}
		for index, got := range borrowed {
			want := unlent[index]
			if got.id != want.id || !got.region.Valid() || !got.region.Entails(want.region) || !want.region.Entails(got.region) {
				t.Fatalf("pass %d cell %d = %v, want the unlent cell %v", pass, index, got.id, want.id)
			}
		}
	}

	// The lent shell is the read's whole Boolean cost: with it, a warm read
	// constructs no transaction of its own.
	unlentAllocations := testing.AllocsPerRun(20, func() { read(nil) })
	lentAllocations := testing.AllocsPerRun(20, func() { read(lent) })
	if lentAllocations >= unlentAllocations {
		t.Fatalf("lent read allocated %.0f times, unlent read allocated %.0f", lentAllocations, unlentAllocations)
	}
}

func TestFoldValueUnderVisitsReachableTerminals(t *testing.T) {
	fixture := newDiagramFixture(t)
	builder := fixture.diagram.Begin()
	root, ok := builder.Set(fixture.diagram.Empty(), factorFirst, 1, fixture.trueAtOne, fixture.values[0])
	if !ok {
		t.Fatal("first branch write")
	}
	root, ok = builder.Set(root, factorFirst, 1, fixture.falseAtOne, fixture.values[1])
	if !ok {
		t.Fatal("second branch write")
	}
	root, ok = builder.Seal(root)
	if !ok {
		t.Fatal("root seal")
	}
	value, present, valid := fixture.diagram.Get(root, factorFirst, 1)
	if !valid || !present {
		t.Fatal("value lookup")
	}
	whole, ok := support.True(fixture.manager)
	if !ok {
		t.Fatal("whole support")
	}

	var folded []terminal.ID[uint8]
	completed := fixture.diagram.FoldValueUnder(value, whole, NewSoleScratch[testKey, uint8](), func(id terminal.ID[uint8]) bool {
		folded = append(folded, id)
		return true
	})
	if !completed || len(folded) != 2 {
		t.Fatalf("direct fold = completed:%t ids:%d, want 2", completed, len(folded))
	}
	seen := map[terminal.ID[uint8]]bool{}
	for _, id := range folded {
		seen[id] = true
	}
	if !seen[fixture.values[0]] || !seen[fixture.values[1]] {
		t.Fatalf("direct fold terminals = %v, want %v and %v", folded, fixture.values[0], fixture.values[1])
	}

	// The same terminal can be reached through two structural routes. The
	// direct walk is allowed to visit that shared suffix once because the
	// semantic consumer's Join is idempotent; it must still retain every
	// distinct terminal reachable from the support.
	branches := support.New(fixture.manager)
	if branches == nil {
		t.Fatal("shared-suffix support work")
	}
	// Use the second guard only on the high branch so both FDD routes share
	// the value-10 terminal while the final high leaf carries value-30.
	onOne, ok := branches.Literal(1, true)
	if !ok {
		t.Fatal("shared-suffix first literal")
	}
	onTwo, ok := branches.Literal(2, true)
	if !ok {
		t.Fatal("shared-suffix second literal")
	}
	offTwo, ok := branches.Literal(2, false)
	if !ok {
		t.Fatal("shared-suffix second complement")
	}
	sharedLow, ok := branches.And(onOne, offTwo)
	if !ok {
		t.Fatal("shared-suffix low branch")
	}
	sharedHigh, ok := branches.And(onOne, onTwo)
	if !ok || !branches.Seal() {
		t.Fatal("shared-suffix support seal")
	}
	sharedBuilder := fixture.diagram.Begin()
	sharedRoot, ok := sharedBuilder.Set(fixture.diagram.Empty(), factorFirst, 2, fixture.falseAtOne, fixture.values[0])
	if !ok {
		t.Fatal("shared-suffix outer low")
	}
	sharedRoot, ok = sharedBuilder.Set(sharedRoot, factorFirst, 2, sharedLow, fixture.values[0])
	if !ok {
		t.Fatal("shared-suffix inner low")
	}
	sharedRoot, ok = sharedBuilder.Set(sharedRoot, factorFirst, 2, sharedHigh, fixture.values[2])
	if !ok {
		t.Fatal("shared-suffix inner high")
	}
	sharedRoot, ok = sharedBuilder.Seal(sharedRoot)
	if !ok {
		t.Fatal("shared-suffix root seal")
	}
	sharedValue, present, valid := fixture.diagram.Get(sharedRoot, factorFirst, 2)
	if !valid || !present {
		t.Fatal("shared-suffix value lookup")
	}
	var sharedFolded []terminal.ID[uint8]
	if !fixture.diagram.FoldValueUnder(sharedValue, whole, NewSoleScratch[testKey, uint8](), func(id terminal.ID[uint8]) bool {
		sharedFolded = append(sharedFolded, id)
		return true
	}) || len(sharedFolded) != 2 || sharedFolded[0] != fixture.values[0] || sharedFolded[1] != fixture.values[2] {
		t.Fatalf("shared-suffix fold = %v, want [%v %v]", sharedFolded, fixture.values[0], fixture.values[2])
	}

	// A constant undefined terminal is still a reachable terminal, while an
	// empty support has no reachable terminal at all.
	constantBuilder := fixture.diagram.Begin()
	constant, ok := constantBuilder.Constant(terminal.ID[uint8]{})
	if !ok {
		t.Fatal("undefined constant")
	}
	undefined := 0
	if !fixture.diagram.FoldValueUnder(constant, whole, NewSoleScratch[testKey, uint8](), func(id terminal.ID[uint8]) bool {
		if id != (terminal.ID[uint8]{}) {
			t.Fatal("undefined constant changed terminal")
		}
		undefined++
		return true
	}) || undefined != 1 {
		t.Fatalf("undefined fold callbacks = %d", undefined)
	}
	constantBuilder.Discard()
	emptyWork := support.New(fixture.manager)
	if emptyWork == nil {
		t.Fatal("empty support work")
	}
	empty := emptyWork.False()
	if !emptyWork.Seal() {
		t.Fatal("empty support seal")
	}
	callbacks := 0
	if !fixture.diagram.FoldValueUnder(value, empty, NewSoleScratch[testKey, uint8](), func(terminal.ID[uint8]) bool {
		callbacks++
		return true
	}) || callbacks != 0 {
		t.Fatalf("empty fold callbacks = %d", callbacks)
	}

	// Cancellation is observed before the first callback and does not turn a
	// partial traversal into a successful read.
	callbacks = 0
	if fixture.diagram.FoldValueUnder(value, whole, NewSoleScratch[testKey, uint8](), func(terminal.ID[uint8]) bool {
		callbacks++
		return false
	}) || callbacks != 1 {
		t.Fatalf("cancelled fold = callbacks:%d", callbacks)
	}

	foreign, ok := New(Config[testFactor, testKey, uint8]{Factors: []testFactor{factorFirst}, Terminals: fixture.diagram.Terminals(), Guards: fixture.manager})
	if !ok {
		t.Fatal("foreign diagram")
	}
	foreignBuilder := foreign.Begin()
	foreignValue, ok := foreignBuilder.Constant(fixture.values[0])
	if !ok {
		t.Fatal("foreign value")
	}
	if fixture.diagram.FoldValueUnder(foreignValue, whole, NewSoleScratch[testKey, uint8](), func(terminal.ID[uint8]) bool { return true }) {
		t.Fatal("foreign diagram value was accepted")
	}
	foreignBuilder.Discard()
	otherManager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	otherWhole, ok := support.True(otherManager)
	if !ok {
		t.Fatal("foreign support")
	}
	if fixture.diagram.FoldValueUnder(value, otherWhole, NewSoleScratch[testKey, uint8](), func(terminal.ID[uint8]) bool { return true }) {
		t.Fatal("foreign support was accepted")
	}
}
