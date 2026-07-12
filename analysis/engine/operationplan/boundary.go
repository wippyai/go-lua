package operationplan

import "github.com/wippyai/go-lua/analysis/symbol"

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

func (p *Plan) BoundaryParams() []symbol.ID {
	if p == nil {
		return nil
	}
	return append([]symbol.ID(nil), p.boundaryParams...)
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
