// Package continuation owns the canonical internal continuation projection.
//
// It retains only lexical Cell scopes and unpolarized reaching Guard support
// for the existing executable candidate-bearing subjects.  Evaluation order
// and already-evaluated operands belong to evaluation; this package does not
// import or retain that projection.  The result is a provenance-fenced,
// allocation-free query surface over the compact lexical-scope chain and
// append-only Guard prefix DAG.
package continuation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

const absentRoot = ^uint32(0)

// Result is the immutable continuation projection.  Cell roots are dense only
// in the six continuation-subject family planes. Guard roots use those same
// planes plus the minimal family-indexed projection of canonical Causal
// Successor endpoints. An absent root is all-ones and a present empty root is
// zero. Cells and Guards keep independent owner-local compact stores.
type Result struct {
	sourceID identity.ContentID
	flowID   identity.ContentID
	staticID identity.ContentID
	moduleID identity.ContentID
	cells    cellProjection
	guards   guardProjection
}

// cellRootRecord is the Seal-time admission record for one Cell subject slot.
// The record is private and immutable after publication: queries compare the
// dense root plane and root header with this exact root/count pair instead of
// revalidating a retained store.
type cellRootRecord struct {
	root    uint32
	count   uint32
	present bool
	node    scopeNode
}

// guardRootRecord is the analogous owner-local record for one Guard subject
// slot.  Keeping the two records separate avoids retaining an unrelated store
// header in every projection.
type guardRootRecord struct {
	root    uint32
	count   uint32
	present bool
	node    guardNode
}

type cellProjection struct {
	roots   [keyspace.FamilyCount][]uint32
	records [keyspace.FamilyCount][]cellRootRecord
	nodes   []scopeNode
	terms   []keyspace.Term
	counts  [keyspace.FamilyCount]uint32
}

type guardProjection struct {
	roots   [keyspace.FamilyCount][]uint32
	records [keyspace.FamilyCount][]guardRootRecord
	nodes   []guardNode
	counts  [keyspace.FamilyCount]uint32
	// families is the family-level admission fence for endpoint roots. The
	// roots remain the per-ordinal fence: an ordinal whose route endpoint is
	// absent retains absentRoot and is not admitted merely because another
	// ordinal in the same family is live.
	families [keyspace.FamilyCount]bool
}

// Matches reports whether result was sealed for the exact Source, authored
// Flow, Static, and Module identities supplied by the caller. Unavailable
// identities never match, including a malformed result carrying plausible
// projection arrays.
func Matches(result *Result, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return result != nil && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		result.sourceID.Available() && result.flowID.Available() && result.staticID.Available() && result.moduleID.Available() &&
		result.sourceID == sourceID && result.flowID == flowID && result.staticID == staticID && result.moduleID == moduleID
}

func (result *Result) available() bool {
	return result != nil && result.sourceID.Available() && result.flowID.Available() && result.staticID.Available() && result.moduleID.Available()
}
