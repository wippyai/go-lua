package heapindex

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// WriteArtifactIdentityFields replays the historical heap-index portion of
// the Artifact identity from the sealed Program publication.
func WriteArtifactIdentityFields(frozen snapshot.Frozen, writer identity.IdentityWriter) bool {
	if writer == nil || !frozen.Published() {
		return false
	}
	catalog := frozen.Schema()
	indexCount, indexesPublished := Family().Count(&frozen, catalog)
	if !indexesPublished || !writer.WriteUint(uint64(indexCount)) {
		return false
	}
	for index := 0; index < indexCount; index++ {
		access, held := Family().At(&frozen, catalog, index)
		if !held {
			return false
		}
		exactKey, _ := access.ExactKey()
		valuesSpan, _, _ := access.Values()
		if !writer.WriteContentID(access.ID()) ||
			!writer.WriteBool(access.Read()) ||
			!writer.WriteContentID(access.BaseSpan()) ||
			!writer.WriteContentID(access.ResultSpan()) ||
			!writer.WriteContentID(access.DynamicKeySpan()) ||
			!writer.WriteUint(uint64(access.LensKind())) ||
			!writer.WriteUint(exactKey) ||
			!writer.WriteContentID(valuesSpan) ||
			!writer.WriteContentID(access.ValuesID()) ||
			!writer.WriteUint(uint64(access.Position()+1)) {
			return false
		}
	}
	return true
}
