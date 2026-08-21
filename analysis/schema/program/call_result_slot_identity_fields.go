package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// CallResultSlotRowsLawVersion pins the ordered scalar encoding of the
// append-only CallResultSlot plane. The row's own framed ID is committed in
// addition to every coordinate, so a consumer cannot substitute a second
// identity equation while preserving the visible fields.
const CallResultSlotRowsLawVersion uint64 = 2

// WriteCallResultSlotIdentityFields appends the canonical slot plane to an
// existing Program identity stream. It validates each row's optional value
// shape but leaves cross-family parent joins to publication.Validate.
func (row Program) WriteCallResultSlotIdentityFields(writer identity.IdentityWriter) bool {
	if writer == nil || !row.Frozen.Published() {
		return false
	}
	catalog := row.Frozen.Schema()
	count, published := CallResultSlotFamily().Count(&row.Frozen, catalog)
	if !published || !writer.WriteUint(CallResultSlotRowsLawVersion) || !writer.WriteUint(uint64(count)) {
		return false
	}
	for index := 0; index < count; index++ {
		slot, held := CallResultSlotFamily().At(&row.Frozen, catalog, index)
		ordinal, ordinalOK := slot.Ordinal()
		position, positionOK := slot.ConsumerPosition()
		value, hasValue := slot.ValueID()
		if !held || !slot.Available() || !ordinalOK || !positionOK {
			return false
		}
		if !writer.WriteContentID(slot.ID()) || !writer.WriteContentID(slot.CallID()) ||
			!writer.WriteUint(uint64(ordinal)) || !writer.WriteUint(uint64(slot.SourceKind())) ||
			!writer.WriteUint(uint64(slot.ConsumerKind())) || !writer.WriteContentID(slot.ConsumerID()) ||
			!writer.WriteUint(uint64(position)) || !writer.WriteBool(hasValue) ||
			!writer.WriteContentID(value) {
			return false
		}
	}
	return true
}
