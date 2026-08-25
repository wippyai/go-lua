// Package publicationfreeze consumes Effect's sealed FreezeSeal publication
// receipts and projects only exact Recent allocation roots onto Heap. It owns
// no Placement state and does not reinterpret non-freeze publication rows.
package publicationfreeze

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/internal/recentplan"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const (
	inlineOperationCapacity = recentplan.InlineWidth
	inlineIDCapacity        = recentplan.InlineWidth
)

// operationGate is the exact Call projection for one mounted publication
// call. Opaque or open alternatives never authorize a strong Heap transition.
type operationGate struct {
	inline      [inlineOperationCapacity]vocabulary.Operation
	count       int
	overflow    []vocabulary.Operation
	opaque      bool
	unsupported bool
}

func (gate operationGate) admits(operation vocabulary.Operation) bool {
	if operation == 0 || gate.count < 0 {
		return false
	}
	inline := gate.count
	if inline > len(gate.inline) {
		inline = len(gate.inline)
	}
	for index := 0; index < inline; index++ {
		if gate.inline[index] == operation {
			return true
		}
	}
	for index := len(gate.inline); index < gate.count; index++ {
		overflow := index - len(gate.inline)
		if overflow < 0 || overflow >= len(gate.overflow) {
			return false
		}
		if gate.overflow[overflow] == operation {
			return true
		}
	}
	return false
}

func (gate operationGate) at(index int) (vocabulary.Operation, bool) {
	if index < 0 || index >= gate.count || gate.count < 0 {
		return 0, false
	}
	if index < len(gate.inline) {
		return gate.inline[index], gate.inline[index] != 0
	}
	overflow := index - len(gate.inline)
	if overflow < 0 || overflow >= len(gate.overflow) {
		return 0, false
	}
	operation := gate.overflow[overflow]
	return operation, operation != 0
}

func (gate *operationGate) add(operation vocabulary.Operation) bool {
	if gate == nil || operation == 0 || gate.count < 0 {
		return false
	}
	if gate.admits(operation) {
		return true
	}
	if gate.count < len(gate.inline) {
		gate.inline[gate.count] = operation
		gate.count++
		return true
	}
	gate.overflow = append(gate.overflow, operation)
	gate.count++
	return true
}

// prepareCall authenticates one published call's FreezeSeal rows out of the
// Effect publication directory and retains only valid FreezeSeal rows. The
// Call key is cached here because module/occurrence provenance is static
// after BindHot; hot selector and fold paths must not repeat that projection.
type contentIDBuffer struct {
	inline   [inlineIDCapacity]identity.ContentID
	count    int
	overflow []identity.ContentID
}

func (ids contentIDBuffer) at(index int) (identity.ContentID, bool) {
	if index < 0 || index >= ids.count || ids.count < 0 {
		return identity.ContentID{}, false
	}
	if index < len(ids.inline) {
		return ids.inline[index], true
	}
	overflow := index - len(ids.inline)
	if overflow < 0 || overflow >= len(ids.overflow) {
		return identity.ContentID{}, false
	}
	return ids.overflow[overflow], true
}

func (ids *contentIDBuffer) add(id identity.ContentID) bool {
	if ids == nil || ids.count < 0 || !id.Available() {
		return false
	}
	for index := 0; index < ids.count; index++ {
		prior, priorOK := ids.at(index)
		if !priorOK {
			return false
		}
		if prior == id {
			return false
		}
	}
	if ids.count < len(ids.inline) {
		ids.inline[ids.count] = id
		ids.count++
		return true
	}
	ids.overflow = append(ids.overflow, id)
	ids.count++
	return true
}

type route = recentplan.Route
type routePlan = recentplan.Plan

// exactRecentAllocation accepts only one owner-fenced Value atom carrying one
// Recent root allocation. Open, Top, Summary, scalar, and ambiguous unions do
// not authorize a strong freeze.
func exactRecentAllocation(values *valuedomain.Schema, fact valuedomain.Value, present bool) (heapdomain.Key, bool) {
	return values.ExactRecentAllocation(fact, present)
}

// planFor intersects the exact Recent-root route set justified by each known
// operation alternative. Any open/top Call, unsupported target, missing
// FreezeSeal row, open subject, or non-exact Value fact yields an empty valid
// plan rather than a strong Heap transition.
