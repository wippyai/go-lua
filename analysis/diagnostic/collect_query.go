package diagnostic

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/conformance"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// CallArgumentSubject is one TypeConformance call-argument site.
type CallArgumentSubject struct {
	ID, FindingID, Mount identity.ContentID
	Location             DiagnosticLocation
	Site                 schemadiag.Site
	Actual               uint32
	DeclaredMay          runtimekind.Set
	Target               string
	Evidence             []identity.ContentID
}

// QuerySummaryKey is one QueryFamily value-summary row address.
type QuerySummaryKey struct {
	Mount, Point, Key identity.ContentID
}

// CollectCallArguments reads each issued TypeConformance call-argument site
// against the QueryFamily value summary at its evidence points. The judgment
// is MayKindConformance: only a family outside the formal's declared may-set
// becomes a finding. Empty, All, and unprojected declarations abstain.
func CollectCallArguments(
	report *DiagnosticReport,
	subjects []CallArgumentSubject,
	summaries []QuerySummaryKey,
	valueWidth int,
	schema *valuedomain.Schema,
	published *snapshot.Snapshot,
	queryPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	severity FindingSeverity,
) bool {
	if report == nil || schema == nil || published == nil || !published.Published() || !queryPlan.Available() || !severity.Available() || valueWidth < 0 {
		return false
	}
	keys := make(map[pointKey]identity.ContentID, len(summaries))
	for _, query := range summaries {
		if !query.Mount.Available() || !query.Point.Available() || !query.Key.Available() {
			continue
		}
		key := pointKey{mount: query.Mount, point: query.Point}
		if _, duplicate := keys[key]; duplicate {
			return false
		}
		keys[key] = query.Key
	}
	for _, subject := range subjects {
		if !subject.ID.Available() || !subject.FindingID.Available() || !subject.Mount.Available() || !subject.Location.Available() ||
			!subject.Site.Available() || !subject.DeclaredMay.Valid() || len(subject.Evidence) == 0 {
			return false
		}
		if subject.DeclaredMay == runtimekind.All {
			continue
		}
		var observed runtimekind.Set
		anyEvidence := false
		for _, point := range subject.Evidence {
			if !point.Available() {
				return false
			}
			publicationKey, known := keys[pointKey{mount: subject.Mount, point: point}]
			if !known {
				report.SetCollectionFailure(DiagnosticCollectionSubjectQueryAbsent)
				return true
			}
			summary, readable := PublishedObservation[valuedomain.ValueSummaryObservation](published, queryPlan, publicationKey)
			if !publicationKey.Available() || !readable {
				report.SetCollectionFailure(DiagnosticCollectionQueryUnreadable)
				return true
			}
			if !summary.Valid {
				report.SetCollectionFailure(DiagnosticCollectionQueryInvalid)
				return true
			}
			if len(summary.Values) != len(summary.Present) || len(summary.Values) != valueWidth || summary.Rows > 1 {
				report.SetCollectionFailure(DiagnosticCollectionValueShapeMismatch)
				return true
			}
			if summary.Rows == 0 {
				continue
			}
			index := int(subject.Actual)
			if index < 0 || index >= len(summary.Values) {
				report.SetCollectionFailure(DiagnosticCollectionValueShapeMismatch)
				return true
			}
			if !summary.Present[index] {
				continue
			}
			kinds := schema.RuntimeKinds(summary.Values[index])
			if !kinds.Valid() {
				return false
			}
			observed |= kinds
			anyEvidence = true
		}
		if !anyEvidence {
			continue
		}
		switch conformance.MayKindConformance(subject.DeclaredMay, observed) {
		case conformance.VerdictViolates:
			if !appendCallArgumentFinding(report, subject, severity) {
				return false
			}
		case conformance.VerdictConforms, conformance.VerdictAbstain:
		default:
			return false
		}
	}
	return true
}

func appendCallArgumentFinding(report *DiagnosticReport, subject CallArgumentSubject, severity FindingSeverity) bool {
	if report == nil || !subject.ID.Available() || !subject.FindingID.Available() || !severity.Available() {
		return false
	}
	name, nameOK := NewSemanticName("argument")
	target, targetOK := NewTargetType(subject.Target)
	if !nameOK || !targetOK || !subject.Location.Available() {
		return false
	}
	table, tableOK := composite.Diagnostics()
	if !tableOK {
		return false
	}
	declared, declaredOK := table.ForBranchObservation(structure.DiagnosticObservationTypeConformance.Key(), subject.Site)
	data := NewTemplateData(name, target, 0, DiagnosticLocation{})
	if !declaredOK || !declared.Site().Available() || !data.ValidFor(declared) {
		return false
	}
	report.AppendFinding(NewFindingRow(subject.FindingID, subject.ID, declared.Code(), severity, subject.Location, data))
	return true
}
