package factor

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/schema"
)

// ExactResultFamily is the canonical query family key for exact effects.
const ExactResultFamily schema.Key = "effect-exact"

const effectResultFormat uint64 = 1

// EncodeResult canonically detaches one frozen Effect observation. The joined
// algebra value never crosses this boundary; only the authenticated public atom
// projection is encoded.
func EncodeResult(observation EffectObservation) (present bool, rows uint64, payload []byte, ok bool) {
	if !observation.Valid || observation.Rows > 1 || observation.Top && len(observation.Atoms) != 0 ||
		!observation.Present && (observation.Top || len(observation.Atoms) != 0) ||
		observation.Present && observation.Rows == 1 && observation.seal != sealAtoms(observation.Atoms) {
		return false, 0, nil, false
	}
	for _, atom := range observation.Atoms {
		if !atom.Available() {
			return false, 0, nil, false
		}
	}
	payload = make([]byte, 8+2+8+len(observation.Atoms)*32)
	binary.BigEndian.PutUint64(payload[:8], effectResultFormat)
	if observation.Present {
		payload[8] = 1
	}
	if observation.Top {
		payload[9] = 1
	}
	binary.BigEndian.PutUint64(payload[10:18], uint64(len(observation.Atoms)))
	cursor := 18
	for _, atom := range observation.Atoms {
		copy(payload[cursor:cursor+32], atom[:])
		cursor += 32
	}
	return observation.Present, uint64(observation.Rows), payload, true
}
