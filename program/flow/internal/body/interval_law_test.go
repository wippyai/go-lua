package body

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestBodyContainsSiblingIntervals(t *testing.T) {
	root := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	left := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	right := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	input := authoredInputForIntervals(3)
	view, staticView, finish, staticFinish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{{left, right}, nil, nil}, input)
	defer finish.Abort()
	defer staticFinish.Abort()
	defer sourceFinish.Abort()

	result, err := Seal(preimage, view, staticView, root)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if !result.Contains(root, root) || !result.AncestorOrSelf(root, left) || !result.Contains(root, right) {
		t.Fatal("root interval does not contain itself and both children")
	}
	if result.Contains(left, right) || result.Contains(right, left) {
		t.Fatal("sibling Body intervals overlap")
	}
	if result.Contains(left, root) || result.AncestorOrSelf(right, left) {
		t.Fatal("Body interval direction is reversed")
	}
}

func TestBodyAncestryQueriesFailClosed(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	var nilResult *Result
	if nilResult.Contains(body1, body1) || nilResult.AncestorOrSelf(body1, body1) {
		t.Fatal("nil Body Result returned positive ancestry")
	}
	cases := []*Result{
		{parents: []keyspace.Term{0, body1, body1}, pre: []uint32{0, 1}, post: []uint32{0, 4, 0}},
		{parents: []keyspace.Term{0, body1, body1}, pre: []uint32{0, 1, 2}, post: []uint32{0, 0, 4}},
		{parents: []keyspace.Term{0, body1, body1}, pre: []uint32{0, 3, 2}, post: []uint32{0, 4, 1}},
		{parents: []keyspace.Term{0, body1, body2}, pre: []uint32{0, 1, 2}, post: []uint32{0, 4, 5}},
	}
	for index, result := range cases {
		if result.Contains(body1, body2) || result.AncestorOrSelf(body1, body2) {
			t.Fatalf("malformed Result %d returned positive ancestry", index)
		}
	}
}

func TestBodyAncestryQueriesDoNotAllocate(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	input := authoredInputForIntervals(1)
	view, staticView, finish, staticFinish, preimage, sourceFinish := prepare(t, [][]keyspace.Term{nil}, input)
	defer finish.Abort()
	defer staticFinish.Abort()
	defer sourceFinish.Abort()
	result, err := Seal(preimage, view, staticView, body)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	var contains, ancestor bool
	allocs := testing.AllocsPerRun(100, func() {
		contains = result.Contains(body, body)
		ancestor = result.AncestorOrSelf(body, body)
	})
	if allocs != 0 || !contains || !ancestor {
		t.Fatalf("Body ancestry queries allocated %f times or returned false", allocs)
	}
}

func authoredInputForIntervals(bodyCount uint32) authored.Input {
	return authored.Input{Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyBody: bodyCount}}
}
