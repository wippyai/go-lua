package typeauthority

import (
	"crypto/sha256"
	"errors"
	"math"
	"sync"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/type/subtype"
)

// sealSubtypeRelation materializes the finite subtype relation of the sealed
// closed universe once, from the canonical checker, while the construction
// graphs are still owned by the builder. Runtime therefore holds no second
// structural proof engine: after this pass every judgment is one bit read out
// of a packed row.
//
// The relation is a pure function of the sealed rows, so it is deliberately
// not part of Runtime's content identity: two Runtimes with equal rows
// materialize the same relation.
//
// That purity is also what makes the relation derivable once per universe
// rather than once per Runtime. Every closed row carries its complete
// canonical structural identity, so the ordered canonical bytes of the closed
// rows are the complete input identity of the relation, and the memo below is
// keyed on exactly that identity. Two structurally equal universes reached
// through unrelated Runtimes therefore share one materialization.
func (b *runtimeBuilder) sealSubtypeRelation() error {
	if b == nil || b.runtime == nil || len(b.runtime.rows) != len(b.construction) {
		return errors.New("typeauthority: malformed Runtime relation source")
	}
	runtime := b.runtime
	runtime.closedPositions = make([]int32, len(runtime.rows))
	runtime.closedRows = make([]uint32, 0, len(runtime.rows))
	for index := range runtime.rows {
		runtime.closedPositions[index] = -1
		if !runtime.rows[index].closed {
			continue
		}
		if b.construction[index] == nil {
			return errors.New("typeauthority: closed Runtime row lacks a relation source graph")
		}
		if uint64(len(runtime.closedRows)) >= uint64(math.MaxInt32) {
			return errors.New("typeauthority: Runtime relation universe overflow")
		}
		runtime.closedPositions[index] = int32(len(runtime.closedRows))
		runtime.closedRows = append(runtime.closedRows, uint32(index+1))
	}
	count := len(runtime.closedRows)
	if count == 0 {
		return errors.New("typeauthority: empty Runtime relation universe")
	}
	runtime.subtypeStride = (count + 63) / 64
	universe, universeErr := runtimeRelationUniverseID(runtime)
	if universeErr != nil {
		return universeErr
	}
	runtimeRelationSeals.Add(1)
	if bits, materialized := loadRuntimeRelation(universe); materialized {
		runtime.subtypeBits = bits
		return nil
	}
	bits := make([]uint64, count*runtime.subtypeStride)
	// One prover serves the whole matrix: each ordered pair still starts from
	// an empty memo, so no coinductive assumption crosses a judgment.
	var prover subtype.Batch
	for leftPosition, leftRow := range runtime.closedRows {
		left := b.construction[leftRow-1]
		base := leftPosition * runtime.subtypeStride
		for rightPosition, rightRow := range runtime.closedRows {
			// The canonical prover memoizes under active coinductive
			// assumptions, so its table belongs to exactly one top-level
			// judgment and is cleared between pairs.
			if !prover.IsSubtype(left, b.construction[rightRow-1]) {
				continue
			}
			bits[base+rightPosition>>6] |= 1 << (uint(rightPosition) & 63)
		}
	}
	runtimeRelationJudgedPairs.Add(uint64(count) * uint64(count))
	runtimeRelationMaterializations.Add(1)
	runtime.subtypeBits = storeRuntimeRelation(universe, bits)
	return nil
}

// runtimeRelationUniverseID is the complete input identity of one closed
// universe's subtype relation: the ordered canonical bytes of its closed rows.
// It reads nothing address-shaped, so two identical universes produce one key.
func runtimeRelationUniverseID(runtime *Runtime) (identity.ContentID, error) {
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.typeauthority.runtime/subtype-relation\x00\x01"))
	writeRuntimeWord(hash, uint64(len(runtime.closedRows)))
	for _, row := range runtime.closedRows {
		encoded := runtime.rows[row-1].encoded
		if len(encoded) == 0 {
			return identity.ContentID{}, errors.New("typeauthority: closed Runtime row lacks canonical relation identity")
		}
		writeRuntimeWord(hash, uint64(len(encoded)))
		_, _ = hash.Write(encoded)
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	if !id.Available() {
		return identity.ContentID{}, errors.New("typeauthority: unavailable Runtime relation identity")
	}
	return id, nil
}

// runtimeRelationJudgedPairs counts the ordered pairs this process has put to
// the canonical prover. A memoized universe adds nothing to it, which is what
// makes the memo observable to the relation laws without instrumenting the
// judgment loop itself.
var runtimeRelationJudgedPairs atomic.Uint64

// runtimeRelationSeals counts the seals that asked for a relation. Read
// against runtimeRelationMaterializations it states how many distinct closed
// universes a workload actually contains.
var runtimeRelationSeals atomic.Uint64

// runtimeRelationMaterializations counts the universes this process built.
var runtimeRelationMaterializations atomic.Uint64

// RuntimeRelationCensus reports the relation memo's state: the seals that asked
// for a relation, the distinct closed universes actually materialized, and the
// ordered pairs put to the canonical prover. It is a structural counter for the
// per-file analysis budget, and it is what states how much of a workload's
// closed universe is shared rather than per-Runtime.
func RuntimeRelationCensus() (seals, materializations, judgedPairs uint64) {
	return runtimeRelationSeals.Load(), runtimeRelationMaterializations.Load(), runtimeRelationJudgedPairs.Load()
}

// The relation memo is sharded by its own key so concurrent solves over
// distinct universes do not serialize on one lock. Entries are immutable
// packed bitsets published under their content identity: a reader takes a
// read lock, reads one map entry, and allocates nothing.
const runtimeRelationShards = 16

type runtimeRelationShard struct {
	mutex     sync.RWMutex
	relations map[identity.ContentID][]uint64
}

var runtimeRelationMemo [runtimeRelationShards]runtimeRelationShard

func runtimeRelationShardFor(universe identity.ContentID) *runtimeRelationShard {
	return &runtimeRelationMemo[universe[0]%runtimeRelationShards]
}

func loadRuntimeRelation(universe identity.ContentID) ([]uint64, bool) {
	shard := runtimeRelationShardFor(universe)
	shard.mutex.RLock()
	bits, materialized := shard.relations[universe]
	shard.mutex.RUnlock()
	return bits, materialized
}

// storeRuntimeRelation publishes one materialization and returns the published
// bitset. A race between two seals of one universe produces byte-identical
// bitsets, and returning the stored one keeps a single copy live.
func storeRuntimeRelation(universe identity.ContentID, bits []uint64) []uint64 {
	shard := runtimeRelationShardFor(universe)
	shard.mutex.Lock()
	defer shard.mutex.Unlock()
	if existing, materialized := shard.relations[universe]; materialized {
		return existing
	}
	if shard.relations == nil {
		shard.relations = make(map[identity.ContentID][]uint64)
	}
	shard.relations[universe] = bits
	return bits
}
