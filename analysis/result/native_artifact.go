package result

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

// nativeMountDirectory is this consumer's occurrence workspace for one
// mounted Program. Program publishes no inverse index; OccurrenceForID is a
// documented cold scan. Building the directory once per mount keeps the
// native summary join linear in the published families.
type nativeMountDirectory struct {
	occurrences map[programschema.OccurrenceKind]map[identity.ContentID]programschema.Occurrence
	points      map[identity.ContentID]identity.ContentID
}

func newNativeMountDirectory(program programschema.Program) (nativeMountDirectory, bool) {
	if !program.Available() {
		return nativeMountDirectory{}, false
	}
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !occurrencesPublished {
		return nativeMountDirectory{}, false
	}
	directory := nativeMountDirectory{
		occurrences: make(map[programschema.OccurrenceKind]map[identity.ContentID]programschema.Occurrence, 4),
		points:      make(map[identity.ContentID]identity.ContentID),
	}
	for index := 0; index < occurrenceCount; index++ {
		dbgNativeJoinRowRead()
		row, rowOK := program.OccurrenceAt(index)
		if !rowOK || !row.Available() {
			return nativeMountDirectory{}, false
		}
		kind := row.Kind()
		rows := directory.occurrences[kind]
		if rows == nil {
			rows = make(map[identity.ContentID]programschema.Occurrence)
			directory.occurrences[kind] = rows
		}
		if _, duplicate := rows[row.ID()]; duplicate {
			return nativeMountDirectory{}, false
		}
		rows[row.ID()] = row
	}
	ruleCount, rulesPublished := program.RuleOccurrenceCount()
	if !rulesPublished {
		return nativeMountDirectory{}, false
	}
	seen := make(map[identity.ContentID]struct{})
	invalid := make(map[identity.ContentID]struct{})
	for index := 0; index < ruleCount; index++ {
		dbgNativeJoinRowRead()
		row, rowOK := program.RuleOccurrenceAt(index)
		ordinal, ordinalOK := row.Occurrence()
		dbgNativeJoinRowRead()
		parent, parentOK := program.OccurrenceAt(int(ordinal))
		candidate := row.PointID()
		if !rowOK || !ordinalOK || !parentOK || !candidate.Available() {
			continue
		}
		parentID := parent.ID()
		if _, refused := invalid[parentID]; refused {
			continue
		}
		if _, exists := seen[parentID]; exists {
			delete(directory.points, parentID)
			invalid[parentID] = struct{}{}
			continue
		}
		seen[parentID] = struct{}{}
		if row.Stage() != programissuance.StageComputation {
			invalid[parentID] = struct{}{}
			continue
		}
		directory.points[parentID] = candidate
	}
	return directory, true
}

func (directory nativeMountDirectory) occurrence(kind programschema.OccurrenceKind, id identity.ContentID) (programschema.Occurrence, bool) {
	row, held := directory.occurrences[kind][id]
	return row, held
}

func (directory nativeMountDirectory) computationPoint(occurrence identity.ContentID) (identity.ContentID, bool) {
	point, held := directory.points[occurrence]
	return point, held && point.Available()
}

// appendNativeArtifactSummaryRows reads the cold Program-owned native summary
// columns directly. Each canonical row is consumed immediately by the native
// publication projection; no summary-shaped transport value is retained.
func appendNativeArtifactSummaryRows(rows *[]nativePublicationRow, seen map[identity.ContentID]struct{}, mounts []programmount.MountedArtifact) bool {
	if rows == nil || seen == nil || len(mounts) == 0 {
		return false
	}
	for _, mount := range mounts {
		if !mount.Available() || !mount.Snapshot.ArtifactID().Available() || mount.Program.ArtifactID != mount.Snapshot.ArtifactID() {
			return false
		}
		directory, directoryOK := newNativeMountDirectory(mount.Program.Program)
		if !directoryOK {
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
			occurrence, occurrenceOK := directory.occurrence(programschema.OccurrenceBinaryArithmetic, summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			if !summaryOK || !occurrenceOK || !bodyOK || summary.BodyPathID() != bodyID {
				return false
			}
			point, pointOK := exactNativeScalarRulePoint(mount.Snapshot, directory, summary.OccurrenceID())
			if !pointOK || !appendNativeStaticScalarRows(rows, seen, summary, mount.ModuleKey, mount.Snapshot.ArtifactID(), bodyID, point) {
				return false
			}
		}
		for summaryIndex := 0; summaryIndex < arithmeticCount; summaryIndex++ {
			summary, summaryOK := mount.Program.ArithmeticSummaryAt(summaryIndex)
			occurrence, occurrenceOK := directory.occurrence(programschema.OccurrenceBinaryArithmetic, summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			point, pointOK := exactNativeScalarRulePoint(mount.Snapshot, directory, summary.OccurrenceID())
			if !summaryOK || !occurrenceOK || !bodyOK || !pointOK || summary.BodyPathID() != bodyID ||
				!appendNativeArithmeticRows(rows, seen, summary, mount.ModuleKey, mount.Snapshot.ArtifactID(), occurrence.ID(), bodyID, point) {
				return false
			}
		}
		for summaryIndex := 0; summaryIndex < unaryCount; summaryIndex++ {
			summary, summaryOK := mount.Program.UnarySummaryAt(summaryIndex)
			occurrence, occurrenceOK := directory.occurrence(programschema.OccurrenceUnary, summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			if !summaryOK || !occurrenceOK || !bodyOK || summary.BodyPathID() != bodyID ||
				!appendNativeUnaryRows(rows, seen, summary, mount.ModuleKey, mount.Snapshot.ArtifactID(), bodyID) {
				return false
			}
		}
	}
	return true
}

func exactNativeScalarRulePoint(snapshot *ingress.Snapshot, directory nativeMountDirectory, occurrence identity.ContentID) (identity.ContentID, bool) {
	if snapshot == nil || !snapshot.Available() || !occurrence.Available() {
		return identity.ContentID{}, false
	}
	return directory.computationPoint(occurrence)
}
