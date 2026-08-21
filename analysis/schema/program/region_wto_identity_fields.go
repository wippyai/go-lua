package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// WriteRegionWTOIdentityFields replays the historical region/member and WTO
// bracket portions of the Artifact identity from their sealed Program planes.
func (row Program) WriteRegionWTOIdentityFields(writer identity.IdentityWriter) bool {
	if writer == nil || !row.Frozen.Published() {
		return false
	}
	catalog := row.Frozen.Schema()
	regionCount, regionsPublished := RegionFamily().Count(&row.Frozen, catalog)
	memberCount, membersPublished := RegionMemberFamily().Count(&row.Frozen, catalog)
	if !regionsPublished || !membersPublished || !writer.WriteUint(uint64(regionCount)) {
		return false
	}
	for index := 0; index < regionCount; index++ {
		region, held := RegionFamily().At(&row.Frozen, catalog, index)
		offset, members, spanOK := region.MemberSpan()
		if !held || !region.Available() || !spanOK || uint64(offset)+uint64(members) > uint64(memberCount) ||
			!writer.WriteContentID(region.ID()) || !writer.WriteContentID(region.ParentID()) ||
			!writer.WriteBool(region.Cyclic()) || !writer.WriteUint(uint64(members)) {
			return false
		}
		for position := uint32(0); position < members; position++ {
			member, memberHeld := RegionMemberFamily().At(&row.Frozen, catalog, int(offset+position))
			if !memberHeld || !member.Available() || !writer.WriteContentID(member.ID()) {
				return false
			}
		}
	}
	eventCount, eventsPublished := WTOEventFamily().Count(&row.Frozen, catalog)
	if !eventsPublished || !writer.WriteUint(uint64(eventCount)) {
		return false
	}
	for index := 0; index < eventCount; index++ {
		event, held := WTOEventFamily().At(&row.Frozen, catalog, index)
		if !held || !event.Available() || !writer.WriteUint(uint64(event.Kind())) ||
			!writer.WriteContentID(event.RegionID()) || !writer.WriteContentID(event.PointID()) {
			return false
		}
	}
	return true
}
