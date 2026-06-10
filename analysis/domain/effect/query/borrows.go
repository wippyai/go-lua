package query

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
)

// OnlyBorrows reports whether r contains an ownership borrow label without an
// ownership store or mutation.Mutate label.
func OnlyBorrows(r effect.Row) bool {
	return ownership.HasBorrow(r) && !ownership.HasStore(r) && !r.Has(func(l effect.Label) bool {
		_, ok := l.(mutation.Mutate)
		return ok
	})
}
