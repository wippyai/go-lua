package flow

import programflow "github.com/wippyai/go-lua/analysis/program/flow"

type callRows struct{ rows []programflow.Call }

func (r *Rows) AppendCall(row programflow.Call) {
	if r != nil {
		r.calls.rows = append(r.calls.rows, row)
	}
}
