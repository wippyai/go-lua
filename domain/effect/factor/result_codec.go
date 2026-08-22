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

// ExactResultStates is the row state vocabulary this family publishes its
// written rows at. An Effect answer either carries an algebra value or no
// producer wrote it, so the family names the one declared class a written row
// is held at and declares no vocabulary of its own.
const ExactResultStates = structure.CategoryPublicationRowClass

// ExactResultColumns are the columns one Effect answer publishes. The joined
// algebra value never crosses this boundary; what is declared is the
// authenticated public atom projection.
//
// Effect answers one point, so it declares no coordinate plane. That is not
// stated here either: it follows from the general fold the query registration
// declares, and the composition seals the two together.
func ExactResultColumns() []plane.Column {
	return []plane.Column{
		{Key: "top", Carrier: plane.CarrierFlag},
		{Key: "atoms", Carrier: plane.CarrierAtoms},
	}
}

// EncodeResult canonically detaches one frozen Effect observation onto the
// family's sealed layout.
func EncodeResult(layout *plane.Sealed, observation EffectObservation) (present bool, rows uint64, payload []byte, ok bool) {
	if !observation.Valid || observation.Rows > 1 || observation.Top && len(observation.Atoms) != 0 ||
		!observation.Present && (observation.Top || len(observation.Atoms) != 0) ||
		observation.Present && observation.Rows == 1 && observation.seal != sealAtoms(observation.Atoms) {
		return false, 0, nil, false
	}
	writer, begun := plane.Begin(layout, identity.ContentID{}, 1, len(observation.Atoms))
	if !begun {
		return false, 0, nil, false
	}
	written := true
	if !observation.Present {
		written = writer.Absent(identity.ContentID{})
	} else {
		written = writer.Row(identity.ContentID{}, structure.PublicationClassHeld)
		written = written && writer.Flag(observation.Top)
		for _, atom := range observation.Atoms {
			written = written && writer.Atom(atom)
		}
		written = written && writer.CloseColumn()
	}
	if !written || !writer.EndRow() {
		return false, 0, nil, false
	}
	return writer.Finish(uint64(observation.Rows))
}
