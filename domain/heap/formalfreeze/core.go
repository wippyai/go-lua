// Package formalfreeze consumes the exact Target freeze ownership row at an
// ordinary mounted call and projects it onto Heap's allocation roots.
//
// The package is deliberately a narrow consumer. Target remains the owner of
// FormalEffect rows, Pack remains the owner of mounted actual geometry, Value
// remains the owner of actual facts, and Heap remains the owner of allocation
// keys and freeze transitions. No Placement state is imported or retained.
package formalfreeze

import (
	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/internal/recentplan"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const formalFreezeInlineWidth = recentplan.InlineWidth

// freezeActualRoot is this derivation's one reading of an actual cell: the
// exact Recent allocation root that cell names, if it names one.
//
// A cell is not a value. It carries whether that coordinate holds one and
// whether the index names a cell at all, and what those mean together is the
// owner's judgment, not this rule's: AuthenticateFactorCell admits a present
// owner-fenced fact or the owner's own sparse Bottom and refuses everything
// else. An absent cell is therefore answered as Bottom - no exact root, and no
// route - while a cell the owner does not admit is malformed owner state that
// fails the whole relation.
//
// The three results are the root, whether the admitted fact names one, and
// whether the cell was admissible at all. Opaque references,
// Summary/Exact materializations, scalar alternatives and ambiguous unions all
// name no root rather than widening a freeze route.
func freezeActualRoot(values *valuedomain.Schema, actuals execution.SummaryVector[valuedomain.Value], ordinal int) (heap.Key, bool, bool) {
	if values == nil {
		return heap.Key{}, false, false
	}
	value, present, cellOK := actuals.At(ordinal)
	fact, admitted := values.AuthenticateFactorCell(value, present, cellOK)
	if !admitted {
		return heap.Key{}, false, false
	}
	root, rooted := values.ExactRecentAllocation(fact, present)
	return root, rooted, true
}

// freezeParamSet is the allocation-free representation of one target's exact
// Freeze selectors. The inline prefix covers ordinary declarations; unusually
// wide formal rows use an invocation-local overflow suffix.
type freezeParamSet struct {
	inline [formalFreezeInlineWidth]int
	extra  []int
	size   int
}

func (set freezeParamSet) count() int {
	if set.size < 0 {
		return 0
	}
	return set.size
}

func (set freezeParamSet) at(index int) (int, bool) {
	if index < 0 || index >= set.size {
		return 0, false
	}
	if index < len(set.inline) {
		return set.inline[index], true
	}
	extra := index - len(set.inline)
	if extra < 0 || extra >= len(set.extra) {
		return 0, false
	}
	return set.extra[extra], true
}

func (set *freezeParamSet) add(param int) bool {
	if set == nil || set.size < 0 || param < 0 {
		return false
	}
	position := 0
	for position < set.size {
		current, ok := set.at(position)
		if !ok {
			return false
		}
		switch {
		case current == param:
			return true
		case current > param:
			goto insert
		default:
			position++
		}
	}

insert:
	if set.size < len(set.inline) {
		for index := set.size; index > position; index-- {
			set.inline[index] = set.inline[index-1]
		}
		set.inline[position] = param
		set.size++
		return true
	}
	if position < len(set.inline) {
		carried := set.inline[len(set.inline)-1]
		for index := len(set.inline) - 1; index > position; index-- {
			set.inline[index] = set.inline[index-1]
		}
		set.inline[position] = param
		set.extra = append(set.extra, 0)
		copy(set.extra[1:], set.extra[:len(set.extra)-1])
		set.extra[0] = carried
	} else {
		extra := position - len(set.inline)
		if extra < 0 || extra > len(set.extra) {
			return false
		}
		set.extra = append(set.extra, 0)
		copy(set.extra[extra+1:], set.extra[extra:len(set.extra)-1])
		set.extra[extra] = param
	}
	set.size++
	return true
}

// freezeParamsForTarget returns every exact fixed formal actual a known
// target justifies. Formal rows are exact-only here: an open row, an invalid
// target, a body target, a missing Freeze row, or an out-of-range parameter
// produces no set. Non-Freeze formal rows belong to other consumers and are
// ignored. Resolved duplicate selectors (for example explicit last-actual and
// Param=-1 on a fixed call) are canonicalized once.
func freezeParamsForTarget(targetContract *contract.Contract, target calldomain.Target, actualCount int, runtimeTail bool) (freezeParamSet, bool, bool) {
	if targetContract == nil || actualCount < 0 {
		return freezeParamSet{}, false, false
	}
	operation, operationOK := target.Operation()
	if !operationOK || operation == 0 {
		return freezeParamSet{}, false, true
	}
	declared, declaredOK := targetContract.Operations.OperationAt(int(operation) - 1)
	if !declaredOK || declared != operation {
		return freezeParamSet{}, false, false
	}
	tail, tailOK := targetContract.Operations.FormalEffectTail(operation)
	if !tailOK {
		return freezeParamSet{}, false, false
	}
	// Unknown-open formal rows are not an exact ownership proof. They do not
	// widen a freeze operation; they simply have no route in this consumer.
	if tail != vocabulary.RowClosed {
		return freezeParamSet{}, false, true
	}
	var params freezeParamSet
	found := false
	for index := 0; index < targetContract.Operations.FormalEffectCount(operation); index++ {
		spec, specOK := targetContract.Operations.FormalEffectAt(operation, index)
		if !specOK {
			return freezeParamSet{}, false, false
		}
		if spec.Kind != vocabulary.FormalEffectFreeze {
			continue
		}
		resolved := int(spec.Param)
		if spec.Param == -1 {
			// The last actual is exact only when the mounted row has no runtime
			// suffix beyond its fixed prefix.
			if runtimeTail || actualCount == 0 {
				return freezeParamSet{}, false, true
			}
			resolved = actualCount - 1
		}
		if resolved < 0 || resolved >= actualCount {
			return freezeParamSet{}, false, true
		}
		if !params.add(resolved) {
			return freezeParamSet{}, false, false
		}
		found = true
	}
	return params, found, true
}
