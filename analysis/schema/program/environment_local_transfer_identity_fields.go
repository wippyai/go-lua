package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// WriteEnvironmentLocalTransferIdentityFields replays the historical
// environment/reset and local-transfer/write portions of the Artifact
// identity. Their child spans are storage only; their emitted order and
// widths are the committed schema contract.
func (row Program) WriteEnvironmentLocalTransferIdentityFields(writer identity.StringIdentityWriter) bool {
	if writer == nil || !row.Frozen.Published() {
		return false
	}
	catalog := row.Frozen.Schema()
	edgeCount, edgesPublished := EnvironmentEdgeFamily().Count(&row.Frozen, catalog)
	resetCount, resetsPublished := EnvironmentResetFamily().Count(&row.Frozen, catalog)
	if !edgesPublished || !resetsPublished || !writer.WriteUint(uint64(edgeCount)) {
		return false
	}
	for index := 0; index < edgeCount; index++ {
		edge, held := EnvironmentEdgeFamily().At(&row.Frozen, catalog, index)
		offset, resets, spanOK := edge.ResetSpan()
		if !held || !edge.Available() || !spanOK || uint64(offset)+uint64(resets) > uint64(resetCount) {
			return false
		}
		guard, _ := edge.GuardID()
		decision, _ := edge.DecisionID()
		condition, _ := edge.ConditionValueSpanID()
		truth, guarded := edge.Truth()
		mu, hasMu := edge.MuPathID()
		reset, hasReset := edge.ResetDigest()
		if !writer.WriteContentID(edge.ID()) || !writer.WriteContentID(edge.From()) || !writer.WriteContentID(edge.To()) || !writer.WriteContentID(edge.RouteID()) ||
			!writer.WriteUint(uint64(edge.Arm())) || !writer.WriteContentID(guard) || !writer.WriteContentID(decision) || !writer.WriteContentID(condition) ||
			!writer.WriteBool(guarded) || !writer.WriteBool(truth) || !writer.WriteContentID(edge.ComponentID()) ||
			!writer.WriteContentID(mu) || !writer.WriteBool(hasMu) || !writer.WriteContentID(reset) || !writer.WriteBool(hasReset) ||
			!writer.WriteUint(uint64(resets)) {
			return false
		}
		for position := uint32(0); position < resets; position++ {
			witness, witnessHeld := EnvironmentResetFamily().At(&row.Frozen, catalog, int(offset+position))
			if !witnessHeld || !witness.Available() || !writer.WriteContentID(witness.ID()) {
				return false
			}
		}
	}

	transferCount, transfersPublished := LocalTransferFamily().Count(&row.Frozen, catalog)
	writeCount, writesPublished := LocalTransferWriteFamily().Count(&row.Frozen, catalog)
	if !transfersPublished || !writesPublished || !writer.WriteUint(uint64(transferCount)) {
		return false
	}
	for index := 0; index < transferCount; index++ {
		transfer, held := LocalTransferFamily().At(&row.Frozen, catalog, index)
		offset, writes, spanOK := transfer.WriteSpan()
		if !held || !transfer.Available() || !spanOK || uint64(offset)+uint64(writes) > uint64(writeCount) ||
			!writer.WriteContentID(transfer.ID()) || !writer.WriteContentID(transfer.From()) || !writer.WriteContentID(transfer.To()) ||
			!writer.WriteBool(transfer.Full()) || !writer.WriteUint(uint64(writes)) {
			return false
		}
		for position := uint32(0); position < writes; position++ {
			write, writeHeld := LocalTransferWriteFamily().At(&row.Frozen, catalog, int(offset+position))
			key, keyOK := write.Key()
			if !writeHeld || !write.Available() || !keyOK || !writer.WriteString(string(key)) {
				return false
			}
		}
	}
	return true
}
