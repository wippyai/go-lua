package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// WriteValuesIdentityFields replays the historical Values/member/tail portion
// of the Artifact identity from the sealed Program publication.
func (row Program) WriteValuesIdentityFields(writer identity.IdentityWriter) bool {
	if writer == nil || !row.Frozen.Published() {
		return false
	}
	catalog := row.Frozen.Schema()
	valuesCount, valuesPublished := ValuesFamily().Count(&row.Frozen, catalog)
	if !valuesPublished || !writer.WriteUint(uint64(valuesCount)) {
		return false
	}
	for index := 0; index < valuesCount; index++ {
		values, held := ValuesFamily().At(&row.Frozen, catalog, index)
		offset, members, spanOK := values.MemberSpan()
		if !held || !spanOK || !writer.WriteContentID(values.ID()) || !writer.WriteContentID(values.BodyPathID()) || !writer.WriteUint(uint64(members)) {
			return false
		}
		for position := uint32(0); position < members; position++ {
			member, memberHeld := ValuesMemberFamily().At(&row.Frozen, catalog, int(offset+position))
			if !memberHeld || !writer.WriteContentID(member.ID()) {
				return false
			}
		}
		tail, present := values.Tail()
		if !writer.WriteBool(present) || !writer.WriteUint(uint64(tail.Kind())) || !writer.WriteContentID(tail.ID()) {
			return false
		}
	}
	return true
}
