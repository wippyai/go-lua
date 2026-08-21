package diagnostic

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/conformance"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// ConformanceSubject is one TypeConformance site: the value the program
// assigns or passes, the declaration it is measured against, and the base
// evidence points its measured value is observed at.
type ConformanceSubject struct {
	ID, FindingID, Mount identity.ContentID
	Location             DiagnosticLocation
	Site                 schemadiag.Site
	Actual               uint32
	DeclaredMay          runtimekind.Set
	Target               string
	// Member is the declared field a structural site names. An absent-member
	// site is the finding itself, so its member is what the finding renders;
	// an established member names the field its value was measured against.
	Member string
	// Points are the execution points of the occurrences that produce the
	// measured value. A statement carries one base evidence point and as many
	// producing occurrences as it has measured values, so a subject addresses
	// its own column by its producers rather than by the point they anchor to.
	Points []identity.ContentID
}

// CollectConformance reads each issued TypeConformance site against the Value
// summary observed at the rule occurrence that produced the measured value,
// and answers it in the conformance judgment's own verdict vocabulary. The
// declaration table renders the answer: this collector emits a verdict and the
// payload that verdict names, never prose.
//
// The measured value is read from the observation column, not from a
// point-keyed query column. A query site is published only at selected points,
// and the occurrence that produces an argument or initializer value is not one
// of them, so a query read abstains on every site by construction.
//
// An absent member is the one site whose answer is not read from a value. The
// issuance decided it from the constructor's own geometry - a required
// declared field the established key set does not supply, over a closed
// constructor - so the collector publishes that verdict rather than measuring
// the allocation's value against a field it does not carry.
func CollectConformance(
	report *DiagnosticReport,
	subjects []ConformanceSubject,
	valueWidth int,
	schema *valuedomain.Schema,
	published *snapshot.Snapshot,
	observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	severities map[schemadiag.Site]FindingSeverity,
) bool {
	if report == nil || schema == nil || published == nil || !published.Published() || !observationPlan.Available() || valueWidth < 0 || len(severities) == 0 {
		return false
	}
	for _, subject := range subjects {
		if !subject.ID.Available() || !subject.FindingID.Available() || !subject.Mount.Available() || !subject.Location.Available() ||
			!subject.Site.Available() || !subject.DeclaredMay.Valid() || len(subject.Points) == 0 {
			return false
		}
		severity, enabled := severities[subject.Site]
		if !enabled || !severity.Available() {
			continue
		}
		if subject.Site == schemadiag.SiteMemberAbsent {
			if subject.Member == "" {
				continue
			}
			if !appendConformanceFinding(report, subject, conformance.VerdictMemberAbsent, EmptyObservedSpelling(), severity) {
				return false
			}
			continue
		}
		if subject.DeclaredMay == runtimekind.All {
			continue
		}
		observed, exact, anyEvidence, readable := observedValue(report, subject, valueWidth, schema, published, observationPlan)
		if !readable {
			return anyEvidence
		}
		if !anyEvidence {
			continue
		}
		verdict := conformanceVerdict(subject.DeclaredMay, observed)
		switch verdict {
		case conformance.VerdictViolates, conformance.VerdictMayBeNil:
			actual := exact
			if verdict == conformance.VerdictViolates && !actual.valid() {
				spelling, spellingOK := ObservedFamilies(report.vocabulary, observed)
				if !spellingOK {
					return false
				}
				actual = spelling
			}
			if verdict == conformance.VerdictMayBeNil {
				actual = EmptyObservedSpelling()
			}
			if !appendConformanceFinding(report, subject, verdict, actual, severity) {
				return false
			}
		case conformance.VerdictConforms, conformance.VerdictAbstain:
		default:
			return false
		}
	}
	return true
}

// conformanceVerdict is the answer for one measured value. The two judgments
// are asked together, in the order their findings are named: nil presence is
// the narrower answer, and a containment violation that is one only because of
// nil is that finding rather than a general one.
func conformanceVerdict(declaredMay, observed runtimekind.Set) conformance.Verdict {
	verdict := conformance.MayKindConformance(declaredMay, observed)
	if verdict != conformance.VerdictViolates {
		return verdict
	}
	if nilPresence := conformance.MayBeNilConformance(declaredMay, observed); nilPresence == conformance.VerdictMayBeNil {
		return nilPresence
	}
	return verdict
}

// observedValue joins one subject's measured value over every occurrence that
// produces it: the families it may carry, and the exact scalar constant when
// every producer agrees on one. A value established on two paths with two
// different constants has no constant of its own, and the families are what
// the finding names.
//
// readable is false when the report has recorded a collection failure, which
// ends collection; anyEvidence is false when no producer supplied a value at
// all, which is a site the program never reached.
func observedValue(
	report *DiagnosticReport,
	subject ConformanceSubject,
	valueWidth int,
	schema *valuedomain.Schema,
	published *snapshot.Snapshot,
	observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
) (observed runtimekind.Set, exact diagnosticObservedSpelling, anyEvidence, readable bool) {
	agreed := true
	for _, point := range subject.Points {
		publicationKey, addressed := ValueObservationAddress(report.compilation, structure.DiagnosticObservationTypeConformance, subject.Mount, point)
		if !point.Available() || !addressed {
			return 0, exact, false, false
		}
		summary, summaryReadable := PublishedObservation[valuedomain.ValueSummaryObservation](published, observationPlan, publicationKey)
		if !summaryReadable {
			report.SetCollectionFailure(DiagnosticCollectionSubjectQueryAbsent)
			return 0, exact, true, false
		}
		if !summary.Valid {
			report.SetCollectionFailure(DiagnosticCollectionQueryInvalid)
			return 0, exact, true, false
		}
		if len(summary.Values) != len(summary.Present) || len(summary.Values) != valueWidth || summary.Rows > 1 {
			report.SetCollectionFailure(DiagnosticCollectionValueShapeMismatch)
			return 0, exact, true, false
		}
		if summary.Rows == 0 {
			continue
		}
		index := int(subject.Actual)
		if index < 0 || index >= len(summary.Values) {
			report.SetCollectionFailure(DiagnosticCollectionValueShapeMismatch)
			return 0, exact, true, false
		}
		if !summary.Present[index] {
			continue
		}
		kinds := schema.RuntimeKinds(summary.Values[index])
		if !kinds.Valid() {
			return 0, exact, false, false
		}
		observed |= kinds
		spelling, spellingOK := exactSpelling(schema, summary.Values[index])
		switch {
		case !spellingOK:
			agreed = false
		case !anyEvidence:
			exact = spelling
		case exact != spelling:
			agreed = false
		}
		anyEvidence = true
	}
	if !agreed {
		exact = EmptyObservedSpelling()
	}
	return observed, exact, anyEvidence, true
}

// exactSpelling is the constant one measured value carries, when it carries
// one. The projection is the value domain's own public scalar constant; this
// layer renders it and never reads the value's atoms.
func exactSpelling(schema *valuedomain.Schema, value valuedomain.Value) (diagnosticObservedSpelling, bool) {
	scalar, exact := schema.ExactScalar(value)
	if !exact {
		return EmptyObservedSpelling(), false
	}
	switch scalar.Kind() {
	case valuedomain.ExactScalarNil:
		return ObservedNil(), true
	case valuedomain.ExactScalarBoolean, valuedomain.ExactScalarLiteral:
		literal, held := scalar.Literal()
		if !held {
			return EmptyObservedSpelling(), false
		}
		return ObservedLiteral(literal)
	default:
		return EmptyObservedSpelling(), false
	}
}

// appendConformanceFinding publishes one answered site. The code is resolved
// from the population and the geometry the site names, and the payload is
// exactly what the answered variant declares it reads.
func appendConformanceFinding(
	report *DiagnosticReport,
	subject ConformanceSubject,
	verdict conformance.Verdict,
	actual diagnosticObservedSpelling,
	severity FindingSeverity,
) bool {
	if report == nil || !subject.ID.Available() || !subject.FindingID.Available() || !severity.Available() || !verdict.Available() {
		return false
	}
	table := report.declarations
	if !table.Available() {
		return false
	}
	declared, declaredOK := table.ForBranchObservation(structure.DiagnosticObservationTypeConformance.Key(), subject.Site)
	if !declaredOK || !subject.Location.Available() {
		return false
	}
	target, targetOK := NewTargetType(subject.Target)
	if !targetOK {
		return false
	}
	name, nameOK := NewSemanticName(conformanceSubjectName(subject.Site))
	member, memberOK := NewSemanticName(subject.Member)
	if !nameOK {
		return false
	}
	if !memberOK {
		member = EmptyName()
	}
	data := conformanceTemplateData(verdict, name, target, actual, member)
	if !data.ValidFor(declared, verdict.Ordinal()) {
		return false
	}
	report.AppendFinding(NewVerdictFindingRow(subject.FindingID, subject.ID, declared.Code(), verdict.Ordinal(), severity, subject.Location, data))
	return true
}

// conformanceTemplateData supplies exactly the payload one answer names. A
// field another answer reads is left absent rather than filled in, which is the
// contract every declared row is admitted under.
func conformanceTemplateData(
	verdict conformance.Verdict,
	name diagnosticSemanticName,
	target diagnosticTargetType,
	actual diagnosticObservedSpelling,
	member diagnosticSemanticName,
) diagnosticTemplateData {
	switch verdict {
	case conformance.VerdictMemberAbsent:
		return NewConformanceTemplateData(EmptyName(), target, EmptyObservedSpelling(), member)
	case conformance.VerdictViolates:
		return NewConformanceTemplateData(name, target, actual, EmptyName())
	default:
		return NewConformanceTemplateData(name, target, EmptyObservedSpelling(), EmptyName())
	}
}

// conformanceSubjectName is the name a finding refers to its subject by. It is
// the geometry's own word and not the program's: the binder, argument, and
// member spellings the source authored are established at issuance and are not
// published into the artifact, so this layer has no path to them. Closing that
// is a publication the issuing compiler owes, not a name this layer may invent.
func conformanceSubjectName(site schemadiag.Site) string {
	switch site {
	case schemadiag.SiteCallArgument:
		return "argument"
	case schemadiag.SiteMember:
		return "member"
	default:
		return "value"
	}
}
