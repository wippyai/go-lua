package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestPendingStoreFixedWidthSetOrderAndSharing(t *testing.T) {
	store := newPendingTermStore()
	first, err := store.insert(0, keyspace.MakeTerm(keyspace.FamilyCall, 2))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.insert(first, keyspace.MakeTerm(keyspace.FamilyCall, 1))
	if err != nil {
		t.Fatal(err)
	}
	third, err := store.insert(second, keyspace.MakeTerm(keyspace.FamilyCall, 3))
	if err != nil {
		t.Fatal(err)
	}
	if store.nodes[first].count != 1 || store.nodes[second].count != 2 || store.nodes[third].count != 3 {
		t.Fatalf("persistent counts = %d,%d,%d", store.nodes[first].count, store.nodes[second].count, store.nodes[third].count)
	}
	if got, want := len(store.nodes), 1+3*33; got != want {
		t.Fatalf("fixed-width node growth = %d, want exactly %d", got, want)
	}
	// Call2 and Call1 diverge at the first differing ordinal bit. The
	// untouched sibling subtree at that divergence must retain its exact
	// append-store index across the path copies, rather than merely producing a
	// different root header.
	branchAt := func(root uint32, term keyspace.Term, bit uint8) pendingNode {
		for {
			node := store.nodes[root]
			if node.bit == bit {
				return node
			}
			if pendingTermBit(term, node.bit) == 0 {
				root = node.left
			} else {
				root = node.right
			}
		}
	}
	firstBitNine := branchAt(first, keyspace.MakeTerm(keyspace.FamilyCall, 2), 9)
	secondBitNine := branchAt(second, keyspace.MakeTerm(keyspace.FamilyCall, 1), 9)
	if firstBitNine.right == 0 || firstBitNine.right != secondBitNine.right {
		t.Fatalf("second insertion failed to preserve unaffected bit-9 subtree index: first=%d second=%d", firstBitNine.right, secondBitNine.right)
	}
	secondBitNine = branchAt(second, keyspace.MakeTerm(keyspace.FamilyCall, 1), 9)
	thirdBitNine := branchAt(third, keyspace.MakeTerm(keyspace.FamilyCall, 3), 9)
	if secondBitNine.left == 0 || secondBitNine.left != thirdBitNine.left {
		t.Fatalf("third insertion failed to preserve unaffected bit-9 subtree index: second=%d third=%d", secondBitNine.left, thirdBitNine.left)
	}
	if _, ok := pendingTermAt(store.nodes, first, 1); ok {
		t.Fatal("single-term root exposed an out-of-range rank")
	}
	want := []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyCall, 1),
		keyspace.MakeTerm(keyspace.FamilyCall, 2),
		keyspace.MakeTerm(keyspace.FamilyCall, 3),
	}
	for index, term := range want {
		got, ok := pendingTermAt(store.nodes, third, uint32(index))
		if !ok || got != term {
			t.Fatalf("At(%d) = %v/%v, want %v", index, got, ok, term)
		}
	}
	if store.nodes[second].left == store.nodes[third].left && store.nodes[second].right == store.nodes[third].right {
		t.Fatal("insertion did not path-copy a changed branch")
	}
}

func TestPendingStoreInsertRejectsMalformedSentinelWithoutMutation(t *testing.T) {
	term := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	store := &pendingTermStore{nodes: []pendingNode{{count: 1}}}
	before := append([]pendingNode(nil), store.nodes...)
	if _, err := store.insert(0, term); err == nil {
		t.Fatal("insertion accepted a malformed empty-root sentinel")
	}
	if len(store.nodes) != len(before) || store.nodes[0] != before[0] {
		t.Fatalf("malformed insertion mutated node storage: got %#v, want %#v", store.nodes, before)
	}
}

func TestPendingStoreInsertRejectsPartialRootWithoutMutation(t *testing.T) {
	store := newPendingTermStore()
	root, err := store.insert(0, keyspace.MakeTerm(keyspace.FamilyCall, 1))
	if err != nil {
		t.Fatalf("canonical insertion: %v", err)
	}
	partial := store.nodes[root].left
	if partial == 0 {
		partial = store.nodes[root].right
	}
	if partial == 0 || store.nodes[partial].bit != pendingRootBit-1 {
		t.Fatalf("canonical root did not expose a bit-30 child: root=%d child=%d", root, partial)
	}
	before := append([]pendingNode(nil), store.nodes...)
	if _, err := store.insert(partial, keyspace.MakeTerm(keyspace.FamilyCall, 2)); err == nil {
		t.Fatal("insertion accepted a partial bit-30 root")
	}
	if len(store.nodes) != len(before) {
		t.Fatalf("partial-root insertion changed node length: got %d, want %d", len(store.nodes), len(before))
	}
	for index := range before {
		if store.nodes[index] != before[index] {
			t.Fatalf("partial-root insertion mutated node %d: got %#v, want %#v", index, store.nodes[index], before[index])
		}
	}
}

func TestPendingParentProofClaimsPayloadAndRejectsCompositeAlias(t *testing.T) {
	builder := &pendingBuilder{}
	unaryIndex, ok := pendingAncestorIndex(keyspace.FamilyUnary)
	if !ok {
		t.Fatal("Unary is not an evaluation ancestor")
	}
	builder.parents[unaryIndex] = make([]keyspace.Term, 2)
	literalIndex, _ := pendingClaimIndex(keyspace.FamilyInteger)
	functionIndex, _ := pendingClaimIndex(keyspace.FamilyFunction)
	builder.claimed[literalIndex] = make([]bool, 2)
	builder.claimed[functionIndex] = make([]bool, 2)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	binary := keyspace.MakeTerm(keyspace.FamilyBinary, 1)
	unary := keyspace.MakeTerm(keyspace.FamilyUnary, 1)
	literal := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	if err := builder.recordEdge(call, literal); err != nil {
		t.Fatalf("literal payload was rejected: %v", err)
	}
	if err := builder.recordEdge(binary, literal); err == nil {
		t.Fatal("reused literal payload was accepted")
	}
	if err := builder.recordEdge(call, function); err != nil {
		t.Fatalf("Function payload was rejected: %v", err)
	}
	if err := builder.recordEdge(binary, function); err == nil {
		t.Fatal("reused Function payload was accepted")
	}
	for _, reference := range []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyCell, 1),
		keyspace.MakeTerm(keyspace.FamilyKey, 1),
		keyspace.MakeTerm(keyspace.FamilyBody, 1),
	} {
		if err := builder.recordEdge(call, reference); err != nil {
			t.Fatalf("first shared reference %v was rejected: %v", reference, err)
		}
		if err := builder.recordEdge(binary, reference); err != nil {
			t.Fatalf("shared reference %v was rejected: %v", reference, err)
		}
	}
	if err := builder.recordEdge(call, unary); err != nil {
		t.Fatalf("first composite occurrence was rejected: %v", err)
	}
	if err := builder.recordEdge(binary, unary); err == nil {
		t.Fatal("composite child with conflicting parents was accepted")
	}
}

func TestPendingCountDistinguishesEmptySubject(t *testing.T) {
	subject := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	pending := &Pending{
		sourceID: identity.ContentID{0: 1},
		flowID:   identity.ContentID{0: 2},
		staticID: identity.ContentID{0: 3},
		moduleID: identity.ContentID{0: 4},
		nodes:    []pendingNode{{}},
		sealed:   true,
	}
	pending.roots[keyspace.FamilyCall] = []uint32{0, 1}
	if !MatchesPending(pending, identity.ContentID{0: 1}, identity.ContentID{0: 2}, identity.ContentID{0: 3}, identity.ContentID{0: 4}) {
		t.Fatal("matching Pending provenance was rejected")
	}
	if MatchesPending(pending, identity.ContentID{0: 1}, identity.ContentID{0: 2}, identity.ContentID{0: 9}, identity.ContentID{0: 4}) ||
		MatchesPending(pending, identity.ContentID{}, identity.ContentID{0: 2}, identity.ContentID{0: 3}, identity.ContentID{0: 4}) {
		t.Fatal("foreign or unavailable Pending provenance was accepted")
	}

	if count, ok := pending.Count(subject); !ok || count != 0 {
		t.Fatalf("empty admitted subject Count = %d/%v, want 0/true", count, ok)
	}
	if count, ok := pending.Count(keyspace.MakeTerm(keyspace.FamilyCall, 2)); ok || count != 0 {
		t.Fatalf("absent subject Count = %d/%v, want 0/false", count, ok)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if count, ok := pending.Count(subject); !ok || count != 0 {
			t.Fatal("allocation-free Count returned an incorrect result")
		}
		if _, ok := pending.At(subject, 0); ok {
			t.Fatal("allocation-free At accepted an empty set")
		}
	}); allocations != 0 {
		t.Fatalf("Pending queries allocated %v objects per run", allocations)
	}
}

func TestPendingStoreWidePrefixRootsRemainCanonical(t *testing.T) {
	const width = 2048
	store := newPendingTermStore()
	root := uint32(0)
	for ordinal := 1; ordinal <= width; ordinal++ {
		var err error
		root, err = store.insert(root, keyspace.MakeTerm(keyspace.FamilyCall, uint32(ordinal)))
		if err != nil {
			t.Fatalf("insert(%d): %v", ordinal, err)
		}
	}
	if store.nodes[root].count != width {
		t.Fatalf("wide root count = %d, want %d", store.nodes[root].count, width)
	}
	for _, index := range []uint32{0, width / 2, width - 1} {
		want := keyspace.MakeTerm(keyspace.FamilyCall, index+1)
		got, ok := pendingTermAt(store.nodes, root, index)
		if !ok || got != want {
			t.Fatalf("wide At(%d) = %v/%v, want %v/true", index, got, ok, want)
		}
	}
}

func TestPendingSealedQueriesRejectForeignLeafAndCompressedBranch(t *testing.T) {
	subject := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	foreign := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	ids := func(nodes []pendingNode, root uint32) *Pending {
		pending := &Pending{
			sourceID: identity.ContentID{0: 1},
			flowID:   identity.ContentID{0: 2},
			staticID: identity.ContentID{0: 3},
			moduleID: identity.ContentID{0: 4},
			nodes:    nodes,
			sealed:   true,
		}
		pending.roots[keyspace.FamilyCall] = []uint32{0, root}
		return pending
	}

	foreignLeaf := ids([]pendingNode{
		{},
		{count: 1, term: foreign, bit: pendingLeafBit},
	}, 2)
	if _, ok := foreignLeaf.Count(subject); ok {
		t.Fatal("sealed query accepted a foreign leaf payload")
	}
	admittedLeaf := ids([]pendingNode{
		{},
		{count: 1, term: subject, bit: pendingLeafBit},
	}, 2)
	if _, ok := admittedLeaf.Count(subject); ok {
		t.Fatal("sealed query accepted a leaf as a nonempty root")
	}

	compressed := ids([]pendingNode{
		{},
		{count: 1, term: subject, bit: pendingLeafBit},
		{left: 1, count: 1, bit: 31},
	}, 3)
	if _, ok := compressed.Count(subject); ok {
		t.Fatal("sealed query accepted a compressed 31-to-leaf branch")
	}
	var roots [keyspace.FamilyCount][]uint32
	roots[keyspace.FamilyCall] = []uint32{0, 3}
	if err := validatePendingStorage(compressed.nodes, roots); err == nil {
		t.Fatal("storage validator accepted a compressed 31-to-leaf branch")
	}

	// Build a genuine fixed-width trie, then expose its bit-30 child as the
	// published root. It is locally valid and used to pass the old root checks,
	// but it is a partial/compressed root and must fail closed everywhere.
	store := newPendingTermStore()
	canonical, err := store.insert(0, subject)
	if err != nil {
		t.Fatalf("build canonical root: %v", err)
	}
	child := store.nodes[canonical].left
	if child == 0 {
		child = store.nodes[canonical].right
	}
	if child == 0 || store.nodes[child].bit != pendingRootBit-1 {
		t.Fatalf("canonical root did not expose a bit-30 child: root=%d child=%d", canonical, child)
	}
	lower := ids(store.nodes, child+1)
	if _, ok := lower.Count(subject); ok {
		t.Fatal("sealed query accepted a bit-30 root")
	}
	if _, ok := pendingRootCount(store.nodes, child); ok {
		t.Fatal("root-count accepted a bit-30 root")
	}
	if _, ok := pendingTermAt(store.nodes, child, 0); ok {
		t.Fatal("At accepted a bit-30 root")
	}
	var lowerRoots [keyspace.FamilyCount][]uint32
	lowerRoots[keyspace.FamilyCall] = []uint32{0, child + 1}
	if err := validatePendingStorage(store.nodes, lowerRoots); err == nil {
		t.Fatal("storage validator accepted a bit-30 root")
	}
	if _, err := store.code(child); err == nil {
		t.Fatal("store code accepted a bit-30 root")
	}
}

func TestPendingStorageRejectsPartitionAndRootOverflow(t *testing.T) {
	left := keyspace.MakeTerm(keyspace.FamilyCall, 1)  // bit zero is one
	right := keyspace.MakeTerm(keyspace.FamilyBool, 1) // bit zero is zero
	nodes := []pendingNode{
		{},
		{count: 1, term: left, bit: pendingLeafBit},
		{count: 1, term: right, bit: pendingLeafBit},
		{left: 1, right: 2, count: 2, bit: 0},
	}
	var roots [keyspace.FamilyCount][]uint32
	roots[keyspace.FamilyCall] = []uint32{0, 4}
	if err := validatePendingStorage(nodes, roots); err == nil {
		t.Fatal("storage validator accepted a child in the wrong branch partition")
	}
	max := ^uint32(0)
	if _, ok := pendingRootCount([]pendingNode{{}}, max); ok {
		t.Fatal("root-count query accepted an overflowing root code")
	}
	if _, ok := pendingTermAt([]pendingNode{{}}, max, 0); ok {
		t.Fatal("At accepted an overflowing root code")
	}
	if _, err := (&pendingTermStore{nodes: []pendingNode{{}}}).code(max); err == nil {
		t.Fatal("store code accepted an overflowing root code")
	}
}
