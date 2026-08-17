package flow

import (
	programflow "github.com/wippyai/go-lua/analysis/program/flow"
)

type functionRows struct {
	rows     []programflow.Function
	captures []programflow.Capture
}

func (r *Rows) AppendFunction(row programflow.Function) {
	if r != nil {
		r.functions.rows = append(r.functions.rows, row)
	}
}

func (r *Rows) FunctionAt(index int) (programflow.Function, bool) {
	if r == nil || index < 0 || index >= len(r.functions.rows) {
		return programflow.Function{}, false
	}
	return r.functions.rows[index], true
}

func (r *Rows) SetFunction(index int, row programflow.Function) bool {
	if r == nil || index < 0 || index >= len(r.functions.rows) {
		return false
	}
	r.functions.rows[index] = row
	return true
}

func (r *Rows) AppendCaptures(captures []programflow.Capture) (programflow.Range, bool) {
	if r == nil {
		return programflow.Range{}, false
	}
	result, ok := rangeFor(len(r.functions.captures), len(captures))
	if !ok {
		return programflow.Range{}, false
	}
	r.functions.captures = append(r.functions.captures, captures...)
	return result, true
}
