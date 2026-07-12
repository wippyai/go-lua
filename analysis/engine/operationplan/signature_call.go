package operationplan

import "github.com/wippyai/go-lua/analysis/module/signature"

// SignatureCallOperation is the immutable, already-resolved producer
// descriptor for one call site. It carries no name: identity resolution and
// signature lookup happen once at body preparation, and all compilers consume
// the same owned type/effect carrier.
type SignatureCallOperation struct {
	signature signature.Function
}

func NewSignatureCallOperation(sig signature.Function) (SignatureCallOperation, bool) {
	if sig.Type == nil {
		return SignatureCallOperation{}, false
	}
	return SignatureCallOperation{signature: sig.Clone()}, true
}

func (o SignatureCallOperation) Signature() signature.Function { return o.signature.Clone() }
func (o SignatureCallOperation) valid() bool                   { return o.signature.Type != nil }
func (o SignatureCallOperation) clone() SignatureCallOperation {
	return SignatureCallOperation{signature: o.signature.Clone()}
}
func (o SignatureCallOperation) equal(other SignatureCallOperation) bool {
	return o.signature.Equals(other.signature)
}
