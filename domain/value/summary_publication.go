package value

import (
	"github.com/wippyai/go-lua/analysis/identity"
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

// SummaryPublication is everything this domain states about publishing a Value
// summary: the one declared class a written coordinate is held at, the columns
// the answer publishes, and the projection those columns are read out of.
//
// The payload keeps Value's compact atom/capability word image rather than
// expanding it into structural objects, so the image is declared as the row's
// one variable column. A coordinate either carries a Value or no producer
// wrote it, so the row state names the one declared publication class and this
// domain declares no vocabulary of its own.
//
// What family these columns are published under, and that its rows carry the
// coordinates they hold, are not stated here: they follow from the query
// registration this family is declared by, and the composition seals the two
// together. Present values must belong to one exact Link schema, and that Link
// identity is the coordinate space the rows are fenced by: it is what makes
// the private word ordinals readable.
//
// Nothing below is a wire statement. Rows are addressed in the schema's
// canonical coordinate order, which is ascending by portable identity, and the
// plane driver turns that walk into the payload; the detached image is
// therefore a function of the coordinates it holds and not of the declaration
// position they were sealed at.
func SummaryPublication() plane.Publication[ValueSummaryObservation] {
	return plane.Publication[ValueSummaryObservation]{
		States: structure.CategoryPublicationRowClass,
		Columns: []plane.Column{
			{Key: "top", Carrier: plane.CarrierFlag},
			{Key: "image", Carrier: plane.CarrierWords},
		},
		Projection: plane.Projection[ValueSummaryObservation]{
			Owner:       summaryPublicationOwner,
			Extent:      summaryPublicationExtent,
			Cardinality: summaryPublicationCardinality,
			Row:         summaryPublicationRow,
			Cell:        summaryPublicationCell,
		},
	}
}

// summaryPublicationOwner is the Link the published coordinate plane belongs
// to. An observation no schema opened states no owner, and the sealed keyed
// layout refuses it.
func summaryPublicationOwner(observation ValueSummaryObservation) identity.ContentID {
	if observation.owner == nil {
		return identity.ContentID{}
	}
	return observation.owner.LinkID()
}

// summaryPublicationExtent is this domain's admission of its own answer as
// well as its size: a complete correlated summary owned by one sealed schema
// publishes one row per coordinate and the words of every present Value, and
// anything else publishes nothing.
func summaryPublicationExtent(observation ValueSummaryObservation) (int, int, bool) {
	owner := observation.owner
	if owner == nil || !summaryObservationOwned(owner, observation) || !owner.LinkID().Available() {
		return 0, 0, false
	}
	words := 0
	for index, held := range observation.Present {
		if !held {
			continue
		}
		words += len(observation.Values[index].image)
	}
	return len(observation.Values), words, true
}

func summaryPublicationCardinality(observation ValueSummaryObservation) uint64 {
	return uint64(observation.Rows)
}

// summaryPublicationRow addresses one published coordinate. A coordinate no
// producer wrote is published at its identity with no class, which is the
// plane's unwritten row.
func summaryPublicationRow(observation ValueSummaryObservation, position int) (identity.ContentID, schema.Key, bool) {
	id, dense, resolved := observation.owner.CanonicalCoordinateAt(position)
	if !resolved || uint64(dense) >= uint64(len(observation.Present)) {
		return identity.ContentID{}, "", false
	}
	if !observation.Present[dense] {
		return id, "", true
	}
	return id, structure.PublicationClassHeld, true
}

// summaryPublicationCell states one written coordinate's column values.
func summaryPublicationCell(observation ValueSummaryObservation, position, column int) (plane.Cell, bool) {
	_, dense, resolved := observation.owner.CanonicalCoordinateAt(position)
	if !resolved || uint64(dense) >= uint64(len(observation.Values)) {
		return plane.Cell{}, false
	}
	value := observation.Values[dense]
	switch column {
	case SummaryColumnTop:
		return plane.FlagCell(value.top), true
	case SummaryColumnImage:
		return plane.WordsCell(value.image), true
	}
	return plane.Cell{}, false
}
