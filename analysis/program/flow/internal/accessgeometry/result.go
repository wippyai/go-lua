package accessgeometry

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// indexAccess is one candidate indexed access. Read is nonzero for an IndexGet
// route; Write is nonzero for an IndexSet route. Values and Position are
// meaningful only for a Write route. Position is the zero-based position in
// its authored Assign, and is -1 on a Read route. Lens and KeyTerm retain the
// authored identities; normalized exact keys remain in ExactLenses.
//
// Base and KeyTerm are the evaluated access geometry. KeyTerm remains the raw
// authored operand; normalized exact keys are queried from ExactLenses and are
// never newly interned here.
type indexAccess struct {
	Read     keyspace.Term
	Write    keyspace.Term
	Base     keyspace.Term
	KeyTerm  keyspace.Term
	Values   keyspace.Term
	Position int
	Lens     keyspace.Term
}

type tableFieldProjection struct {
	keys []keyspace.Key
}

type exactLensProjection struct {
	keys []keyspace.Key
}

type dynamicLensProjection struct {
	// Dynamic rows deliberately retain a dense zero plane.  The plane keeps
	// the authored denominator explicit without inventing a dynamic key.
	keys []keyspace.Key
}

type indexProjection struct {
	accesses   []indexAccess
	reads      []uint32
	writes     []uint32
	readCount  int
	writeCount int
}

// Result is the immutable derived access-geometry projection.  The four
// scalar identities are the only retained provenance; no Source, authored
// Flow, or candidates owner is retained after Seal.
type Result struct {
	sourceID identity.ContentID
	flowID   identity.ContentID
	staticID identity.ContentID
	moduleID identity.ContentID

	tableFields   tableFieldProjection
	exactLenses   exactLensProjection
	dynamicLenses dynamicLensProjection
	indexAccesses indexProjection
}

// Matches reports whether result belongs to the exact Source, authored Flow,
// Static, and Module identities supplied by assembly. Unavailable identities
// never match, including a malformed result carrying plausible arrays.
func Matches(result *Result, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return result != nil && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		result.sourceID.Available() && result.flowID.Available() && result.staticID.Available() && result.moduleID.Available() &&
		result.sourceID == sourceID && result.flowID == flowID && result.staticID == staticID && result.moduleID == moduleID
}

func (result *Result) available() bool {
	return result != nil && result.sourceID.Available() && result.flowID.Available() && result.staticID.Available() && result.moduleID.Available()
}

// TableFields returns normalized keys for every authored TableField in its
// canonical ordinal order. FieldKey rows and non-storable nil/NaN rows return
// a zero Key with ok=true; malformed rows fail closed.
func (result *Result) TableFields() TableFields { return TableFields{result: result} }

// ExactLenses returns normalized keys for every authored exact Lens.
func (result *Result) ExactLenses() ExactLenses { return ExactLenses{result: result} }

// DynamicLenses returns the dense zero key plane for every authored dynamic
// Lens. A dynamic key is intentionally never guessed or interned here.
func (result *Result) DynamicLenses() DynamicLenses { return DynamicLenses{result: result} }

// IndexAccesses returns the route-free candidate indexed-access projection.
// Reads and Writes are separate typed planes; no combined arm/kind API is
// retained.
func (result *Result) IndexAccesses() IndexAccesses { return IndexAccesses{result: result} }
