package heapallocation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// WriteArtifactIdentityFields replays the historical allocation and field
// portion of the Artifact identity from the sealed Program publication.
func WriteArtifactIdentityFields(frozen snapshot.Frozen, writer identity.IdentityWriter) bool {
	if writer == nil || !frozen.Published() {
		return false
	}
	catalog := frozen.Schema()
	allocationCount, allocationsPublished := AllocationFamily().Count(&frozen, catalog)
	if !allocationsPublished || !writer.WriteUint(uint64(allocationCount)) {
		return false
	}
	for index := 0; index < allocationCount; index++ {
		allocation, held := AllocationFamily().At(&frozen, catalog, index)
		offset, fields, spanOK := allocation.FieldSpan()
		if !held || !spanOK ||
			!writer.WriteContentID(allocation.ID()) ||
			!writer.WriteUint(uint64(allocation.Role())) ||
			!writer.WriteUint(uint64(allocation.Form())) ||
			!writer.WriteContentID(allocation.RootSpan()) ||
			!writer.WriteUint(uint64(fields)) {
			return false
		}
		for position := uint32(0); position < fields; position++ {
			field, fieldHeld := FieldFamily().At(&frozen, catalog, int(offset+position))
			valuesSpan, width, finalOpen, valuesOK := field.Values()
			normalized, normalizedOK := field.NormalizedKey()
			if !fieldHeld || !valuesOK ||
				!writer.WriteContentID(field.ID()) ||
				!writer.WriteUint(uint64(field.Kind())) ||
				!writer.WriteContentID(field.FieldSpan()) ||
				!writer.WriteContentID(field.SelectorSpan()) ||
				!writer.WriteContentID(valuesSpan) ||
				!writer.WriteContentID(field.ValuesID()) ||
				!writer.WriteUint(uint64(width)) ||
				!writer.WriteBool(finalOpen) ||
				!writer.WriteBool(field.SharesFirstValueCell()) ||
				!writer.WriteUint(normalized) ||
				!writer.WriteBool(normalizedOK) {
				return false
			}
		}
	}
	return true
}
