package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestEnvironmentRouteIndexDistinguishesAbsentUniqueAndAmbiguous(t *testing.T) {
	first, repeated, independent := valuesLawID(1), valuesLawID(2), valuesLawID(3)
	index := make(map[identity.ContentID]environmentRouteIndex)
	if _, found := index[first]; found {
		t.Fatal("absent route unexpectedly had an index")
	}
	if !recordEnvironmentRoute(index, first, 0, 1) {
		t.Fatal("unique route was not recorded")
	}
	if ordinal, unique := index[first].uniqueAt(1); !unique || ordinal != 0 {
		t.Fatalf("unique route lookup = %d/%v, want 0/true", ordinal, unique)
	}
	if !recordEnvironmentRoute(index, first, 1, 2) || !recordEnvironmentRoute(index, first, 2, 3) {
		t.Fatal("repeated route was not recorded")
	}
	if _, unique := index[first].uniqueAt(3); unique {
		t.Fatal("ambiguous route was returned as a unique predecessor")
	}
	if ordinal, representative := index[first].representativeAt(3); !representative || ordinal != 0 {
		t.Fatalf("ambiguous representative = %d/%v, want first ordinal 0/true", ordinal, representative)
	}
	if !recordEnvironmentRoute(index, repeated, 3, 4) || !recordEnvironmentRoute(index, independent, 4, 5) {
		t.Fatal("independent routes were not recorded")
	}
	if ordinal, unique := index[independent].uniqueAt(5); !unique || ordinal != 4 {
		t.Fatalf("independent route lookup = %d/%v, want 4/true", ordinal, unique)
	}
}

func TestEnvironmentRouteIndexRejectsMalformedOrdinalAndFailsClosed(t *testing.T) {
	route := valuesLawID(10)
	index := make(map[identity.ContentID]environmentRouteIndex)
	if recordEnvironmentRoute(index, route, -1, 1) || recordEnvironmentRoute(index, route, 1, 1) {
		t.Fatal("malformed ordinal was recorded")
	}
	if _, found := index[route]; found {
		t.Fatal("failed admission left a route entry")
	}
	index[route] = environmentRouteIndex{state: environmentRouteUnique, representative: 7}
	if _, unique := index[route].uniqueAt(2); unique {
		t.Fatal("out-of-range representative was accepted")
	}
	if recordEnvironmentRoute(index, route, 1, 2) {
		t.Fatal("duplicate over malformed representative was accepted")
	}
	if _, representative := index[route].representativeAt(2); representative {
		t.Fatal("malformed route index did not become invalid")
	}
	index[route] = environmentRouteIndex{state: environmentRouteInvalid, representative: 0}
	if _, representative := index[route].representativeAt(1); representative {
		t.Fatal("invalid state with a plausible ordinal was accepted")
	}
}
