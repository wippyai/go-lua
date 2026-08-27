package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// These versions remain part of the Artifact preimage, but the Program point
// publication owns the row shape and therefore the versioned stream.
const (
	PointGeometryLawVersion   uint64 = 3
	PointAttachmentLawVersion uint64 = 2
)

// WritePointIdentityFields replays the historical point and decision portion
// of the Artifact identity from the sealed Program publication.
func (row Program) WritePointIdentityFields(writer identity.IdentityWriter) bool {
	if writer == nil || !row.Frozen.Published() {
		return false
	}
	catalog := row.Frozen.Schema()
	pointCount, pointsPublished := PointFamily().Count(&row.Frozen, catalog)
	if !pointsPublished || !writer.WriteUint(PointGeometryLawVersion) || !writer.WriteUint(PointAttachmentLawVersion) || !writer.WriteUint(uint64(pointCount)) {
		return false
	}
	for index := 0; index < pointCount; index++ {
		point, held := PointFamily().At(&row.Frozen, catalog, index)
		offset, decisions, spanOK := point.DecisionSpan()
		if !held || !spanOK || !writer.WriteContentID(point.ID()) || !writer.WriteContentID(point.ScopeID()) || !writer.WriteBool(point.Initial()) || !writer.WriteUint(uint64(decisions)) {
			return false
		}
		for position := uint32(0); position < decisions; position++ {
			decision, decisionHeld := PointDecisionFamily().At(&row.Frozen, catalog, int(offset+position))
			if !decisionHeld || !writer.WriteContentID(decision.ID()) {
				return false
			}
		}
	}
	return true
}
