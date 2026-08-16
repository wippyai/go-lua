// Package query owns the detached result contracts for the registered query
// families. It owns only detached domain result types, so both the schema
// owner and top-level hot binding can use the exact same contracts.
package query

import (
	"crypto/sha256"
	"encoding/binary"

	effectfactor "github.com/wippyai/go-lua/analysis/domain/effect/factor"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/identity"
)

// ValueSummaryObservation is the detached result of the Value summary query.
// Fields are exported solely so the hot projector in analysis can populate
// the exact type declared by the cold schema owner.
type ValueSummaryObservation struct {
	Values  []valuedomain.Value
	Present []bool
	Rows    uint32
	Valid   bool
}

// EffectObservation is the detached result of one exact Effect query.
type EffectObservation struct {
	Atoms        []identity.ContentID
	Rows         uint32
	Present      bool
	Top          bool
	Valid        bool
	joined       effectfactor.Value
	certificates []identity.ContentID
}

// BeginEffect starts the fold state for the exact Effect query. The joined
// value is retained only as an internal accumulator; detached observations
// continue to expose the stable atom/presence result surface.
func BeginEffect(algebra *effectfactor.Algebra) EffectObservation {
	return EffectObservation{Valid: algebra != nil && algebra.Valid()}
}

// AccumulateEffect joins one guarded Effect row into the detached result. The
// opaque algebra value lets the fold preserve joins without rebuilding atoms
// from portable IDs or retaining a mutable query-side map.
func AccumulateEffect(algebra *effectfactor.Algebra, result EffectObservation, value effectfactor.Value, present, available bool) (EffectObservation, bool) {
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
	result.certificates = nil
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
			result.certificates = append(result.certificates, id)
		}
	}
	return result, true
}

func CloneValueSummary(input ValueSummaryObservation) ValueSummaryObservation {
	input.Values = append([]valuedomain.Value(nil), input.Values...)
	input.Present = append([]bool(nil), input.Present...)
	return input
}

func EqualValueSummary(schema *valuedomain.Schema, left, right ValueSummaryObservation) bool {
	if schema == nil || left.Valid != right.Valid || left.Rows != right.Rows || len(left.Values) != len(right.Values) || len(left.Present) != len(right.Present) {
		return false
	}
	for index := range left.Values {
		if left.Present[index] != right.Present[index] || left.Present[index] && !schema.Equal(left.Values[index], right.Values[index]) {
			return false
		}
	}
	return true
}

func FingerprintValueSummary(schema *valuedomain.Schema, value ValueSummaryObservation) uint64 {
	if schema == nil {
		return 0
	}
	result := uint64(value.Rows) << 32
	for index := range value.Values {
		result ^= uint64(index+1) * 0x9e3779b97f4a7c15
		if index < len(value.Present) && value.Present[index] {
			result ^= schema.Fingerprint(value.Values[index])
		}
	}
	if value.Valid {
		result ^= 1 << 63
	}
	return result
}

func CloneEffect(input EffectObservation) EffectObservation {
	input.Atoms = append([]identity.ContentID(nil), input.Atoms...)
	input.certificates = append([]identity.ContentID(nil), input.certificates...)
	input.joined = effectfactor.Value{}
	return input
}

// ProvesAtomBinding is the only typed membership bridge from a detached
// Effect observation to an already-issued beta binding. It never accepts an
// atom ID from its caller. The private certificates authenticate the public
// atom projection after freezing, so a manually shaped observation cannot be
// used as a runtime publication proof.
func (observation EffectObservation) ProvesAtomBinding(binding effectfactor.AtomBinding) bool {
	if !observation.Valid || observation.Rows != 1 || !observation.Present || observation.Top || len(observation.Atoms) != len(observation.certificates) {
		return false
	}
	matched := false
	for index, certificate := range observation.certificates {
		if !certificate.Available() || observation.Atoms[index] != certificate {
			return false
		}
		if binding.MatchesCertificate(certificate) {
			matched = true
		}
	}
	return matched
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
