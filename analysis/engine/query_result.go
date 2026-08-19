package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// frozenValue is one published, transitively immutable result value. It is
// never copied on the borrowed read path: a caller that needs an owned value
// asks the typed freezer for one through an explicit detachment.
type frozenValue interface {
	equal(frozenValue) bool
	fingerprint() uint64
	rowPresent() bool
}

type typedFrozenValue[R any] struct {
	value  R
	freeze FrozenResult[R]
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

func (value *typedFrozenValue[R]) rowPresent() bool {
	return value != nil && value.freeze.Present(value.value)
}

type queryResult struct {
	owner queryOwner
	key   composition.Key
	value frozenValue
}

type runtimeQuery interface {
	query() equation.Query
	queryOwner() queryOwner
	PublicationKey() (identity.ContentID, bool)
	materialize(*carrier.Work, carrier.State) (*queryResult, bool)
}

type queryOwner interface {
	validQueryOwner(*solverRuntime, equation.Query) bool
}
