package result

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// appendNativeArtifactSummaryRows reads the cold Program-owned native summary
// columns directly. Each canonical row is consumed immediately by the native
// publication projection; no summary-shaped transport value is retained.
func appendNativeArtifactSummaryRows(rows *[]nativePublicationRow, seen map[identity.ContentID]struct{}, mounts []Mount) bool {
	if rows == nil || seen == nil || len(mounts) == 0 {
		return false
	}
	for _, mount := range mounts {
		if mount.snapshot == nil || !mount.snapshot.Available() || !mount.program.Available() || !mount.snapshot.ArtifactID().Available() || mount.program.ArtifactID != mount.snapshot.ArtifactID() {
			return false
		}
		exactCount, exactPublished := mount.program.ExactScalarSummaryCount()
		arithmeticCount, arithmeticPublished := mount.program.ArithmeticSummaryCount()
		unaryCount, unaryPublished := mount.program.UnarySummaryCount()
		if !exactPublished || !arithmeticPublished || !unaryPublished {
			return false
		}
		for summaryIndex := 0; summaryIndex < exactCount; summaryIndex++ {
			summary, summaryOK := mount.program.ExactScalarSummaryAt(summaryIndex)
			occurrence, occurrenceOK := mount.program.OccurrenceForID(programschema.OccurrenceBinaryArithmetic, summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			if !summaryOK || !occurrenceOK || !bodyOK || summary.BodyPathID() != bodyID {
				return false
			}
			point, pointOK := exactNativeScalarRulePoint(mount.snapshot, mount.program.Program, summary.OccurrenceID())
			if !pointOK || !appendNativeStaticScalarRows(rows, seen, summary, mount.program.ModuleKey, mount.snapshot.ArtifactID(), bodyID, point) {
				return false
			}
		}
		for summaryIndex := 0; summaryIndex < arithmeticCount; summaryIndex++ {
			summary, summaryOK := mount.program.ArithmeticSummaryAt(summaryIndex)
			occurrence, occurrenceOK := mount.program.OccurrenceForID(programschema.OccurrenceBinaryArithmetic, summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			point, pointOK := exactNativeScalarRulePoint(mount.snapshot, mount.program.Program, summary.OccurrenceID())
			if !summaryOK || !occurrenceOK || !bodyOK || !pointOK || summary.BodyPathID() != bodyID ||
				!appendNativeArithmeticRows(rows, seen, summary, mount.program.ModuleKey, mount.snapshot.ArtifactID(), occurrence.ID(), bodyID, point) {
				return false
			}
		}
		for summaryIndex := 0; summaryIndex < unaryCount; summaryIndex++ {
			summary, summaryOK := mount.program.UnarySummaryAt(summaryIndex)
			occurrence, occurrenceOK := mount.program.OccurrenceForID(programschema.OccurrenceUnary, summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			if !summaryOK || !occurrenceOK || !bodyOK || summary.BodyPathID() != bodyID ||
				!appendNativeUnaryRows(rows, seen, summary, mount.program.ModuleKey, mount.snapshot.ArtifactID(), bodyID) {
				return false
			}
		}
	}
	return true
}

func exactNativeScalarRulePoint(snapshot *ingress.Snapshot, program programschema.Program, occurrence identity.ContentID) (identity.ContentID, bool) {
	if snapshot == nil || !snapshot.Available() || !occurrence.Available() {
		return identity.ContentID{}, false
	}
	if !program.Available() {
		return identity.ContentID{}, false
	}
	var point identity.ContentID
	found := false
	ruleCount, rulePublished := program.RuleOccurrenceCount()
	if !rulePublished {
		return identity.ContentID{}, false
	}
	for index := 0; index < ruleCount; index++ {
		row, rowOK := program.RuleOccurrenceAt(index)
		ordinal, ordinalOK := row.Occurrence()
		parent, parentOK := program.OccurrenceAt(int(ordinal))
		candidate := row.PointID()
		if !rowOK || !ordinalOK || !parentOK || !candidate.Available() {
			continue
		}
		if parent.ID() != occurrence {
			continue
		}
		if found || row.Stage() != programschema.RuleStageLocal {
			return identity.ContentID{}, false
		}
		point, found = candidate, true
	}
	return point, found && point.Available()
}
