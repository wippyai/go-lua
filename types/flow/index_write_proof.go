package flow

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
