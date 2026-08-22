package value

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// SummaryResultFamily is the canonical query family key for value summaries.
const SummaryResultFamily schema.Key = "value-summary"

// The declaration positions of this family's published columns. A consumer
// names a column by its position in the columns declared below; nothing in
// this domain spells a byte offset.
const (
	SummaryColumnTop = iota
	SummaryColumnImage
)

// SummaryResultStates is the row state vocabulary this family publishes its
// written rows at. A coordinate either carries a Value or no producer wrote
// it, so the family names the one declared class a written row is held at and
// declares no vocabulary of its own.
const SummaryResultStates = structure.CategoryPublicationRowClass

// SummaryResultColumns are the columns one Value summary publishes. The
// payload keeps Value's compact atom/capability word image rather than
// expanding it into structural objects, so the image is declared as the row's
// one variable column.
//
// What family these columns are published under, and that its rows carry the
// coordinates they hold, are not stated here: they follow from the query
// registration this family is declared by, and the composition seals the two
// together. Present values must belong to one exact Link schema, and that Link
// identity is the coordinate space the rows are fenced by: it is what makes
// the private word ordinals readable.
func SummaryResultColumns() []plane.Column {
	return []plane.Column{
		{Key: "top", Carrier: plane.CarrierFlag},
		{Key: "image", Carrier: plane.CarrierWords},
	}
}

// EncodeSummaryResult canonically detaches the complete correlated Value
// summary onto the family's sealed layout. Rows are published in the schema's
// canonical coordinate order, which is ascending by portable identity, so the
// detached image is a function of the coordinates it holds and not of the
// declaration position they were sealed at.
func EncodeSummaryResult(layout *plane.Sealed, observation ValueSummaryObservation) (present bool, rows uint64, payload []byte, ok bool) {
	count := len(observation.Values)
	owner := observation.owner
	if owner == nil || !summaryObservationOwned(owner, observation) || count == 0 ||
		len(observation.Present) != count || observation.Rows > 1 || !owner.LinkID().Available() ||
		owner.CoordinateCount() != count {
		return false, 0, nil, false
	}
	words := 0
	any := false
	for index, held := range observation.Present {
		if !held {
			continue
		}
		value := observation.Values[index]
		if !value.valid() || value.schema != owner {
			return false, 0, nil, false
		}
		words += len(value.image)
		any = true
	}
	if any != (observation.Rows == 1) {
		return false, 0, nil, false
	}
	writer, begun := plane.Begin(layout, owner.LinkID(), count, words)
	if !begun {
		return false, 0, nil, false
	}
	for position := 0; position < count; position++ {
		id, dense, resolved := owner.CanonicalCoordinateAt(position)
		if !resolved || uint64(dense) >= uint64(count) {
			return false, 0, nil, false
		}
		written := true
		if !observation.Present[dense] {
			written = writer.Absent(id)
		} else {
			written = writer.Row(id, structure.PublicationClassHeld)
			value := observation.Values[dense]
			written = written && writer.Flag(value.top)
			for _, word := range value.image {
				written = written && writer.Word(word)
			}
			written = written && writer.CloseColumn()
		}
		if !written || !writer.EndRow() {
			return false, 0, nil, false
		}
	}
	return writer.Finish(uint64(observation.Rows))
}
