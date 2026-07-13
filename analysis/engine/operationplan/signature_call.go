package operationplan

import (
	"context"

	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturecontent"
)

// SignatureCallOperation is the immutable, already-resolved producer
// descriptor for one call site. It carries no name: identity resolution and
// signature lookup happen once at body preparation, and all compilers consume
// the same owned type/effect carrier.
type SignatureCallOperation struct {
	signature           signature.Function
	contentID           signature.ContentID
	allocationContentID signature.ContentID
	intrinsic           signature.Intrinsic
}

// NewSignatureIntrinsicCallOperation seals a signature together with a
// binding-resolved semantic intrinsic. The intrinsic is immutable operation
// content and is intentionally independent from signature spelling.
func NewSignatureIntrinsicCallOperation(sig signature.Function, intrinsic signature.Intrinsic) (SignatureCallOperation, bool) {
	return NewSignatureIntrinsicCallOperationContext(context.Background(), sig, intrinsic)
}

func NewSignatureIntrinsicCallOperationContext(ctx context.Context, sig signature.Function, intrinsic signature.Intrinsic) (SignatureCallOperation, bool) {
	if !intrinsic.Valid() {
		return SignatureCallOperation{}, false
	}
	op, ok := NewSignatureCallOperationContext(ctx, sig)
	if !ok {
		return SignatureCallOperation{}, false
	}
	op.intrinsic = intrinsic
	return op, true
}

func NewSignatureCallOperation(sig signature.Function) (SignatureCallOperation, bool) {
	return NewSignatureCallOperationContext(context.Background(), sig)
}

// NewSignatureCallOperationContext seals an owned call descriptor together
// with its immutable semantic identity. Canonicalization failure is fail-closed:
// a descriptor without exact identity is never admitted to an operation plan.
func NewSignatureCallOperationContext(ctx context.Context, sig signature.Function) (SignatureCallOperation, bool) {
	if sig.Type == nil {
		return SignatureCallOperation{}, false
	}
	owned := sig.Clone()
	contentID, err := signaturecontent.Derive(ctx, owned)
	if err != nil || !contentID.Available() {
		return SignatureCallOperation{}, false
	}
	allocationContentID, err := signaturecontent.DeriveAllocationTemplates(ctx, owned)
	if err != nil {
		return SignatureCallOperation{}, false
	}
	return SignatureCallOperation{
		signature:           owned,
		contentID:           contentID,
		allocationContentID: allocationContentID,
	}, true
}

func (o SignatureCallOperation) Signature() signature.Function  { return o.signature.Clone() }
func (o SignatureCallOperation) ContentID() signature.ContentID { return o.contentID }
func (o SignatureCallOperation) AllocationContentID() signature.ContentID {
	return o.allocationContentID
}
func (o SignatureCallOperation) Intrinsic() (signature.Intrinsic, bool) {
	return o.intrinsic, o.intrinsic.Valid()
}
func (o SignatureCallOperation) valid() bool {
	return o.signature.Type != nil && o.contentID.Available() &&
		(o.intrinsic == signature.IntrinsicNone || o.intrinsic.Valid())
}
func (o SignatureCallOperation) clone() SignatureCallOperation {
	return SignatureCallOperation{
		signature:           o.signature.Clone(),
		contentID:           o.contentID,
		allocationContentID: o.allocationContentID,
		intrinsic:           o.intrinsic,
	}
}
func (o SignatureCallOperation) equal(other SignatureCallOperation) bool {
	return o.contentID == other.contentID &&
		o.allocationContentID == other.allocationContentID &&
		o.intrinsic == other.intrinsic &&
		o.signature.Equals(other.signature)
}
