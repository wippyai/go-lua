package typ

import (
	"math"
	"sync"
	"sync/atomic"
)

// A canonical form's validity is a pure property of one pair: its canonical
// bytes, and the external formal count they are read against. The bytes are the
// complete structural identity of the form - that is what makes them canonical
// - and every law the scoped validators apply is a function of that identity.
// A form this process has already admitted therefore cannot become invalid, and
// the records below are that admission, kept once per process instead of
// re-derived per artifact, per solve, and per fixture.
//
// Only a completed validation admits a form. A form that has never been
// admitted is judged in full on every sighting, so an invalid form neither
// enters the record nor is carried into it by being presented after valid ones.
//
// The wire record and the graph record are deliberately separate. The wire
// validator proves that a byte string is the unique canonical spelling of a
// lawful form; the graph validator proves that a discovered source graph obeys
// the scoped scope laws and the static generic recurrence law. Neither property
// is inferred from the other, so neither record answers for the other.
var (
	canonicalValidatedWireForms  canonicalValidatedFormRecord
	canonicalValidatedGraphForms canonicalValidatedFormRecord
)

// CanonicalFormRecordCensus is one validated-form record's state. Forms is the
// distinct forms it holds, Sightings the questions asked of it, and Misses the
// questions it could not answer and which were therefore derived in full.
type CanonicalFormRecordCensus struct {
	Forms     uint64
	Sightings uint64
	Misses    uint64
}

// CanonicalFormCensus reports both validated-form records. It is a structural
// counter for the per-file analysis budget: a workload whose misses track its
// sightings is deriving one full validation per occurrence of a form, and the
// distinct form count is what states whether an exact record is the right shape
// for that workload.
func CanonicalFormCensus() (wire, graph CanonicalFormRecordCensus) {
	return canonicalValidatedWireForms.census(), canonicalValidatedGraphForms.census()
}

func (r *canonicalValidatedFormRecord) census() CanonicalFormRecordCensus {
	return CanonicalFormRecordCensus{
		Forms:     r.admitted.Load(),
		Sightings: r.sightings.Load(),
		Misses:    r.misses.Load(),
	}
}

// canonicalValidatedFormShards keeps concurrent solves off one lock. A shard is
// selected from the form's own bytes, so the same form always reaches the same
// shard without hashing the payload.
const canonicalValidatedFormShards = 16

type canonicalValidatedFormRecord struct {
	shards [canonicalValidatedFormShards]canonicalValidatedFormShard
	// admitted counts the distinct forms this record holds; sightings counts the
	// questions asked of it and misses the questions it could not answer. Read
	// together they state one workload's form distribution and how much of its
	// validation is re-derivation, which is what makes the record's shape - and
	// its effect - measurable rather than assumed.
	admitted  atomic.Uint64
	sightings atomic.Uint64
	misses    atomic.Uint64
}

// forms is keyed by the canonical bytes themselves rather than by a digest of
// them, so admission needs no collision argument. The outer key is the external
// formal count: the same bytes read against a different scope are a different
// question.
type canonicalValidatedFormShard struct {
	mutex sync.RWMutex
	forms map[uint32]map[string]struct{}
}

func canonicalValidatedFormShardIndex(encoded []byte) int {
	mixed := uint32(len(encoded))*2654435761 + uint32(encoded[len(encoded)-1])
	return int(mixed % canonicalValidatedFormShards)
}

// admits reports whether this record already holds the form. The lookup reads
// the bytes in place: it allocates nothing, so asking is cheap enough to ask
// before every validation.
func (r *canonicalValidatedFormRecord) admits(encoded []byte, externalFormalCount int) bool {
	if len(encoded) == 0 || externalFormalCount < 0 || uint64(externalFormalCount) > math.MaxUint32 {
		return false
	}
	r.sightings.Add(1)
	shard := &r.shards[canonicalValidatedFormShardIndex(encoded)]
	shard.mutex.RLock()
	var held bool
	if forms, scoped := shard.forms[uint32(externalFormalCount)]; scoped {
		_, held = forms[string(encoded)]
	}
	shard.mutex.RUnlock()
	if !held {
		r.misses.Add(1)
	}
	return held
}

// admit records a form its own validator has just accepted in full.
func (r *canonicalValidatedFormRecord) admit(encoded []byte, externalFormalCount int) {
	if len(encoded) == 0 || externalFormalCount < 0 || uint64(externalFormalCount) > math.MaxUint32 {
		return
	}
	scope := uint32(externalFormalCount)
	shard := &r.shards[canonicalValidatedFormShardIndex(encoded)]
	shard.mutex.Lock()
	defer shard.mutex.Unlock()
	if shard.forms == nil {
		shard.forms = make(map[uint32]map[string]struct{})
	}
	forms, scoped := shard.forms[scope]
	if !scoped {
		forms = make(map[string]struct{})
		shard.forms[scope] = forms
	}
	if _, held := forms[string(encoded)]; held {
		return
	}
	forms[string(encoded)] = struct{}{}
	r.admitted.Add(1)
}
