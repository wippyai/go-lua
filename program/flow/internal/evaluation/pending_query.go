package evaluation

import "github.com/wippyai/go-lua/program/keyspace"

// Pending is the immutable evaluation-owned projection of already-evaluated
// payload Terms at function-style subject boundaries. Roots and Patricia nodes
// are private; callers can only query an existing subject by Term.
type Pending struct {
	sourceID keyspace.ContentID
	flowID   keyspace.ContentID
	staticID keyspace.ContentID
	moduleID keyspace.ContentID
	nodes    []pendingNode
	roots    [keyspace.FamilyCount][]uint32 // one-based root code; zero is absent
	// sealed is established only after the complete retained trie/root
	// invariant is validated. Hot queries trust this private bit and perform
	// only scalar gates plus a bounded trie lookup.
	sealed bool
}

// MatchesPending reports whether pending was sealed for the exact Source and
// authored Flow, Static, and Module identities supplied by the caller.
// Unavailable identities and malformed empty roots fail closed. The four
// scalar fences are intentionally kept in canonical owner order; no composite
// identity or compatibility overload can stand in for one of them.
func MatchesPending(pending *Pending, sourceID, flowID, staticID, moduleID keyspace.ContentID) bool {
	return pending != nil && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		pending.sourceID.Available() && pending.flowID.Available() && pending.staticID.Available() && pending.moduleID.Available() &&
		pending.sealed && pendingStorageSentinelValid(pending.nodes) &&
		pending.sourceID == sourceID && pending.flowID == flowID &&
		pending.staticID == staticID && pending.moduleID == moduleID
}

func pendingStorageSentinelValid(nodes []pendingNode) bool {
	return len(nodes) != 0 && nodes[0] == (pendingNode{})
}

// Count reports the number of retained pending payload Terms for subject. The
// boolean distinguishes a valid admitted subject whose exact set is empty
// from a non-subject, foreign, malformed, or absent subject.
func (pending *Pending) Count(subject keyspace.Term) (int, bool) {
	root, ok := pending.root(subject)
	if !ok {
		return 0, false
	}
	return int(pending.nodes[root].count), true
}

// At returns one retained pending payload Term in canonical packed-Term order.
// It performs no allocation and returns false for an invalid index or subject.
func (pending *Pending) At(subject keyspace.Term, index int) (keyspace.Term, bool) {
	if index < 0 {
		return 0, false
	}
	root, ok := pending.root(subject)
	if !ok {
		return 0, false
	}
	count := pending.nodes[root].count
	if uint64(index) >= uint64(count) {
		return 0, false
	}
	term, ok := pendingTermAt(pending.nodes, root, uint32(index))
	return term, ok && pendingPayloadTerm(term)
}

func (pending *Pending) root(subject keyspace.Term) (uint32, bool) {
	if pending == nil || !pending.sourceID.Available() || !pending.flowID.Available() ||
		!pending.staticID.Available() || !pending.moduleID.Available() || !pending.sealed ||
		!pendingStorageSentinelValid(pending.nodes) {
		return 0, false
	}
	family, ordinal := keyspace.TermFamily(subject), keyspace.TermOrdinal(subject)
	if !pendingSubjectFamily(family) || ordinal == 0 {
		return 0, false
	}
	plane := pending.roots[family]
	if uint64(ordinal) >= uint64(len(plane)) {
		return 0, false
	}
	code := plane[ordinal]
	if code == 0 || uint64(code-1) >= uint64(len(pending.nodes)) {
		return 0, false
	}
	root := code - 1
	if root == 0 {
		return 0, true
	}
	if !pendingCanonicalRootValid(pending.nodes, root) {
		return 0, false
	}
	return root, true
}

func pendingSubjectFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyUnary, keyspace.FamilyBinary, keyspace.FamilyRead,
		keyspace.FamilyWrite, keyspace.FamilyCall, keyspace.FamilyLoop:
		return true
	default:
		return false
	}
}
