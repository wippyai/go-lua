package binding

import (
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Result is the immutable dense Cell definition projection. Index zero in
// roles, hosts, and functionCells is reserved for the invalid zero Term;
// published state is limited to those arrays and the optional derived chunk
// Cell. functionCells is the one-way Function-ordinal -> Cell projection used
// by the recursive self-capture proof.
type Result struct {
	// Provenance is the narrow owner fence for this projection. Binding does
	// not retain Source, Flow, or Body owners; the scalar identities are copied
	// once at seal and checked by downstream assembly before use.
	sourceID      keyspace.ContentID
	flowID        keyspace.ContentID
	roles         []kind.CellRole
	hosts         []keyspace.Term
	chunk         keyspace.Term
	functionCells []keyspace.Term
	captures      []captureCellRole
	loops         []loopCellRole
}

// These private inverse sidecars are populated while their authoritative CSR
// rows are sealed.  They let the semantic certificate prove one Cell's
// capture/loop position without reopening or scanning Function/Loop storage.
type captureCellRole struct {
	function keyspace.Term
	outer    keyspace.Term
	position uint32
}

type loopCellRole struct {
	loop     keyspace.Term
	position uint32
	kind     kind.LoopKind
}

// Matches reports whether r was sealed for the exact Source and authored Flow
// identities supplied by the final assembly. Unavailable identities never
// match, including a malformed Result carrying plausible dense projections.
func Matches(r *Result, sourceID, flowID keyspace.ContentID) bool {
	return r != nil && sourceID.Available() && flowID.Available() &&
		r.sourceID == sourceID && r.flowID == flowID
}

func (r Result) available() bool {
	return r.sourceID.Available() && r.flowID.Available()
}
