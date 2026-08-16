package snapshot

import "github.com/wippyai/go-lua/analysis/identity"

// Axis identifies one published column: the schema that sealed it and the
// dense slot it occupies in the snapshots that schema produces. It never
// contains storage, so carrying an Axis costs a copy of an identity and a
// word, and holding one keeps no snapshot alive.
//
// K and V are the static claim about the column's contents. A read carries
// that claim to the snapshot, which either recovers a column built for
// exactly those types or fails closed.
type Axis[K comparable, V any] struct {
	SchemaID identity.ContentID
	Slot     uint32
}

// Available reports whether ax names a column of a sealed schema. The zero
// Axis names none.
func (ax Axis[K, V]) Available() bool { return ax.SchemaID.Available() }

// ReadStatus is the outcome of a snapshot read. Three of its values are read
// outcomes and carry the dependency meaning a causal reader records; the
// zero value is not an outcome at all, so a caller that ignores the status
// can never mistake a rejected read for an answer.
type ReadStatus uint8

const (
	// ReadInvalid reports that no read happened. The axis named another
	// schema, the slot was out of bounds, or the column was not built for
	// this key and value type. It is the zero value and produces no
	// dependency evidence.
	ReadInvalid ReadStatus = iota
	// ReadHit reports that the column stores a row for the key, and the
	// returned value is that row.
	ReadHit
	// ReadMiss reports that the column stores no row for the key and cannot
	// prove that none exists. It is ignorance, not absence.
	ReadMiss
	// ReadProvenAbsent reports that the column's sealed denominator covers
	// the key and the column stores no row for it, so the key's absence is
	// a published fact.
	ReadProvenAbsent
)

// Outcome reports whether status is a read outcome rather than a rejection.
func (status ReadStatus) Outcome() bool { return status != ReadInvalid }

// String returns the diagnostic name of status.
func (status ReadStatus) String() string {
	switch status {
	case ReadHit:
		return "hit"
	case ReadMiss:
		return "miss"
	case ReadProvenAbsent:
		return "proven-absent"
	default:
		return "invalid"
	}
}
