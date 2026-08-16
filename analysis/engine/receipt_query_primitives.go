package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

type frozenValue interface {
	clone() frozenValue
	equal(frozenValue) bool
	fingerprint() uint64
}

type typedFrozenValue[R any] struct {
	value  R
	freeze FrozenResult[R]
}

func (value *typedFrozenValue[R]) clone() frozenValue {
	if value == nil {
		return nil
	}
	return &typedFrozenValue[R]{value: value.freeze.Clone(value.value), freeze: value.freeze}
}
func (value *typedFrozenValue[R]) equal(other frozenValue) bool {
	right, ok := other.(*typedFrozenValue[R])
	return ok && value != nil && right != nil && value.freeze.Equal(value.value, right.value)
}
func (value *typedFrozenValue[R]) fingerprint() uint64 {
	if value == nil {
		return 0
	}
	return value.freeze.Fingerprint(value.value)
}

type queryResult struct {
	owner queryOwner
	key   composition.Key
	value frozenValue
}

func (result *queryResult) clone() *queryResult {
	if result == nil || result.owner == nil || result.value == nil {
		return nil
	}
	return &queryResult{owner: result.owner, key: result.key, value: result.value.clone()}
}

type runtimeQuery interface {
	query() equation.Query
	queryOwner() queryOwner
	materialize(*carrier.Work, carrier.State) (*queryResult, bool)
}

type queryOwner interface {
	validQueryOwner(*solverRuntime, equation.Query) bool
}
