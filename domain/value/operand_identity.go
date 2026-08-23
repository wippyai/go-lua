package value

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
)

// FrameOperandIdentity frames one already owner-issued Value identity under a
// consuming rule's owner-issued semantic key. Two rules that bind over one
// operand row - body-result and result-alias both bind the first result slot
// of a mounted call - therefore declare distinct operand entities without
// either of them minting an identity of its own.
func (schema *Schema) FrameOperandIdentity(semantic identity.SemanticKey, id identity.ContentID) (identity.ContentID, bool) {
	if schema == nil || !schema.Valid() || !semantic.Available() || !id.Available() {
		return identity.ContentID{}, false
	}
	digest, version := semantic.Digest(), semantic.Version()
	var frame [8]byte
	binary.BigEndian.PutUint64(frame[:], version)
	return identity.DeriveContentID("wippy.analysis.value.operand", schema.linkID[:], digest[:], frame[:], id[:])
}
