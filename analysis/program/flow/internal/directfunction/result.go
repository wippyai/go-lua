package directfunction

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Result is the immutable occurrence-specific direct Function proof.  Each
// retained plane is one-based and dense by its authored family; zero is the
// explicit absence value. No Source, authored Flow, Static, or Module view is
// retained after sealing; only their scalar content identities remain as the
// owner fence.
type Result struct {
	// Provenance is the narrow owner fence for this derived projection. The
	// sealed proof retains only these scalar identities, never owner
	// views/pointers. Their order is the canonical assembly order.
	sourceID identity.ContentID
	flowID   identity.ContentID
	staticID identity.ContentID
	moduleID identity.ContentID

	readFunctions []keyspace.Term
	callFunctions []keyspace.Term
	loopFunctions []keyspace.Term
	functionCount uint32
}

// Matches reports whether r was sealed for the exact Source, authored Flow,
// Static, and Module identities supplied by final assembly. Unavailable
// identities never match, including a malformed Result carrying plausible
// query planes.
func Matches(r *Result, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return r != nil && r.available() && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		r.sourceID == sourceID && r.flowID == flowID && r.staticID == staticID && r.moduleID == moduleID
}

func (r *Result) available() bool {
	return r != nil && r.sourceID.Available() && r.flowID.Available() && r.staticID.Available() && r.moduleID.Available()
}

// DirectFunction returns the exact Function denoted by value.  A Function
// Term denotes itself even when it is dead; a Read is accepted only when the
// sealed Read projection retained an occurrence-specific candidate.
func (r *Result) DirectFunction(value keyspace.Term) (keyspace.Term, bool) {
	if !r.available() {
		return 0, false
	}
	switch keyspace.TermFamily(value) {
	case keyspace.FamilyFunction:
		if r.validFunction(value) {
			return value, true
		}
	case keyspace.FamilyRead:
		return r.ReadFunction(value)
	}
	return 0, false
}

// ReadFunction returns the retained direct Function candidate for one Read.
func (r *Result) ReadFunction(read keyspace.Term) (keyspace.Term, bool) {
	return r.plane(r.readFunctions, read, keyspace.FamilyRead)
}

// CallFunction returns the retained direct Function candidate for one Call.
func (r *Result) CallFunction(call keyspace.Term) (keyspace.Term, bool) {
	return r.plane(r.callFunctions, call, keyspace.FamilyCall)
}

// GenericLoopFunction returns the retained candidate for one GenericFor
// Loop.  Non-generic loops have no retained slot and fail closed.
func (r *Result) GenericLoopFunction(loop keyspace.Term) (keyspace.Term, bool) {
	return r.plane(r.loopFunctions, loop, keyspace.FamilyLoop)
}

func (r *Result) plane(plane []keyspace.Term, term keyspace.Term, family keyspace.Family) (keyspace.Term, bool) {
	if !r.available() || keyspace.TermFamily(term) != family {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(plane)) {
		return 0, false
	}
	function := plane[ordinal]
	if !r.validFunction(function) {
		return 0, false
	}
	return function, true
}

func (r *Result) validFunction(function keyspace.Term) bool {
	return r.available() && keyspace.TermFamily(function) == keyspace.FamilyFunction &&
		keyspace.TermOrdinal(function) != 0 && keyspace.TermOrdinal(function) <= r.functionCount
}
