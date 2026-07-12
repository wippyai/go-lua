package operationplan

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// WithBoundaryParams returns a plan owning the ordered lexical parameter
// symbols used by symbolic boundary roots. Duplicate/zero symbols fail closed
// by clearing the boundary rather than publishing ambiguous bindings.
func (p *Plan) WithBoundaryParams(params []symbol.ID) *Plan {
	if p == nil {
		return nil
	}
	out := *p
	out.boundaryParams = nil
	seen := make(map[symbol.ID]bool, len(params))
	owned := make([]symbol.ID, 0, len(params))
	for _, param := range params {
		if param == 0 || seen[param] {
			return &out
		}
		seen[param] = true
		owned = append(owned, param)
	}
	out.boundaryParams = owned
	return &out
}

// WithBoundaryParamContracts binds the ordered declared parameter contracts
// to BoundaryParams. A width mismatch clears the contracts fail-closed.
func (p *Plan) WithBoundaryParamContracts(values []product.Value) *Plan {
	if p == nil {
		return nil
	}
	out := *p
	out.boundaryParamContracts = nil
	if len(values) != len(p.boundaryParams) {
		return &out
	}
	out.boundaryParamContracts = append([]product.Value(nil), values...)
	return &out
}

func (p *Plan) BoundaryParamContracts() []product.Value {
	if p == nil {
		return nil
	}
	return append([]product.Value(nil), p.boundaryParamContracts...)
}

// WithBoundaryCaptures returns a plan owning the ordered lexical capture
// symbols used by symbolic capture roots. Duplicate/zero symbols, or a symbol
// that is also a parameter, fail closed by clearing the capture boundary.
func (p *Plan) WithBoundaryCaptures(captures []symbol.ID) *Plan {
	if p == nil {
		return nil
	}
	out := *p
	out.boundaryCaptures = nil
	out.boundaryCapturesValid = false
	seen := make(map[symbol.ID]bool, len(p.boundaryParams)+len(captures))
	for _, param := range p.boundaryParams {
		seen[param] = true
	}
	owned := make([]symbol.ID, 0, len(captures))
	for _, capture := range captures {
		if capture == 0 || seen[capture] {
			return &out
		}
		seen[capture] = true
		owned = append(owned, capture)
	}
	out.boundaryCaptures = owned
	out.boundaryCapturesValid = true
	return &out
}

// BoundaryCapturesValid distinguishes an intentionally empty capture boundary
// from one cleared after invalid duplicate/zero input.
func (p *Plan) BoundaryCapturesValid() bool {
	return p != nil && p.boundaryCapturesValid
}

// WithBoundaryReturns returns a plan owning the ordered declared return
// contracts used by canonical Summary projection.
func (p *Plan) WithBoundaryReturns(values []product.Value) *Plan {
	if p == nil {
		return nil
	}
	out := *p
	out.boundaryReturns = append([]product.Value(nil), values...)
	return &out
}

// BoundaryReturns returns an immutable snapshot of declared return contracts.
func (p *Plan) BoundaryReturns() []product.Value {
	if p == nil {
		return nil
	}
	return append([]product.Value(nil), p.boundaryReturns...)
}

func (p *Plan) BoundaryParams() []symbol.ID {
	if p == nil {
		return nil
	}
	return append([]symbol.ID(nil), p.boundaryParams...)
}

// BoundaryCaptures returns an immutable snapshot of the ordered capture
// symbols. Order is binder first-use order and therefore deterministic.
func (p *Plan) BoundaryCaptures() []symbol.ID {
	if p == nil {
		return nil
	}
	return append([]symbol.ID(nil), p.boundaryCaptures...)
}

func (p *Plan) BoundaryParamIndex(target symbol.ID) (int, bool) {
	if p == nil || target == 0 {
		return 0, false
	}
	for index, param := range p.boundaryParams {
		if param == target {
			return index, true
		}
	}
	return 0, false
}

func (p *Plan) BoundaryCaptureIndex(target symbol.ID) (int, bool) {
	if p == nil || target == 0 {
		return 0, false
	}
	for index, capture := range p.boundaryCaptures {
		if capture == target {
			return index, true
		}
	}
	return 0, false
}
