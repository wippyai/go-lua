package flow

import (
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// IndexWriteAdmissionProof publishes one normalized dynamic-index write
// admission fact into point state.
type IndexWriteAdmissionProof struct {
	Fact IndexWriteAdmissionAddressFact
}

// IndexWriteKeyAliasProof copies index-write admissions from SourceKey to
// TargetKey after a local assignment proves both paths denote the same key.
type IndexWriteKeyAliasProof struct {
	SourceKey StableAddress
	TargetKey StableAddress
}

// IndexWriteKeyAliasReadbackProof copies dynamic-index admissions from
// SourceKey to TargetKey and derives table[target] readbacks from current
// table/key facts when the source key value is known.
type IndexWriteKeyAliasReadbackProof struct {
	SourceKey   StableAddress
	TargetKey   StableAddress
	SourceValue product.AbstractValue
}

// ApplyIndexWriteAdmissionProof applies a normalized admission proof to point state.
func ApplyIndexWriteAdmissionProof(out *PointState, proof IndexWriteAdmissionProof) bool {
	return RecordIndexWriteAdmission(out, proof.Fact)
}

// ApplyIndexWriteKeyAliasProof replays admitted writes whose key path is
// SourceKey under TargetKey.
func ApplyIndexWriteKeyAliasProof(out *PointState, proof IndexWriteKeyAliasProof) bool {
	return ReplayIndexWriteKeyAlias(out, proof.SourceKey, proof.TargetKey)
}

// ApplyIndexWriteKeyAliasReadbackProof applies all readback consequences of a
// local key alias assignment.
func ApplyIndexWriteKeyAliasReadbackProof(out *PointState, proof IndexWriteKeyAliasReadbackProof) bool {
	if out == nil || proof.SourceKey.Key() == "" || proof.TargetKey.Key() == "" {
		return false
	}
	changed := ApplyIndexWriteKeyAliasProof(out, IndexWriteKeyAliasProof{
		SourceKey: proof.SourceKey,
		TargetKey: proof.TargetKey,
	})
	sourceValue := proof.SourceValue
	if sourceValue.DefinitelyAbsent() {
		return changed
	}
	if sourceValue.IsZero() {
		sourceValue = product.FromType(typ.Unknown)
	}
	for _, tableUse := range KeyPresenceOfPoint(out).TablesWithKeyAddress(proof.SourceKey) {
		table := tableUse.Address
		tableValue, ok := PointFactsOfBorrowed(out).AddressValue(table)
		if !ok || tableValue.IsZero() {
			continue
		}
		read, ok := product.RuntimeIndexOf(tableValue, sourceValue)
		if !ok || read.IsZero() {
			continue
		}
		present := product.NarrowPresent(read)
		if present.IsZero() || !AdmissibleDynamicIndexWriteProofValue(present) {
			continue
		}
		changed = RecordIndexWriteAdmission(out, IndexWriteAdmissionAddressFact{
			Target:     table,
			KeyPath:    proof.TargetKey,
			HasKeyPath: true,
			Key:        sourceValue,
			Value:      present,
		}) || changed
	}
	return changed
}
