package body

import (
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// BranchAlgebra exposes the same immutable branch vocabulary consumed by the
// concrete edge executor. Symbolic/differential tooling can inspect capability
// without reconstructing facts from source syntax or result state.
func (r *Result) BranchAlgebra(point cfg.Point) factapply.BranchAlgebra {
	if r == nil {
		return factapply.BranchAlgebra{}
	}
	return factapply.NewBranchAlgebra(r.facts, point)
}
