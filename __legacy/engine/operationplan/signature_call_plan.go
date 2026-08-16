package operationplan

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// WithSignatureCalls attaches resolved signature descriptors as an immutable,
// typed Plan sidecar. Equal descriptors are interned once; point cells retain
// only a compact one-based reference. A type hash selects candidates and full
// signature equality is the collision check.
func (p *Plan) WithSignatureCalls(input map[cfg.Point]SignatureCallOperation) *Plan {
	if p == nil {
		return nil
	}
	out := *p
	out.signatureRefs = make([]uint32, len(p.rows))
	out.signatures = make([]SignatureCallOperation, 0, len(input))
	buckets := make(map[uint64][]uint32, len(input))
	for rawPoint := 0; rawPoint < len(p.rows); rawPoint++ {
		point := cfg.Point(rawPoint)
		op, ok := input[point]
		if !ok || !op.valid() {
			continue
		}
		digest := typ.EqualityHash(op.signature.Type)
		var ref uint32
		for _, candidate := range buckets[digest] {
			if out.signatures[candidate-1].equal(op) {
				ref = candidate
				break
			}
		}
		if ref == 0 {
			out.signatures = append(out.signatures, op.clone())
			ref = uint32(len(out.signatures))
			buckets[digest] = append(buckets[digest], ref)
		}
		out.signatureRefs[rawPoint] = ref
	}
	return &out
}

// SignatureCallOperation returns an owned descriptor for point.
func (p *Plan) SignatureCallOperation(point cfg.Point) (SignatureCallOperation, bool) {
	if p == nil || uint64(point) >= uint64(len(p.signatureRefs)) {
		return SignatureCallOperation{}, false
	}
	ref := p.signatureRefs[point]
	if ref == 0 || int(ref) > len(p.signatures) {
		return SignatureCallOperation{}, false
	}
	return p.signatures[ref-1].clone(), true
}
