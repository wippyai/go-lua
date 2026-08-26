package obligation

import (
	"testing"

	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/typestate/statecell"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The site one actual is read at is the site its own row names. Call already
// numbers its occurrences, so the row resolves an existing coordinate; every
// actual of one call resolves to that one coordinate, which is why the rows
// hang off the actual's directory instead of a correspondence.
func TestCallSiteRowIsTheActualsOwnOccurrence(t *testing.T) {
	fixture := buildJudgmentFixture(t, judgmentSource)
	judgment, ok := Derive(fixture.values, fixture.calls, fixture.packs)
	if !ok {
		t.Fatal("judgment not derived")
	}
	if fixture.values.MountedCallArgumentCount() == 0 {
		t.Fatal("the fixture publishes no mounted call actual")
	}
	for index := 0; index < fixture.values.MountedCallArgumentCount(); index++ {
		candidate, candidateOK := fixture.values.MountedCallArgumentAt(index)
		if !candidateOK {
			t.Fatalf("mounted call actual %d unavailable", index)
		}
		site, siteOK := judgment.DeriveCallSite(candidate)
		if !siteOK {
			t.Fatalf("actual %d addresses no call site", index)
		}
		module, moduleOK := candidate.Module()
		call, callOK := candidate.CallID()
		if !moduleOK || !callOK {
			t.Fatalf("actual %d carries no site identity", index)
		}
		expected, expectedOK := fixture.calls.CallCoordinateForOccurrence(module, call)
		if !expectedOK || site != expected {
			t.Fatalf("actual %d resolved a coordinate other than its own occurrence's", index)
		}
		if _, keyOK := site.Key(); !keyOK {
			t.Fatalf("actual %d resolved a site with no read key", index)
		}
	}
}

// An actual another Value schema issued addresses no site here. The row is
// resolved through the owner that sealed the actual, so an equal-shaped row
// from a second seal carries no occurrence this judgment may address.
func TestForeignActualAddressesNoCallSite(t *testing.T) {
	own := buildJudgmentFixture(t, judgmentSource)
	foreign := buildJudgmentFixture(t, "local function g(a) return a end\ng(3)\n")
	judgment, ok := Derive(own.values, own.calls, own.packs)
	if !ok {
		t.Fatal("judgment not derived")
	}
	candidate, candidateOK := foreign.values.MountedCallArgumentAt(0)
	if !candidateOK {
		t.Fatal("the foreign fixture publishes no mounted call actual")
	}
	if _, siteOK := judgment.DeriveCallSite(candidate); siteOK {
		t.Fatal("a foreign actual was given a call site")
	}
}

// The absence of an obligation is not a cell. An actual dispatched to a
// function body declares nothing about any protocol, so the read publishes no
// row rather than a row per protocol the resource could be governed by.
func TestUngovernedActualPublishesNoStateCell(t *testing.T) {
	fixture := buildJudgmentFixture(t, judgmentSource)
	judgment, ok := Derive(fixture.values, fixture.calls, fixture.packs)
	if !ok {
		t.Fatal("judgment not derived")
	}
	candidate, candidateOK := fixture.values.MountedCallArgumentAt(0)
	if !candidateOK {
		t.Fatal("the fixture publishes no mounted call actual")
	}
	space, spaceOK := statecell.Seal(fixture.values.LinkID(), 4, 2)
	if !spaceOK {
		t.Fatal("cell space unavailable")
	}
	plan, planOK := judgment.DeriveStateCells(fixture.values.Heap(), space, candidate,
		fixture.values.Top(), opaqueCallValue(t, fixture.calls))
	if !planOK {
		t.Fatal("the cell plan was refused for an admitted actual")
	}
	if plan.Count() != 0 {
		t.Fatalf("cell rows = %d, want none for an actual no protocol speaks about", plan.Count())
	}
}

// Every published cell is one the axis's own space issued, and both endpoints
// of a row are that one cell: an operation moves a resource's state and does
// not move the resource. The rows exist for the protocols the dispatched
// operation declares an obligation about, and for no others.
func TestPublishedCellsAreIssuedByTheAxisSpace(t *testing.T) {
	fixture := buildJudgmentFixture(t, resourceSource)
	judgment, ok := Derive(fixture.values, fixture.calls, fixture.packs)
	if !ok {
		t.Fatal("judgment not derived")
	}
	space, spaceOK := statecell.Seal(fixture.values.LinkID(), fixture.values.Heap().AllocationKeyCount(), 4)
	if !spaceOK {
		t.Fatal("cell space unavailable")
	}
	published := 0
	for index := 0; index < fixture.values.MountedCallArgumentCount(); index++ {
		candidate, candidateOK := fixture.values.MountedCallArgumentAt(index)
		if !candidateOK {
			t.Fatalf("mounted call actual %d unavailable", index)
		}
		actual, actualOK := candidate.ActualIndex()
		if !actualOK {
			t.Fatalf("mounted call actual %d carries no position", index)
		}
		site, siteOK := judgment.DeriveCallSite(candidate)
		if !siteOK {
			continue
		}
		key, keyOK := site.Key()
		if !keyOK {
			t.Fatalf("actual %d resolved a site with no read key", index)
		}
		for support := 0; support < fixture.calls.SupportCount(key); support++ {
			target, targetOK := fixture.calls.SupportTargetAt(key, support)
			if !targetOK {
				t.Fatalf("actual %d support %d unavailable", index, support)
			}
			operation, kind := fixture.calls.ClassifyTargetOperation(target)
			if kind != calldomain.TargetOperationPresent || len(judgment.sealed.protocolsAt(operation, actual)) == 0 {
				continue
			}
			dispatched, dispatchedOK := fixture.calls.DispatchValue(key, []calldomain.Target{target}, false)
			if !dispatchedOK {
				t.Fatalf("actual %d support %d dispatches no fact", index, support)
			}
			plan, planOK := judgment.DeriveStateCells(fixture.values.Heap(), space, candidate, resourceFact(t, fixture), dispatched)
			if !planOK {
				t.Fatalf("actual %d was refused a cell plan", index)
			}
			for row := 0; row < plan.Count(); row++ {
				cell, cellOK := plan.At(row)
				if !cellOK {
					t.Fatalf("cell row %d unavailable", row)
				}
				published++
				read, destination := cell.Coordinates()
				if read != destination {
					t.Fatalf("cell row %d moves the resource between coordinates", row)
				}
				if !space.Owns(read) {
					t.Fatalf("cell row %d publishes at a coordinate this space did not issue", row)
				}
				tag := cell.Predicate()
				if tag == 0 {
					t.Fatalf("cell row %d carries the tag a selection reserves for no member", row)
				}
				protocol, protocolOK := space.Protocol(read)
				if !protocolOK || uint64(protocol) != tag {
					t.Fatalf("cell row %d is tagged %d and held under protocol %d", row, tag, protocol)
				}
				if _, declaredOK := judgment.sealed.obligationAt(protocol, operation, actual); !declaredOK {
					t.Fatalf("cell row %d reads a protocol the operation declares nothing about", row)
				}
			}
		}
	}
	if published == 0 {
		t.Fatal("no cell row was published for the declared-lifecycle fixture, so this law proves nothing")
	}
}

// An unsealed space issues no coordinate, so the read publishes nothing
// through it rather than minting a cell of its own.
func TestUnsealedSpacePublishesNoCell(t *testing.T) {
	fixture := buildJudgmentFixture(t, judgmentSource)
	judgment, ok := Derive(fixture.values, fixture.calls, fixture.packs)
	if !ok {
		t.Fatal("judgment not derived")
	}
	candidate, candidateOK := fixture.values.MountedCallArgumentAt(0)
	if !candidateOK {
		t.Fatal("the fixture publishes no mounted call actual")
	}
	if _, planOK := judgment.DeriveStateCells(fixture.values.Heap(), statecell.Space{}, candidate,
		fixture.values.Top(), opaqueCallValue(t, fixture.calls)); planOK {
		t.Fatal("an unsealed space published a cell plan")
	}
}

// resourceFact is one Value fact that names an allocation root of the sealed
// program: the fresh result the declared acquisition issued. A cell is keyed
// by the resource a fact reaches, so a law about published cells has to hand
// the read a fact that reaches one.
func resourceFact(t *testing.T, fixture judgmentFixture) valuedomain.Value {
	t.Helper()
	heap := fixture.values.Heap()
	for index := 0; index < heap.AllocationKeyCount(); index++ {
		key, keyOK := heap.AllocationKeyAt(index)
		if !keyOK {
			continue
		}
		for _, role := range materialization.Roles() {
			fact, factOK := fixture.values.FreshResultFact(key, role)
			if factOK {
				return fact
			}
		}
	}
	t.Fatal("the fixture publishes no fresh allocation to key a cell by")
	return valuedomain.Value{}
}
