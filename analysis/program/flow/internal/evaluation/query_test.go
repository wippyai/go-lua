package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestPendingQueriesRejectMalformedTrieShapes(t *testing.T) {
	subject := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	ids := func(nodes []pendingNode) *Pending {
		pending := &Pending{
			sourceID: identity.ContentID{0: 1},
			flowID:   identity.ContentID{0: 2},
			staticID: identity.ContentID{0: 3},
			moduleID: identity.ContentID{0: 4},
			nodes:    nodes,
		}
		pending.roots[keyspace.FamilyCall] = []uint32{0, 1}
		return pending
	}

	malformedSentinel := ids([]pendingNode{{count: 1}})
	if _, ok := malformedSentinel.Count(subject); ok {
		t.Fatal("malformed empty-root sentinel was accepted")
	}
	malformedLeaf := ids([]pendingNode{{}, {count: 0, term: subject, bit: pendingLeafBit}})
	malformedLeaf.roots[keyspace.FamilyCall][1] = 2
	if _, ok := malformedLeaf.Count(subject); ok {
		t.Fatal("malformed leaf count was accepted")
	}
	malformedBranch := ids([]pendingNode{
		{},
		{count: 1, term: subject, bit: pendingLeafBit},
		{left: 1, count: 2, bit: 0},
	})
	malformedBranch.roots[keyspace.FamilyCall][1] = 3
	if _, ok := malformedBranch.Count(subject); ok {
		t.Fatal("malformed branch count was accepted")
	}
	forward := ids([]pendingNode{{}, {left: 2, count: 1, bit: 0}, {left: 1, count: 1, bit: 0}})
	forward.roots[keyspace.FamilyCall][1] = 2
	if _, ok := forward.Count(subject); ok {
		t.Fatal("forward/cyclic branch was accepted")
	}
	var invalidFamily [keyspace.FamilyCount][]uint32
	invalidFamily[keyspace.FamilyInvalid] = []uint32{1}
	if err := validatePendingStorage([]pendingNode{{}}, invalidFamily); err == nil {
		t.Fatal("invalid-family root plane was accepted")
	}
}
