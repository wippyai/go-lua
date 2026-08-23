package factor

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// ExactResultFamily is the canonical query family key for exact effects.
const ExactResultFamily schema.Key = "effect-exact"

// The declaration positions of this family's published columns. A consumer
// names a column by its position in the sealed layout below; nothing in this
// domain spells a byte offset.
const (
	ExactColumnTop = iota
	ExactColumnAtoms
)

// ExactPublication is everything this domain states about publishing an Effect
// answer: the one declared class a written answer is held at, the columns it
// publishes, and the projection those columns are read out of.
//
// The joined algebra value never crosses this boundary; what is declared is
// the authenticated public atom projection. An Effect answer either carries an
// algebra value or no producer wrote it, so the row state names the one
// declared publication class and this domain declares no vocabulary of its
// own.
//
// Effect answers one point, so it publishes no coordinate plane. That is not
// stated here either: it follows from the general fold the query registration
// declares, and the composition seals the two together.
func ExactPublication() plane.Publication[EffectObservation] {
	return plane.Publication[EffectObservation]{
		States: structure.CategoryPublicationRowClass,
		Columns: []plane.Column{
			{Key: "top", Carrier: plane.CarrierFlag},
			{Key: "atoms", Carrier: plane.CarrierAtoms},
		},
		Projection: plane.Projection[EffectObservation]{
			Owner:       exactPublicationOwner,
			Extent:      exactPublicationExtent,
			Cardinality: exactPublicationCardinality,
			Row:         exactPublicationRow,
			Cell:        exactPublicationCell,
		},
	}
}

// exactPublicationOwner states that this family publishes no coordinate plane.
// The sealed unkeyed layout refuses an owner, so an Effect answer can never
// acquire one.
func exactPublicationOwner(EffectObservation) identity.ContentID { return identity.ContentID{} }

// exactPublicationExtent is this domain's admission of its own answer as well
// as its size: one published row, the atoms of a present answer, and a refusal
// for any observation whose presence, top arm, cardinality, or atom seal
// disagree with one another.
func exactPublicationExtent(observation EffectObservation) (int, int, bool) {
	if !observation.Valid || observation.Rows > 1 || observation.Top && len(observation.Atoms) != 0 ||
		!observation.Present && (observation.Top || len(observation.Atoms) != 0) ||
		observation.Present && observation.Rows == 1 && observation.seal != sealAtoms(observation.Atoms) {
		return 0, 0, false
	}
	return 1, len(observation.Atoms), true
}

func exactPublicationCardinality(observation EffectObservation) uint64 {
	return uint64(observation.Rows)
}

// exactPublicationRow addresses the family's one published row. An answer no
// producer wrote is the plane's unwritten row.
func exactPublicationRow(observation EffectObservation, _ int) (identity.ContentID, schema.Key, bool) {
	if !observation.Present {
		return identity.ContentID{}, "", true
	}
	return identity.ContentID{}, structure.PublicationClassHeld, true
}

// exactPublicationCell states the written answer's column values.
func exactPublicationCell(observation EffectObservation, _, column int) (plane.Cell, bool) {
	switch column {
	case ExactColumnTop:
		return plane.FlagCell(observation.Top), true
	case ExactColumnAtoms:
		return plane.AtomsCell(observation.Atoms), true
	}
	return plane.Cell{}, false
}
