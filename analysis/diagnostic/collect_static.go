package diagnostic

import (
	"github.com/wippyai/go-lua/analysis/identity"
	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// StaticSubject is one owner-issued static observation the collector projects.
type StaticSubject struct {
	ID, FindingID identity.ContentID
	Location      DiagnosticLocation
	Kind          structure.DiagnosticObservationKind
	Name          string
}

// StaticDeclaration resolves the declared row one artifact-issued observation
// population feeds. The sealed table is the sole authority: a population no
// row claims is a collector hole, not a row to skip.
func StaticDeclaration(table schemadiag.Table, vocabulary structure.Table, kind structure.DiagnosticObservationKind) (*schemadiag.Entry, bool) {
	if !table.Available() {
		return nil, false
	}
	population, populationOK := structure.DiagnosticObservationEntry(vocabulary, kind)
	if !populationOK {
		return nil, false
	}
	return table.ForStaticObservation(population.Key())
}

// CollectStatic owns policy selection for static rows. A row without an
// enabled matching projector remains a no-op, rather than being reverse-
// engineered from Engine state.
func CollectStatic(report *DiagnosticReport, subjects []StaticSubject, policy DiagnosticPolicy) bool {
	if report == nil {
		return false
	}
	for _, subject := range subjects {
		if !subject.ID.Available() || !subject.FindingID.Available() || !subject.Location.Available() || subject.Name == "" {
			return false
		}
		entry, known := StaticDeclaration(report.declarations, report.vocabulary, subject.Kind)
		if !known {
			return false
		}
		severity, enabled := policy.EnabledFor(report.declarations, entry.Code())
		if !enabled {
			continue
		}
		switch entry.Code() {
		case DiagnosticCodeUnresolvedTypeReference, DiagnosticCodeUnresolvedValueReference:
			if !appendStaticUnresolvedFinding(report, subject, entry.Code(), severity) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func appendStaticUnresolvedFinding(report *DiagnosticReport, subject StaticSubject, code DiagnosticCode, severity FindingSeverity) bool {
	if report == nil || !subject.ID.Available() || !subject.FindingID.Available() || !severity.Available() {
		return false
	}
	name, nameOK := NewSemanticName(subject.Name)
	if !nameOK || !subject.Location.Available() {
		return false
	}
	report.AppendFinding(NewFindingRow(subject.FindingID, subject.ID, code, severity, subject.Location, NewTemplateData(name, EmptyTarget(), 0, DiagnosticLocation{})))
	return true
}
