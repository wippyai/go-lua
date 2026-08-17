package factor

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	internalhash "github.com/wippyai/go-lua/internal/hash"
)

// EffectObservation is the detached result of one exact Effect query. It is
// declared beside the algebra that folds it: a fold's output shape belongs to
// the domain that reduces into it, and only this package can write the joined
// accumulator or the seal.
//
// The joined value is the accumulator alone and is dropped when the result
// freezes, so a frozen observation retains no algebra. The seal is the scalar
// that survives freezing: it authenticates the public atom projection, so
// neither a hand-shaped literal nor a mutated clone can present itself as an
// observed one.
type EffectObservation struct {
	Atoms   []identity.ContentID
	Rows    uint32
	Present bool
	Top     bool
	Valid   bool
	joined  Value
	seal    uint64
}

// BeginEffect starts the fold state for the exact Effect query.
func BeginEffect(algebra *Algebra) EffectObservation {
	return EffectObservation{Valid: algebra != nil && algebra.Valid()}
}

// AccumulateEffect joins one guarded Effect row into the detached result. The
// opaque algebra value lets the fold preserve joins without rebuilding atoms
// from portable IDs or retaining a mutable query-side map.
func AccumulateEffect(algebra *Algebra, result EffectObservation, value Value, present, available bool) (EffectObservation, bool) {
	if algebra == nil || !algebra.Valid() || !available {
		return EffectObservation{}, false
	}
	result.Rows = 1
	if !present {
		return result, true
	}
	if !result.Present {
		result.joined = value
		result.Present = true
	} else {
		joined, ok := algebra.Join(result.joined, value)
		if !ok {
			return EffectObservation{}, false
		}
		result.joined = joined
	}
	result.Atoms = nil
	result.Top = algebra.Equal(result.joined, algebra.Top())
	if !result.Top {
		for index := 0; ; index++ {
			atom, found := algebra.AtomAt(result.joined, index)
			if !found {
				break
			}
			id, ok := algebra.AtomID(atom)
			if !ok || !id.Available() {
				return EffectObservation{}, false
			}
			result.Atoms = append(result.Atoms, id)
		}
	}
	result.seal = sealAtoms(result.Atoms)
	return result, true
}

func CloneEffect(input EffectObservation) EffectObservation {
	input.Atoms = append([]identity.ContentID(nil), input.Atoms...)
	input.joined = Value{}
	return input
}

// ProvesAtomBinding is the only typed membership bridge from a detached Effect
// observation to an already-issued beta binding. It never accepts an atom ID
// from its caller. The seal authenticates the public atom projection after
// freezing, so a manually shaped observation cannot be used as a runtime
// publication proof.
func (observation EffectObservation) ProvesAtomBinding(binding AtomBinding) bool {
	if !observation.Valid || observation.Rows != 1 || !observation.Present || observation.Top || observation.seal != sealAtoms(observation.Atoms) {
		return false
	}
	for _, atom := range observation.Atoms {
		if !atom.Available() {
			return false
		}
		if binding.MatchesCertificate(atom) {
			return true
		}
	}
	return false
}

// sealAtoms is the scalar authentication of one observed atom projection. The
// low bit is set so the zero seal of an unfolded literal never collides with a
// folded one.
func sealAtoms(atoms []identity.ContentID) uint64 {
	writer := internalhash.NewWriter()
	writer.WriteUintDecimal(uint64(len(atoms)))
	for index := range atoms {
		_, _ = writer.Write(atoms[index][:])
	}
	return writer.Sum64() | 1
}

func EqualEffect(left, right EffectObservation) bool {
	if left.Rows != right.Rows || left.Present != right.Present || left.Top != right.Top || left.Valid != right.Valid || len(left.Atoms) != len(right.Atoms) {
		return false
	}
	for index := range left.Atoms {
		if left.Atoms[index] != right.Atoms[index] {
			return false
		}
	}
	return true
}

func FingerprintEffect(value EffectObservation) uint64 {
	hash := sha256.New()
	var header [8]byte
	binary.BigEndian.PutUint32(header[:4], value.Rows)
	if value.Present {
		header[4] = 1
	}
	if value.Top {
		header[5] = 1
	}
	if value.Valid {
		header[6] = 1
	}
	_, _ = hash.Write(header[:])
	for _, id := range value.Atoms {
		_, _ = hash.Write(id[:])
	}
	sum := hash.Sum(nil)
	return binary.BigEndian.Uint64(sum[:8])
}
