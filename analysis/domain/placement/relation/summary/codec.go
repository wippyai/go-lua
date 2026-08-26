package summary

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// AllocationColumns are the typed output columns of the child family. The
// allocation identity is optional for compatibility with the original
// two-column child ABI; the parent E2E uses the three-column form so one
// Complete child carries AllocationID, Fact, and Evidence as distinct slots.
// They are supplied by the owner binding; this package never issues a
// relation, column, denominator, or schema identity.
type AllocationColumns struct {
	AllocationID *relbindgen.Column[identity.ContentID]
	Fact         *relbindgen.Column[placementdomain.Fact]
	Evidence     *relbindgen.Column[placementdomain.AllocationEvidence]
}

var _ relbindgen.Encoder[AllocationRow] = AllocationCodec{}

// NewAllocationColumns adopts the original two-column child ABI. No second
// column authority is created here.
func NewAllocationColumns(fact *relbindgen.Column[placementdomain.Fact], evidence *relbindgen.Column[placementdomain.AllocationEvidence]) (AllocationColumns, bool) {
	columns := AllocationColumns{Fact: fact, Evidence: evidence}
	if !columns.Available() {
		return AllocationColumns{}, false
	}
	return columns, true
}

// NewAllocationColumnsWithID adopts the honest three-column child ABI used by
// a parent that consumes the child allocation identity alongside Fact and
// Evidence. The ID is a payload slot, not a new key or directory authority.
func NewAllocationColumnsWithID(allocationID *relbindgen.Column[identity.ContentID], fact *relbindgen.Column[placementdomain.Fact], evidence *relbindgen.Column[placementdomain.AllocationEvidence]) (AllocationColumns, bool) {
	columns := AllocationColumns{AllocationID: allocationID, Fact: fact, Evidence: evidence}
	if !columns.AvailableWithID() {
		return AllocationColumns{}, false
	}
	return columns, true
}

// Available reports whether every child output codec is live and typed.
func (columns AllocationColumns) Available() bool {
	return columns.Fact.Available() && columns.Evidence.Available()
}

// AvailableWithID reports whether the optional identity slot is live as well
// as the two canonical child payload columns.
func (columns AllocationColumns) AvailableWithID() bool {
	return columns.AllocationID.Available() && columns.Available()
}

// ParentColumns are the typed output column of the parent answer family.
// The child relation is read through its own sealed columns; it is not copied
// into this product or retained as a second store.
type ParentColumns struct {
	PlacementSchemaID *relbindgen.Column[identity.ContentID]
}

// NewParentColumns adopts the parent answer codec from the owner binding. The
// row's canonical emission presence is the answer marker, so no presence
// payload is duplicated here.
func NewParentColumns(placementSchemaID *relbindgen.Column[identity.ContentID]) (ParentColumns, bool) {
	columns := ParentColumns{PlacementSchemaID: placementSchemaID}
	if !columns.Available() {
		return ParentColumns{}, false
	}
	return columns, true
}

// Available reports whether the parent output codec is live and typed.
func (columns ParentColumns) Available() bool {
	return columns.PlacementSchemaID.Available()
}

// AllocationCodec is the owner encoder for one typed child output.  It is
// intentionally a value type: the mounted binding owns the columns and the
// codec carries no mutable result or store of its own.
type AllocationCodec struct{ columns AllocationColumns }

// NewAllocationCodec creates a child output codec over columns supplied by
// the owner binding.
func NewAllocationCodec(columns AllocationColumns) (AllocationCodec, bool) {
	if !columns.Available() {
		return AllocationCodec{}, false
	}
	return AllocationCodec{columns: columns}, true
}

// Available reports whether this codec can publish a complete child row.
func (codec AllocationCodec) Available() bool { return codec.columns.Available() }

// Columns returns the immutable typed column set used by this codec.
func (codec AllocationCodec) Columns() AllocationColumns { return codec.columns }

// Encode publishes one coherent child product across its declared
// output columns.  The emitter/buffer is transactional; if any later column
// refuses, the binding abandons the staged row rather than exposing a partial
// tuple.
func (codec AllocationCodec) Encode(outputs relbindgen.Outputs, value AllocationRow) bool {
	if !codec.Available() {
		return false
	}
	if !outputs.CarriesValue() {
		// A sparse Placement factor produces one proven-absent child status
		// under the allocation denominator. All payload columns must carry the
		// same absence; encoding a zero AllocationRow as data would fabricate
		// an allocation identity and evidence record.
		if codec.columns.AllocationID.Available() {
			return relbindgen.PutAbsentColumn(outputs, 0) &&
				relbindgen.PutAbsentColumn(outputs, 1) &&
				relbindgen.PutAbsentColumn(outputs, 2)
		}
		return relbindgen.PutAbsentColumn(outputs, 0) &&
			relbindgen.PutAbsentColumn(outputs, 1)
	}
	if !value.Valid() {
		return false
	}
	if codec.columns.AllocationID.Available() {
		if !relbindgen.PutColumn(outputs, 0, codec.columns.AllocationID, value.AllocationID) {
			return false
		}
		if !relbindgen.PutColumn(outputs, 1, codec.columns.Fact, value.Fact) {
			return false
		}
		return relbindgen.PutColumn(outputs, 2, codec.columns.Evidence, value.Evidence)
	}
	if !relbindgen.PutColumn(outputs, 0, codec.columns.Fact, value.Fact) {
		return false
	}
	return relbindgen.PutColumn(outputs, 1, codec.columns.Evidence, value.Evidence)
}

// ParentCodec is the owner encoder for one typed parent answer.
type ParentCodec struct{ columns ParentColumns }

var _ relbindgen.Encoder[ParentAnswer] = ParentCodec{}

// NewParentCodec creates a parent answer codec over columns supplied by the
// owner binding.
func NewParentCodec(columns ParentColumns) (ParentCodec, bool) {
	if !columns.Available() {
		return ParentCodec{}, false
	}
	return ParentCodec{columns: columns}, true
}

// Available reports whether this codec can publish a complete parent answer.
func (codec ParentCodec) Available() bool { return codec.columns.Available() }

// Columns returns the immutable typed column set used by this codec.
func (codec ParentCodec) Columns() ParentColumns { return codec.columns }

// Encode publishes the exact schema identity as the one parent output column.
// The binding's emitted presence carries the canonical answer marker.
func (codec ParentCodec) Encode(outputs relbindgen.Outputs, value ParentAnswer) bool {
	if !codec.Available() || !value.Valid() {
		return false
	}
	return relbindgen.PutColumn(outputs, 0, codec.columns.PlacementSchemaID, value.PlacementSchemaID)
}
