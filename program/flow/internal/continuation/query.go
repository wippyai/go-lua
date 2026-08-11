package continuation

import "github.com/wippyai/go-lua/program/keyspace"

// CellCount reports the exact lexical Cell count for one admitted subject.
// The boolean distinguishes a live subject with an empty environment from a
// non-subject, foreign, malformed, or unavailable Term.  Admission and the
// count come from the Seal-time root record; this query never rescans the
// retained scope store.
func (result *Result) CellCount(subject keyspace.Term) (int, bool) {
	_, count, ok := result.cellRoot(subject)
	if !ok {
		return 0, false
	}
	if uint64(count) > uint64(^uint(0)>>1) {
		return 0, false
	}
	return int(count), true
}

// CellAt returns one lexical Cell in canonical inner-to-outer scope order.
// It allocates nothing and never materializes a subject-by-Cell plane.
func (result *Result) CellAt(subject keyspace.Term, index int) (keyspace.Term, bool) {
	if index < 0 {
		return 0, false
	}
	if uint64(index) > uint64(^uint32(0)) {
		return 0, false
	}
	root, count, ok := result.cellRoot(subject)
	if !ok {
		return 0, false
	}
	return result.cells.at(root, count, uint32(index))
}

// GuardCount reports the exact unpolarized reaching Guard count for one
// admitted continuation subject or Causal Successor endpoint. Empty support
// is a valid result and returns (0,true). The Seal-time root record supplies
// the exact count without chain-wide validation during the query.
func (result *Result) GuardCount(subject keyspace.Term) (int, bool) {
	_, count, ok := result.guardRoot(subject)
	if !ok {
		return 0, false
	}
	if uint64(count) > uint64(^uint(0)>>1) {
		return 0, false
	}
	return int(count), true
}

// GuardAt returns one existing Select, Branch, or Loop Term in canonical
// numeric Term order for an admitted continuation subject or Causal Successor
// endpoint. Polarity is deliberately not retained in this owner.
func (result *Result) GuardAt(subject keyspace.Term, index int) (keyspace.Term, bool) {
	if index < 0 {
		return 0, false
	}
	if uint64(index) > uint64(^uint32(0)) {
		return 0, false
	}
	root, count, ok := result.guardRoot(subject)
	if !ok {
		return 0, false
	}
	return result.guards.at(root, count, uint32(index))
}

func (result *Result) cellRoot(subject keyspace.Term) (uint32, uint32, bool) {
	if !result.available() || len(result.cells.nodes) == 0 || result.cells.nodes[0] != (scopeNode{}) {
		return 0, 0, false
	}
	family, ordinal := keyspace.TermFamily(subject), keyspace.TermOrdinal(subject)
	if !subjectFamily(family) || ordinal == 0 {
		return 0, 0, false
	}
	if ordinal > result.cells.counts[family] {
		return 0, 0, false
	}
	plane := result.cells.roots[family]
	records := result.cells.records[family]
	if uint64(len(plane)) != uint64(result.cells.counts[family])+1 || len(records) != len(plane) || uint64(ordinal) >= uint64(len(plane)) {
		return 0, 0, false
	}
	record := records[ordinal]
	if !record.present || plane[ordinal] != record.root || record.root == absentRoot || uint64(record.root) >= uint64(len(result.cells.nodes)) || record.node.total != record.count || result.cells.nodes[record.root] != record.node {
		return 0, 0, false
	}
	return record.root, record.count, true
}

func (result *Result) guardRoot(subject keyspace.Term) (uint32, uint32, bool) {
	if !result.available() || len(result.guards.nodes) == 0 || result.guards.nodes[0] != (guardNode{}) {
		return 0, 0, false
	}
	family, ordinal := keyspace.TermFamily(subject), keyspace.TermOrdinal(subject)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount ||
		(!subjectFamily(family) && !result.guards.families[family]) || ordinal == 0 {
		return 0, 0, false
	}
	if ordinal > result.guards.counts[family] {
		return 0, 0, false
	}
	plane := result.guards.roots[family]
	records := result.guards.records[family]
	if uint64(len(plane)) != uint64(result.guards.counts[family])+1 || len(records) != len(plane) || uint64(ordinal) >= uint64(len(plane)) {
		return 0, 0, false
	}
	record := records[ordinal]
	if !record.present || plane[ordinal] != record.root || record.root == absentRoot || uint64(record.root) >= uint64(len(result.guards.nodes)) ||
		result.guards.nodes[record.root] != record.node || record.node.count != record.count {
		return 0, 0, false
	}
	return record.root, record.count, true
}

func subjectFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyUnary, keyspace.FamilyBinary, keyspace.FamilyRead,
		keyspace.FamilyWrite, keyspace.FamilyCall, keyspace.FamilyLoop:
		return true
	default:
		return false
	}
}
