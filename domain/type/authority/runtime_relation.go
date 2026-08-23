package typeauthority

import (
	"errors"
	"math"
	"sync"

	"github.com/wippyai/go-lua/domain/type/subtype"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// sealClosedUniverse publishes the finite closed universe of the sealed rows:
// the dense positions a structural judgment may name. It is one linear pass
// over the rows, so admitting a declaration costs work proportional to its
// node count and never to the number of ordered pairs those nodes form.
func (b *runtimeBuilder) sealClosedUniverse() error {
	if b == nil || b.runtime == nil || len(b.runtime.rows) != len(b.construction) {
		return errors.New("typeauthority: malformed Runtime relation source")
	}
	runtime := b.runtime
	runtime.closedPositions = make([]int32, len(runtime.rows))
	runtime.closedRows = make([]uint32, 0, len(runtime.rows))
	for index := range runtime.rows {
		runtime.closedPositions[index] = -1
		if runtime.rows[index].scopedID.Available() {
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
	if len(runtime.closedRows) == 0 {
		return errors.New("typeauthority: empty Runtime relation universe")
	}
	runtime.sources = b.construction
	return nil
}

// runtimeRelation is the sealed subtype judgment of the closed universe.
// Runtime states no rule of its own: it asks the canonical checker about the
// retained construction values and remembers the answer for the ordered pair
// it was asked about.
//
// The relation is therefore paid per asked pair rather than per possible
// pair. Sealing a declaration decides nothing, so one deep declaration costs
// its node count and not the square of it, and no owner ever holds a
// quadratic matrix of judgments no consumer demanded.
type runtimeRelation struct {
	mutex     sync.Mutex
	prover    subtype.Batch
	memo      map[uint64]bool
	judgments int
}

func relationPair(left, right uint32) uint64 {
	return uint64(left)<<32 | uint64(right)
}

func (relation *runtimeRelation) decide(left, right uint32, leftValue, rightValue typ.Type) (bool, bool) {
	if leftValue == nil || rightValue == nil {
		return false, false
	}
	pair := relationPair(left, right)
	relation.mutex.Lock()
	defer relation.mutex.Unlock()
	if answer, known := relation.memo[pair]; known {
		return answer, true
	}
	// Every judgment starts from the prover's empty memo, so no coinductive
	// assumption of one pair is ever observed by another.
	answer := relation.prover.IsSubtype(leftValue, rightValue)
	if relation.memo == nil {
		relation.memo = make(map[uint64]bool)
	}
	relation.memo[pair] = answer
	relation.judgments++
	return answer, true
}

func (relation *runtimeRelation) judgmentCount() int {
	relation.mutex.Lock()
	defer relation.mutex.Unlock()
	return relation.judgments
}
