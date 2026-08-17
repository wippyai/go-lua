package flow

import programflow "github.com/wippyai/go-lua/analysis/program/flow"

type operatorRows struct {
	unaries  []programflow.Unary
	binaries []programflow.Binary
	selects  []programflow.Select
}

func (r *Rows) AppendUnary(row programflow.Unary) {
	if r != nil {
		r.operators.unaries = append(r.operators.unaries, row)
	}
}

func (r *Rows) UnaryAt(index int) (programflow.Unary, bool) {
	if r == nil || index < 0 || index >= len(r.operators.unaries) {
		return programflow.Unary{}, false
	}
	return r.operators.unaries[index], true
}

func (r *Rows) AppendBinary(row programflow.Binary) {
	if r != nil {
		r.operators.binaries = append(r.operators.binaries, row)
	}
}

func (r *Rows) AppendSelect(row programflow.Select) {
	if r != nil {
		r.operators.selects = append(r.operators.selects, row)
	}
}
