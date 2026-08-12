package analysis

import (
	"crypto/sha256"
	"encoding/binary"

	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
)

// effectObservation is a detached projection of one exact Effect root. Hot
// Factor values and owner pointers never cross the query result boundary.
type effectObservation struct {
	atoms   []keyspace.ContentID
	rows    uint32
	present bool
	top     bool
	valid   bool
}

func projectEffectObservation(algebra *effectfactor.Algebra, observation engine.Observation, read engine.QueryRead[engine.OrderedCells[effectfactor.Value]]) effectObservation {
	result := effectObservation{}
	rows := uint32(0)
	joined := effectfactor.Value{}
	complete := algebra != nil && engine.ProjectRows(observation, func(row engine.QueryRow) bool {
		rows++
		cells, ok := engine.QueryValue(row, read)
		if !ok || cells.Count() != 1 {
			return false
		}
		value, present, available := cells.At(0)
		if !available {
			return false
		}
		if !present {
			return true
		}
		if !result.present {
			joined, result.present = value, true
			return true
		}
		var joinedOK bool
		joined, joinedOK = algebra.Join(joined, value)
		return joinedOK
	})
	if rows != 0 {
		result.rows = 1
	}
	if complete && result.present {
		if algebra.Equal(joined, algebra.Top()) {
			result.top = true
		} else {
			for index := 0; ; index++ {
				atom, found := algebra.AtomAt(joined, index)
				if !found {
					break
				}
				id, idOK := algebra.AtomID(atom)
				if !idOK || !id.Available() {
					complete = false
					break
				}
				result.atoms = append(result.atoms, id)
			}
		}
	}
	// Guard rows remain precise during solving and are joined only at this
	// detached, guard-insensitive body boundary. Zero rows are the exact
	// empty/unreachable Effect observation; any nonempty partition normalizes
	// to one canonical summary row.
	result.valid = complete
	if !result.valid {
		result.atoms = nil
	}
	return result
}

func cloneEffectObservation(input effectObservation) effectObservation {
	result := input
	result.atoms = append([]keyspace.ContentID(nil), input.atoms...)
	return result
}

func equalEffectObservation(left, right effectObservation) bool {
	if left.rows != right.rows || left.present != right.present || left.top != right.top || left.valid != right.valid || len(left.atoms) != len(right.atoms) {
		return false
	}
	for index := range left.atoms {
		if left.atoms[index] != right.atoms[index] {
			return false
		}
	}
	return true
}

func fingerprintEffectObservation(value effectObservation) uint64 {
	hash := sha256.New()
	var header [8]byte
	binary.BigEndian.PutUint32(header[:4], value.rows)
	if value.present {
		header[4] = 1
	}
	if value.top {
		header[5] = 1
	}
	if value.valid {
		header[6] = 1
	}
	_, _ = hash.Write(header[:])
	for _, id := range value.atoms {
		_, _ = hash.Write(id[:])
	}
	sum := hash.Sum(nil)
	return binary.BigEndian.Uint64(sum[:8])
}
