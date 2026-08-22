package typ

import "crypto/sha256"

// CanonicalFormalsReceipt is the immutable result of scoped canonical
// admission.  The owner marker is private so callers cannot manufacture a
// receipt from arbitrary bytes or a formal count.  Copies of a receipt are
// safe: its byte image is never exposed for mutation.
type CanonicalFormalsReceipt struct {
	owner           *canonicalFormalsReceiptOwner
	encoded         []byte
	digest          CanonicalDigest
	externalFormals uint32
}

type canonicalFormalsReceiptOwner struct{}

var canonicalFormalsReceiptAuthority = &canonicalFormalsReceiptOwner{}

func (receipt CanonicalFormalsReceipt) valid() bool {
	return receipt.owner == canonicalFormalsReceiptAuthority && len(receipt.encoded) != 0
}

// Valid reports whether receipt was minted by the canonical scoped-formal
// owner.  A zero receipt and a receipt with an empty image are unavailable.
func (receipt CanonicalFormalsReceipt) Valid() bool { return receipt.valid() }

// ExternalFormals returns the receiver scope cardinality carried by receipt.
// Callers should check Valid before treating zero as an admitted empty scope.
func (receipt CanonicalFormalsReceipt) ExternalFormals() uint32 {
	if !receipt.valid() {
		return 0
	}
	return receipt.externalFormals
}

// Digest is the owner-issued identity of the admitted scoped image. Consumers
// use it instead of cloning and hashing the receipt bytes again.
func (receipt CanonicalFormalsReceipt) Digest() (CanonicalDigest, bool) {
	if !receipt.valid() {
		return CanonicalDigest{}, false
	}
	return receipt.digest, true
}

// Bytes returns an ownership-isolated copy of the admitted scoped bytes.
// There is intentionally no borrowed-byte accessor: the receipt's image is
// immutable for its entire lifetime.
func (receipt CanonicalFormalsReceipt) Bytes() []byte {
	if !receipt.valid() {
		return nil
	}
	return append([]byte(nil), receipt.encoded...)
}

func newCanonicalFormalsReceipt(encoded []byte, externalFormals uint32) CanonicalFormalsReceipt {
	if len(encoded) == 0 {
		return CanonicalFormalsReceipt{}
	}
	return CanonicalFormalsReceipt{
		owner:           canonicalFormalsReceiptAuthority,
		encoded:         encoded,
		digest:          CanonicalDigest(sha256.Sum256(encoded)),
		externalFormals: externalFormals,
	}
}
