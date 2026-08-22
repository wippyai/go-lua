package factor

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/plane"
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

// ExactClassHeld is the class of an answer a producer wrote. An Effect answer
// either carries an algebra value or no producer wrote it, so the state
// vocabulary names exactly that one class.
const ExactClassHeld schema.Key = "held"

// ExactResultLayout is the sealed declaration one Effect answer is published
// under. Effect answers one point, so it declares no coordinate plane: the
// row's identity is the query site's own and restating it here would publish
// a second authority for it. The joined algebra value never crosses this
// boundary; what is declared is the authenticated public atom projection.
var ExactResultLayout = exactResultLayout()

func exactResultLayout() *plane.Sealed {
	sealed, _ := plane.Seal(plane.Layout{
		Family: ExactResultFamily,
		States: []schema.Key{ExactClassHeld},
		Columns: []plane.Column{
			{Key: "top", Carrier: plane.CarrierFlag},
			{Key: "atoms", Carrier: plane.CarrierAtoms},
		},
	})
	return sealed
}

// EncodeResult canonically detaches one frozen Effect observation onto the
// family's sealed layout.
func EncodeResult(observation EffectObservation) (present bool, rows uint64, payload []byte, ok bool) {
	if !observation.Valid || observation.Rows > 1 || observation.Top && len(observation.Atoms) != 0 ||
		!observation.Present && (observation.Top || len(observation.Atoms) != 0) ||
		observation.Present && observation.Rows == 1 && observation.seal != sealAtoms(observation.Atoms) {
		return false, 0, nil, false
	}
	writer, begun := plane.Begin(ExactResultLayout, identity.ContentID{}, 1, len(observation.Atoms))
	if !begun {
		return false, 0, nil, false
	}
	written := true
	if !observation.Present {
		written = writer.Absent(identity.ContentID{})
	} else {
		written = writer.Row(identity.ContentID{}, ExactClassHeld)
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
