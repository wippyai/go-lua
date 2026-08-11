package staticcheck

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func TestStaticCheckContextIntervalsDeepAndWideIterative(t *testing.T) {
	const size = 12000
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	deep := &contextTree{points: make([]contextPoint, size), cellPoint: []int{0, 1}}
	for index := 1; index < size; index++ {
		deep.points[index] = contextPoint{body: body, gap: index, outer: index - 1}
	}
	if err := contextIntervals(deep); err != nil {
		t.Fatalf("deep contextIntervals: %v", err)
	}
	if !deep.cellVisible(size-1, keyspace.MakeTerm(keyspace.FamilyCell, 1)) {
		t.Fatal("deep lexical Cell is not visible")
	}

	wide := &contextTree{points: make([]contextPoint, size), cellPoint: []int{0, 1}}
	for index := 1; index < size; index++ {
		wide.points[index] = contextPoint{body: body, gap: index, outer: 0}
	}
	if err := contextIntervals(wide); err != nil {
		t.Fatalf("wide contextIntervals: %v", err)
	}
	if !wide.cellVisible(1, keyspace.MakeTerm(keyspace.FamilyCell, 1)) || wide.cellVisible(size-1, keyspace.MakeTerm(keyspace.FamilyCell, 1)) {
		t.Fatal("wide sibling lexical Cell leaked across gaps")
	}
}
