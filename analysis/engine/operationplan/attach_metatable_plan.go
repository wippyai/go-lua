package operationplan

import "github.com/wippyai/go-lua/analysis/ir/cfg"

// WithAttachMetatables attaches the sparse, preparation-owned metatable
// operations to an immutable plan. Equal operand pairs share one descriptor.
func (p *Plan) WithAttachMetatables(input map[cfg.Point]AttachMetatableOperation) *Plan {
	if p == nil {
		return nil
	}
	out := *p
	out.attachMetatableRefs = make([]uint32, len(p.rows))
	out.attachMetatables = make([]AttachMetatableOperation, 0, len(input))
	for rawPoint := 0; rawPoint < len(p.rows); rawPoint++ {
		point := cfg.Point(rawPoint)
		op, ok := input[point]
		if !ok || !op.valid() {
			continue
		}
		var ref uint32
		for index, existing := range out.attachMetatables {
			if existing.equal(op) {
				ref = uint32(index + 1)
				break
			}
		}
		if ref == 0 {
			out.attachMetatables = append(out.attachMetatables, op)
			ref = uint32(len(out.attachMetatables))
		}
		out.attachMetatableRefs[rawPoint] = ref
	}
	return &out
}

// AttachMetatableOperation returns the typed operation at point.
func (p *Plan) AttachMetatableOperation(point cfg.Point) (AttachMetatableOperation, bool) {
	if p == nil || uint64(point) >= uint64(len(p.attachMetatableRefs)) {
		return AttachMetatableOperation{}, false
	}
	ref := p.attachMetatableRefs[point]
	if ref == 0 || int(ref) > len(p.attachMetatables) {
		return AttachMetatableOperation{}, false
	}
	return p.attachMetatables[ref-1], true
}
