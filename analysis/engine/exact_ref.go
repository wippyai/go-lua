package engine

type exactRef interface {
	receiptRaw(factorRuntimeReceipt) (uint64, bool)
}

// exactRef is the private factor-coordinate projection shared by receipt
// runtime binding and selector evaluation. Ref is the only engine-created
// implementation, so callers cannot manufacture raw equation surfaces.
//
// It intentionally has no Assembly dependency: receipt-native binding uses
// the exact same sealed factor authority directly.
// receiptRaw requires the exact sealed Binding receipt before exposing the
// factor-local coordinate to receipt-native runtime code.
func (ref Ref[K]) receiptRaw(receipt factorRuntimeReceipt) (uint64, bool) {
	if !validateRefForReceipt(receipt, ref) {
		return 0, false
	}
	return uint64(ref.raw), true
}

func validateRefForReceipt[K ~uint32 | ~uint64](receipt factorRuntimeReceipt, ref Ref[K]) bool {
	return receipt.valid() && ref.bindingAuthority == receipt.authority && ref.compositionID == receipt.schema.ID() && ref.factorKey == receipt.semantic && ref.factorIndex == receipt.ordinal && uint64(ref.raw) < receipt.keyEnd
}
