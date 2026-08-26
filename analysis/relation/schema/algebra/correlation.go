package algebra

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// ApplyCorrelation is the sealed declaration for a correlated multi-child
// Apply. Population is the independent closed Q universe that drives the
// cogroup. Coordinate is the owner-issued semantic column in Population that
// identifies one query site, Type is the codec identity for that coordinate,
// and each projection is the exact ordered column vector used to look up the
// corresponding child partition.
//
// Every Complete input keeps its own signature.Input relation, denominator,
// and order key; the checker proves those authorities independently. Mount
// may therefore derive one keyed lookup per child without copying facts into
// a composite relation or retaining a second state/cache. Population is the
// one exception because it is the independent driver authority, not a child
// range or a replacement for any child's denominator.
type ApplyCorrelation struct {
	population  model.DenominatorRef
	coordinate  model.ColumnID
	typeID      model.TypeID
	projections [][]model.ColumnID
	digest      identity.ContentID
}

// NewApplyCorrelation freezes one independent query-site population, one
// common query-site coordinate, and the exact ordered projection vector for
// each Apply child. A one-column vector is the owner-issued coordinate used
// to redeem that child's Q partition. An empty vector is a sealed shared
// Complete child: it has no Q partition and is broadcast from its own global
// Complete denominator. Child order is declaration order; no child is a
// runtime population anchor.
// Cross-schema membership, child shape, and Complete range compatibility
// remain checker laws.
func NewApplyCorrelation(population model.DenominatorRef, coordinate model.ColumnID, typeID model.TypeID, projections [][]model.ColumnID) ApplyCorrelation {
	value := ApplyCorrelation{
		population:  population,
		coordinate:  coordinate,
		typeID:      typeID,
		projections: cloneColumnVectors(projections),
	}
	value.digest = digestApplyCorrelation(value)
	return value
}

// Population returns the independent closed query-site denominator. It is
// not a child range denominator and must never be inferred from a child.
func (correlation ApplyCorrelation) Population() model.DenominatorRef { return correlation.population }

// Coordinate returns the owner-issued common query-site coordinate.
func (correlation ApplyCorrelation) Coordinate() model.ColumnID { return correlation.coordinate }

// Type returns the owner-issued semantic type of Coordinate.
func (correlation ApplyCorrelation) Type() model.TypeID { return correlation.typeID }

// Specified reports whether an Apply carries correlation declaration data,
// including malformed data that the checker must refuse. The zero value is
// the ordinary uncorrelated Apply form.
func (correlation ApplyCorrelation) Specified() bool {
	return correlation.population.Available() || correlation.coordinate.Available() || correlation.typeID.Available() || len(correlation.projections) != 0 || correlation.digest.Available()
}

// ProjectionCount reports the number of ordered child projections without
// exposing mutable storage.
func (correlation ApplyCorrelation) ProjectionCount() int {
	if !correlation.Available() {
		return 0
	}
	return len(correlation.projections)
}

// ProjectionAt returns one exact child projection in Apply child order.
func (correlation ApplyCorrelation) ProjectionAt(index int) ([]model.ColumnID, bool) {
	if !correlation.Available() || index < 0 || index >= len(correlation.projections) {
		return nil, false
	}
	return append([]model.ColumnID(nil), correlation.projections[index]...), true
}

// SharedAt reports whether the child at index is a globally shared Complete
// child. A shared child intentionally has no projection coordinate and must
// therefore be authenticated by its own Complete denominator rather than a
// per-population partition. The checker proves the exact child shape and
// forbids Q-coordinate-dependent slots; this accessor only exposes the
// sealed algebra marker.
func (correlation ApplyCorrelation) SharedAt(index int) bool {
	projection, ok := correlation.ProjectionAt(index)
	return ok && len(projection) == 0
}

// Projections returns the exact ordered child projection vectors.
func (correlation ApplyCorrelation) Projections() [][]model.ColumnID {
	if !correlation.Available() {
		return nil
	}
	return cloneColumnVectors(correlation.projections)
}

// Available reports whether the local correlation shape is sealed. Registry
// membership and the relationship between projections and child ranges are
// deliberately checked by authority/typing, not hidden in this constructor.
func (correlation ApplyCorrelation) Available() bool {
	if !correlation.population.Available() || !correlation.coordinate.Available() || !correlation.typeID.Available() || len(correlation.projections) < 2 || !correlation.digest.Available() {
		return false
	}
	for _, projection := range correlation.projections {
		// A child is either keyed by exactly one owner-issued query-site
		// coordinate or is the sealed shared-Complete form with no coordinate.
		// A future multi-component key must be a separate sealed contract;
		// silently treating an arbitrary vector as one coordinate would make
		// its codec and replay semantics ambiguous.
		if len(projection) > 1 {
			return false
		}
		for _, column := range projection {
			if !column.Available() {
				return false
			}
		}
	}
	return correlation.digest == digestApplyCorrelation(correlation)
}

// Digest returns the sealed declaration identity.
func (correlation ApplyCorrelation) Digest() identity.ContentID {
	if !correlation.Available() {
		return identity.ContentID{}
	}
	return correlation.digest
}

// Equal compares the complete immutable declaration, including projection
// order. Unavailable declarations never establish a correlation.
func (correlation ApplyCorrelation) Equal(other ApplyCorrelation) bool {
	if !correlation.Available() || !other.Available() || correlation.digest != other.digest || correlation.population != other.population || correlation.coordinate != other.coordinate || correlation.typeID != other.typeID || len(correlation.projections) != len(other.projections) {
		return false
	}
	for index := range correlation.projections {
		left, right := correlation.projections[index], other.projections[index]
		if len(left) != len(right) {
			return false
		}
		for column := range left {
			if left[column] != right[column] {
				return false
			}
		}
	}
	return true
}

func (correlation ApplyCorrelation) digestBytes() []byte {
	parts := appendDenominator(nil, correlation.population)
	parts = appendColumn(parts, correlation.coordinate)
	parts = appendType(parts, correlation.typeID)
	parts = appendLength(parts, len(correlation.projections))
	for _, projection := range correlation.projections {
		parts = appendColumns(parts, projection)
	}
	return parts
}

func digestApplyCorrelation(correlation ApplyCorrelation) identity.ContentID {
	return derive("analysis/relation/schema/algebra/apply-correlation/v1", correlation.digestBytes())
}

func cloneColumnVectors(source [][]model.ColumnID) [][]model.ColumnID {
	if len(source) == 0 {
		return nil
	}
	copyOf := make([][]model.ColumnID, len(source))
	for index, columns := range source {
		copyOf[index] = cloneColumns(columns)
	}
	return copyOf
}
