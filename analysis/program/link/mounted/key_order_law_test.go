package mounted

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func orderLawID(label string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte("mounted-order-law/" + label)))
}

// TestExecutionPointOrderIsTotalAndMountMajor proves the key order every census
// in this package is frozen under: it is antisymmetric, transitive, decided by
// the mount identity before the point identity, and a function of the key bytes
// alone.
func TestExecutionPointOrderIsTotalAndMountMajor(t *testing.T) {
	low, high := orderLawID("low"), orderLawID("high")
	if CompareExecutionPoint(ExecutionPoint{Mount: low, Point: high}, ExecutionPoint{Mount: high, Point: low}) >= 0 {
		t.Fatal("mount identity does not decide the order before the point identity")
	}
	keys := []ExecutionPoint{
		{Mount: low, Point: low},
		{Mount: low, Point: high},
		{Mount: high, Point: low},
		{Mount: high, Point: high},
	}
	for leftIndex, left := range keys {
		if CompareExecutionPoint(left, left) != 0 {
			t.Fatalf("key %d does not compare equal to itself", leftIndex)
		}
		for rightIndex, right := range keys {
			forward, backward := CompareExecutionPoint(left, right), CompareExecutionPoint(right, left)
			if forward != -backward {
				t.Fatalf("keys %d and %d compare %d forward and %d backward", leftIndex, rightIndex, forward, backward)
			}
			if (forward == 0) != (leftIndex == rightIndex) {
				t.Fatalf("keys %d and %d compare equal without being the same key", leftIndex, rightIndex)
			}
		}
	}
	for index := 1; index < len(keys); index++ {
		if CompareExecutionPoint(keys[index-1], keys[index]) >= 0 {
			t.Fatalf("key %d does not follow its predecessor", index)
		}
	}
}

// TestExecutionRootOrderResolvesBodyBeforeEntry proves the root key order: the
// mount decides first, then the body that roots execution, then the entry
// attachment inside it.
func TestExecutionRootOrderResolvesBodyBeforeEntry(t *testing.T) {
	mount, low, high := orderLawID("mount"), orderLawID("low"), orderLawID("high")
	roots := []ExecutionRoot{
		{Mount: mount, Body: low, Entry: low},
		{Mount: mount, Body: low, Entry: high},
		{Mount: mount, Body: high, Entry: low},
	}
	for index := 1; index < len(roots); index++ {
		if CompareExecutionRoot(roots[index-1], roots[index]) >= 0 {
			t.Fatalf("root %d does not follow its predecessor", index)
		}
	}
	if CompareExecutionRoot(roots[0], roots[0]) != 0 {
		t.Fatal("a root does not compare equal to itself")
	}
}

// TestUnsealedPopulationsAnswerNothing proves the closed default: a population
// that was never sealed reports no members and addresses no row, so a caller
// cannot read an empty column as a total one.
func TestUnsealedPopulationsAnswerNothing(t *testing.T) {
	var points ExecutionPoints
	if points.Available() || points.Count() != 0 || points.Contains(ExecutionPoint{Mount: orderLawID("mount"), Point: orderLawID("point")}) {
		t.Fatal("an unsealed execution-point denominator answers membership")
	}
	if _, ok := points.At(0); ok {
		t.Fatal("an unsealed execution-point denominator addresses a row")
	}
	var roots ExecutionRoots
	if roots.Available() || roots.Count() != 0 || roots.Seeds().Available() {
		t.Fatal("an unsealed execution-root set answers membership")
	}
	var census ObservationSites
	if census.Available() || census.Count() != 0 {
		t.Fatal("an unsealed observation census answers membership")
	}
	if _, ok := census.At(0); ok {
		t.Fatal("an unsealed observation census addresses a row")
	}
}

// TestSealRejectsIncompleteMountInput proves the shared admission: no mount, an
// unavailable mount, or one module identity placed twice leaves every
// population unavailable rather than published over a partial input.
func TestSealRejectsIncompleteMountInput(t *testing.T) {
	if mountsAvailable(nil) {
		t.Fatal("an empty mount set is admitted")
	}
	if mountsAvailable([]Mount{{ModuleKey: orderLawID("mount")}}) {
		t.Fatal("a mount with no artifact is admitted")
	}
	if _, ok := SealExecutionPoints(nil); ok {
		t.Fatal("the denominator seals over an empty mount set")
	}
	if _, ok := SealExecutionRoots(nil); ok {
		t.Fatal("the root set seals over an empty mount set")
	}
	if _, ok := SealObservationSites(nil, nil); ok {
		t.Fatal("the observation census seals over an empty mount set")
	}
}
