package result

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
)

// appendNativeArtifactSummaryRows reads the cold Program-owned native summary
// columns directly. Each canonical row is consumed immediately by the native
// publication projection; no summary-shaped transport value is retained.
func appendNativeArtifactSummaryRows(rows *[]nativePublicationRow, seen map[identity.ContentID]struct{}, mounts []Mount) bool {
	if rows == nil || seen == nil || len(mounts) == 0 {
		return false
	}
	for _, mount := range mounts {
		if mount.Snapshot == nil || !mount.Snapshot.Available() || !mount.Program.Available() || !mount.Snapshot.ArtifactID().Available() || mount.Program.ArtifactID != mount.Snapshot.ArtifactID() {
			return false
		}
		exactCount, exactPublished := mount.Program.ExactScalarSummaryCount()
		arithmeticCount, arithmeticPublished := mount.Program.ArithmeticSummaryCount()
		unaryCount, unaryPublished := mount.Program.UnarySummaryCount()
		if !exactPublished || !arithmeticPublished || !unaryPublished {
			return false
		}
		for summaryIndex := 0; summaryIndex < exactCount; summaryIndex++ {
			summary, summaryOK := mount.Program.ExactScalarSummaryAt(summaryIndex)
			occurrence, occurrenceOK := mount.Snapshot.OccurrenceForID(uint8(programartifact.OccurrenceBinaryArithmetic), summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			if !summaryOK || !occurrenceOK || !bodyOK || summary.BodyPathID() != bodyID {
				return false
			}
			point, pointOK := exactNativeScalarRulePoint(mount.Snapshot, summary.OccurrenceID())
			if !pointOK || !appendNativeStaticScalarRows(rows, seen, summary, mount.Program.ModuleKey, mount.Snapshot.ArtifactID(), bodyID, point) {
				return false
			}
		}
		for summaryIndex := 0; summaryIndex < arithmeticCount; summaryIndex++ {
			summary, summaryOK := mount.Program.ArithmeticSummaryAt(summaryIndex)
			occurrence, occurrenceOK := mount.Snapshot.OccurrenceForID(uint8(programartifact.OccurrenceBinaryArithmetic), summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			point, pointOK := exactNativeScalarRulePoint(mount.Snapshot, summary.OccurrenceID())
			if !summaryOK || !occurrenceOK || !bodyOK || !pointOK || summary.BodyPathID() != bodyID ||
				!appendNativeArithmeticRows(rows, seen, summary, mount.Program.ModuleKey, mount.Snapshot.ArtifactID(), occurrence.ID(), bodyID, point) {
				return false
			}
		}
		for summaryIndex := 0; summaryIndex < unaryCount; summaryIndex++ {
			summary, summaryOK := mount.Program.UnarySummaryAt(summaryIndex)
			occurrence, occurrenceOK := mount.Snapshot.OccurrenceForID(uint8(programartifact.OccurrenceUnary), summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			if !summaryOK || !occurrenceOK || !bodyOK || summary.BodyPathID() != bodyID ||
				!appendNativeUnaryRows(rows, seen, summary, mount.Program.ModuleKey, mount.Snapshot.ArtifactID(), bodyID) {
				return false
			}
		}
	}
	return true
}

func exactNativeScalarRulePoint(snapshot *ingress.Snapshot, occurrence identity.ContentID) (identity.ContentID, bool) {
	if snapshot == nil || !snapshot.Available() || !occurrence.Available() {
		return identity.ContentID{}, false
	}
	var point identity.ContentID
	found := false
	for index := 0; index < snapshot.RulePlacementCount(); index++ {
		row, rowOK := snapshot.RulePlacementAt(index)
		output, outputOK := row.OutputSemanticID()
		candidate := row.PointID()
		if !rowOK || !outputOK || !candidate.Available() {
			continue
		}
		if row.OccurrenceID() != occurrence || output != occurrence {
			continue
		}
		if found || row.Stage() != uint8(programartifact.RuleStageLocal) {
			return identity.ContentID{}, false
		}
		point, found = candidate, true
	}
	return point, found && point.Available()
}
