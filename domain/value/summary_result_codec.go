package value

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/plane"
)

// SummaryResultFamily is the canonical query family key for value summaries.
const SummaryResultFamily schema.Key = "value-summary"

// The declaration positions of this family's published columns, and the one
// class a written coordinate row is published at. A coordinate either carries a
// Value or no producer wrote it, so the state vocabulary names exactly that one
// class rather than encoding presence as a number.
const (
	SummaryColumnTop = iota
	SummaryColumnImage
)

// SummaryClassHeld is the class of a coordinate row a producer wrote.
const SummaryClassHeld schema.Key = "held"

// SummaryResultLayout is the sealed declaration one Value summary is published
// under. The payload keeps Value's compact atom/capability word image rather
// than expanding it into structural objects, so the image is declared as the
// row's one variable column. Present values must belong to one exact Link
// schema, and that Link identity is the coordinate space the rows are fenced
// by: it is what makes the private word ordinals readable.
var SummaryResultLayout = summaryResultLayout()

func summaryResultLayout() *plane.Sealed {
	sealed, _ := plane.Seal(plane.Layout{
		Family: SummaryResultFamily,
		Keyed:  true,
		States: []schema.Key{SummaryClassHeld},
		Columns: []plane.Column{
			{Key: "top", Carrier: plane.CarrierFlag},
			{Key: "image", Carrier: plane.CarrierWords},
		},
	})
	return sealed
}

// EncodeSummaryResult canonically detaches the complete correlated Value
// summary onto the family's sealed layout. Rows are published in the schema's
// canonical coordinate order, which is ascending by portable identity, so the
// detached image is a function of the coordinates it holds and not of the
// declaration position they were sealed at.
func EncodeSummaryResult(observation ValueSummaryObservation) (present bool, rows uint64, payload []byte, ok bool) {
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
	writer, begun := plane.Begin(SummaryResultLayout, owner.LinkID(), count, words)
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
			written = writer.Row(id, SummaryClassHeld)
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
