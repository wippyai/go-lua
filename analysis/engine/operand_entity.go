package engine

import "github.com/wippyai/go-lua/analysis/engine/internal/composition"

const instanceOperandEntityVersion uint64 = 1

// OperandEntity is the opaque equation-owned content identity used by the
// receipt runtime when it validates a materialized rule member. It is not a
// source-admission capability.
type OperandEntity struct{ key composition.Key }

func (entity OperandEntity) MatchesContentDigest(digest [32]byte) bool {
	return digest != [32]byte{} && entity.key.Available() && entity.key.Version == instanceOperandEntityVersion &&
		[32]byte(entity.key.ID) == digest
}

func operandEntityForContent(digest [32]byte) (composition.Key, bool) {
	entity := composition.Key{ID: composition.ID(digest), Version: instanceOperandEntityVersion}
	return entity, digest != [32]byte{} && entity.Available()
}
