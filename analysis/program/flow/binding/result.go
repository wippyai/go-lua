package binding

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Result is the immutable dense Cell definition projection. Index zero in
// roles, hosts, slots, and functionCells is reserved for the invalid zero
// Term; published state is limited to those arrays and the optional derived
// chunk Cell. slots carry each Cell's position inside the ordered group its
// definition host introduces, so a consumer that needs the position never
// re-walks the authored Bind, formal, capture, or Loop order. functionCells
// is the one-way Function-ordinal -> Cell projection used by the recursive
// self-capture proof.
type Result struct {
	// Provenance is the narrow owner fence for this projection. Binding does
	// not retain Source, Flow, or Body owners; the scalar identities are copied
	// once at seal and checked by downstream assembly before use.
	sourceID      identity.ContentID
	flowID        identity.ContentID
	roles         []kind.CellRole
	hosts         []keyspace.Term
	slots         []uint32
	chunk         keyspace.Term
	functionCells []keyspace.Term
}

// Matches reports whether r was sealed for the exact Source and authored Flow
// identities supplied by the final assembly. Unavailable identities never
// match, including a malformed Result carrying plausible dense projections.
func Matches(r *Result, sourceID, flowID identity.ContentID) bool {
	return r != nil && sourceID.Available() && flowID.Available() &&
		r.sourceID == sourceID && r.flowID == flowID
}

func (r Result) available() bool {
	return r.sourceID.Available() && r.flowID.Available()
}
