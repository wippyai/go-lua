package operationplan

import (
	"slices"

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
	out.boundaryParamsValid = false
	seen := make(map[symbol.ID]bool, len(params)+len(p.boundaryCaptures)+len(p.boundaryGlobals))
	for _, capture := range p.boundaryCaptures {
		seen[capture] = true
	}
	for _, global := range p.boundaryGlobals {
		seen[global] = true
	}
	owned := make([]symbol.ID, 0, len(params))
	for _, param := range params {
		if param == 0 || seen[param] {
			return &out
		}
		seen[param] = true
		owned = append(owned, param)
	}
	out.boundaryParams = owned
	out.boundaryParamsValid = true
	return &out
}

// BoundaryParamsValid distinguishes an exact empty parameter boundary from
// one rejected because its namespaces were malformed.
func (p *Plan) BoundaryParamsValid() bool {
	return p != nil && p.boundaryParamsValid
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
	if !p.boundaryParamsValid {
		return &out
	}
	seen := make(map[symbol.ID]bool, len(p.boundaryParams)+len(captures)+len(p.boundaryGlobals))
	for _, param := range p.boundaryParams {
		seen[param] = true
	}
	for _, global := range p.boundaryGlobals {
		seen[global] = true
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

// WithBoundaryGlobals returns a plan owning the exact ordered global symbols
// read directly by the body. GlobalBoundary remains the authority which may
// later certify those roots immutable and instantiate them; this plan field is
// only its canonical symbol-sorted dense order. Zero, duplicate, parameter,
// and capture overlaps fail closed rather than publishing an ambiguous
// namespace.
func (p *Plan) WithBoundaryGlobals(globals []symbol.ID) *Plan {
	if p == nil {
		return nil
	}
	out := *p
	out.boundaryGlobals = nil
	out.boundaryGlobalsValid = false
	out.boundaryGlobalContracts = nil
	if !p.boundaryParamsValid || !p.boundaryCapturesValid {
		return &out
	}
	seen := make(map[symbol.ID]bool, len(p.boundaryParams)+len(p.boundaryCaptures)+len(globals))
	for _, param := range p.boundaryParams {
		seen[param] = true
	}
	for _, capture := range p.boundaryCaptures {
		seen[capture] = true
	}
	owned := make([]symbol.ID, 0, len(globals))
	for _, global := range globals {
		if global == 0 || seen[global] {
			return &out
		}
		seen[global] = true
		owned = append(owned, global)
	}
	// GlobalBoundary seals descriptors by Symbol. Use the same dense order here
	// so RootGlobal indices have one canonical meaning from plan to artifact.
	slices.Sort(owned)
	out.boundaryGlobals = owned
	out.boundaryGlobalsValid = true
	return &out
}

// WithBoundaryGlobalContracts binds each ordered RootGlobal to the exact
// abstract value supplied by the prepared body's immutable type environment.
// A width mismatch clears the complete vector fail-closed.
func (p *Plan) WithBoundaryGlobalContracts(values []product.Value) *Plan {
	if p == nil {
		return nil
	}
	out := *p
	out.boundaryGlobalContracts = nil
	if !p.boundaryGlobalsValid || len(values) != len(p.boundaryGlobals) {
		return &out
	}
	out.boundaryGlobalContracts = append([]product.Value(nil), values...)
	return &out
}

// BoundaryGlobalContracts returns the immutable dense RootGlobal value vector.
func (p *Plan) BoundaryGlobalContracts() []product.Value {
	if p == nil {
		return nil
	}
	return append([]product.Value(nil), p.boundaryGlobalContracts...)
}

// BoundaryGlobalsValid distinguishes an exact empty global-read census from
// one rejected because its namespaces were malformed.
func (p *Plan) BoundaryGlobalsValid() bool {
	return p != nil && p.boundaryGlobalsValid
}

// BoundaryGlobals returns an immutable snapshot in canonical symbol order,
// matching GlobalBoundary's dense RootGlobal authority.
func (p *Plan) BoundaryGlobals() []symbol.ID {
	if p == nil {
		return nil
	}
	return append([]symbol.ID(nil), p.boundaryGlobals...)
}

func (p *Plan) BoundaryGlobalIndex(target symbol.ID) (int, bool) {
	if p == nil || target == 0 {
		return 0, false
	}
	index, ok := slices.BinarySearch(p.boundaryGlobals, target)
	return index, ok
}
