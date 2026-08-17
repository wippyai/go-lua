package flow

import programflow "github.com/wippyai/go-lua/analysis/program/flow"

type operandRows struct {
	claims     []programflow.ValueClaim
	typeValues []programflow.TypeValue
}

func (r *Rows) AppendClaim(row programflow.ValueClaim) {
	if r != nil {
		r.operands.claims = append(r.operands.claims, row)
	}
}

func (r *Rows) ClaimAt(index int) (programflow.ValueClaim, bool) {
	if r == nil || index < 0 || index >= len(r.operands.claims) {
		return programflow.ValueClaim{}, false
	}
	return r.operands.claims[index], true
}

func (r *Rows) AppendTypeValue(row programflow.TypeValue) {
	if r != nil {
		r.operands.typeValues = append(r.operands.typeValues, row)
	}
}
