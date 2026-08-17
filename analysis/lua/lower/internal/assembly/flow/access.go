package flow

import programflow "github.com/wippyai/go-lua/analysis/program/flow"

type accessRows struct {
	exact   []programflow.ExactLens
	dynamic []programflow.DynamicLens
}

func (r *Rows) AppendExactLens(row programflow.ExactLens) {
	if r != nil {
		r.access.exact = append(r.access.exact, row)
	}
}

func (r *Rows) AppendDynamicLens(row programflow.DynamicLens) {
	if r != nil {
		r.access.dynamic = append(r.access.dynamic, row)
	}
}
