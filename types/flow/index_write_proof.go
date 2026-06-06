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
	if out == nil {
		return false
	}
	before := out.IndexWrites
	out.IndexWrites = out.IndexWrites.WithAddress(proof.Fact)
	return !IndexWriteAdmissionFactsDomain.Equal(before, out.IndexWrites)
}

// ApplyIndexWriteKeyAliasProof replays admitted writes whose key path is
// SourceKey under TargetKey.
func ApplyIndexWriteKeyAliasProof(out *PointState, proof IndexWriteKeyAliasProof) bool {
	if out == nil || proof.SourceKey.Key() == "" || proof.TargetKey.Key() == "" {
		return false
	}
	sourceKey := proof.SourceKey.Key()
	targetKey := proof.TargetKey.Key()
	before := out.IndexWrites
	for _, entry := range out.IndexWrites.Entries() {
		if entry.KeyPath != sourceKey {
			continue
		}
		next := entry
		next.KeyPath = targetKey
		out.IndexWrites = out.IndexWrites.With(next)
	}
	return !IndexWriteAdmissionFactsDomain.Equal(before, out.IndexWrites)
}

// ApplyIndexWriteKeyAliasReadbackProof applies all readback consequences of a
// local key alias assignment.
func ApplyIndexWriteKeyAliasReadbackProof(out *PointState, proof IndexWriteKeyAliasReadbackProof) bool {
	if out == nil || proof.SourceKey.Key() == "" || proof.TargetKey.Key() == "" {
		return false
	}
	before := out.IndexWrites
	ApplyIndexWriteKeyAliasProof(out, IndexWriteKeyAliasProof{
		SourceKey: proof.SourceKey,
		TargetKey: proof.TargetKey,
	})
	sourceValue := proof.SourceValue
	if sourceValue.DefinitelyAbsent() {
		return !IndexWriteAdmissionFactsDomain.Equal(before, out.IndexWrites)
	}
	if sourceValue.IsZero() {
		sourceValue = product.FromType(typ.Unknown)
	}
	sourceKey := proof.SourceKey.Key()
	for _, entry := range out.KeyPresence.Entries() {
		if entry.Key != sourceKey {
			continue
		}
		table, ok := StableAddressFromKey(entry.Table)
		if !ok {
			continue
		}
		tableValue, ok := PointFactsOf(*out).AddressValue(table)
		if !ok || tableValue.IsZero() {
			continue
		}
		read, ok := product.RuntimeIndexOf(tableValue, sourceValue)
		if !ok || read.IsZero() {
			continue
		}
		present := product.NarrowPresent(read)
		if present.IsZero() || !AdmissibleMapWriteProofValue(present) {
			continue
		}
		out.IndexWrites = out.IndexWrites.WithAddress(IndexWriteAdmissionAddressFact{
			Target:     table,
			KeyPath:    proof.TargetKey,
			HasKeyPath: true,
			Key:        sourceValue,
			Value:      present,
		})
	}
	return !IndexWriteAdmissionFactsDomain.Equal(before, out.IndexWrites)
}
