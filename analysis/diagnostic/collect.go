package diagnostic

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	declschema "github.com/wippyai/go-lua/analysis/schema"
	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/composite"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

type pointKey struct {
	mount, point identity.ContentID
}

// ObservationKey is one Snapshot row address for a mounted evidence point.
type ObservationKey struct {
	Mount, Point, Key identity.ContentID
}

// GuardSubject is one branch-condition site the polarity collector reads.
type GuardSubject struct {
	ID, TrueFindingID, FalseFindingID, Mount identity.ContentID
	Location                                 DiagnosticLocation
	ValueIndex                               uint32
	Points                                   []identity.ContentID
}

type guardPolarity uint8

const (
	guardPolarityInvalid guardPolarity = iota
	guardPolarityTrue
	guardPolarityFalse
)

// ClassifyGuardPolarity is the closed decision over reachable condition
// truths. No rows, an unknown truth, or disagreement proves neither polarity.
func ClassifyGuardPolarity(truths []valuedomain.Truth) (truePolarity, falsePolarity bool) {
	switch classifyGuardPolarity(truths) {
	case guardPolarityTrue:
		return true, false
	case guardPolarityFalse:
		return false, true
	default:
		return false, false
	}
}

func classifyGuardPolarity(truths []valuedomain.Truth) guardPolarity {
	if len(truths) == 0 {
		return guardPolarityInvalid
	}
	var polarity guardPolarity
	for _, truth := range truths {
		candidate := guardPolarityInvalid
		switch truth {
		case valuedomain.TruthTrue:
			candidate = guardPolarityTrue
		case valuedomain.TruthFalse:
			candidate = guardPolarityFalse
		default:
			return guardPolarityInvalid
		}
		if polarity == guardPolarityInvalid {
			polarity = candidate
			continue
		}
		if polarity != candidate {
			return guardPolarityInvalid
		}
	}
	return polarity
}

// PublishedObservation reads one typed Snapshot row from an opened plan.
func PublishedObservation[R any](published *snapshot.Snapshot, plan snapshot.QueryPlan[identity.ContentID, engine.Answer], id identity.ContentID) (R, bool) {
	var zero R
	if published == nil || !published.Published() || !plan.Available() || !id.Available() {
		return zero, false
	}
	answer, status := snapshot.Query(published, plan, id)
	if status != snapshot.ReadHit || !answer.Available() {
		return zero, false
	}
	return engine.AnswerValue[R](answer)
}

// PublishedBranchConditionTruth reads the schema result cell from one
// published Value summary.
func PublishedBranchConditionTruth(schema *valuedomain.Schema, valueIndex uint32, observation valuedomain.ValueSummaryObservation) (valuedomain.Truth, bool) {
	if schema == nil || !observation.Valid || len(observation.Values) != len(observation.Present) {
		return valuedomain.TruthNone, false
	}
	resultIdx := int(valueIndex)
	if resultIdx < 0 || resultIdx >= len(observation.Present) || !observation.Present[resultIdx] {
		return valuedomain.TruthNone, false
	}
	return schema.Truthiness(observation.Values[resultIdx]), true
}

// CollectGuardPolarity reads each selected branch observation once. A polarity
// is emitted only when every reachable row has a present value of that exact
// truthiness.
func CollectGuardPolarity(
	report *DiagnosticReport,
	subjects []GuardSubject,
	selected []ObservationKey,
	valueWidth int,
	schema *valuedomain.Schema,
	published *snapshot.Snapshot,
	observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	trueSeverity, falseSeverity FindingSeverity,
) bool {
	if report == nil || schema == nil || published == nil || !published.Published() || !observationPlan.Available() || valueWidth < 0 ||
		(trueSeverity != FindingSeverityInvalid && !trueSeverity.Available()) ||
		(falseSeverity != FindingSeverityInvalid && !falseSeverity.Available()) ||
		(trueSeverity == FindingSeverityInvalid && falseSeverity == FindingSeverityInvalid) {
		return false
	}
	expected := make(map[pointKey]struct{})
	for _, subject := range subjects {
		if !subject.ID.Available() || !subject.Mount.Available() || !subject.Location.Available() || len(subject.Points) == 0 {
			return false
		}
		for _, point := range subject.Points {
			if !point.Available() {
				return false
			}
			expected[pointKey{mount: subject.Mount, point: point}] = struct{}{}
		}
	}
	observations := make(map[pointKey]valuedomain.ValueSummaryObservation, len(expected))
	for _, row := range selected {
		key := pointKey{mount: row.Mount, point: row.Point}
		if _, want := expected[key]; !want {
			report.SetCollectionFailure(DiagnosticCollectionSubjectQueryAbsent)
			return true
		}
		if _, duplicate := observations[key]; duplicate {
			report.SetCollectionFailure(DiagnosticCollectionSubjectQueryAbsent)
			return true
		}
		observation, readable := PublishedObservation[valuedomain.ValueSummaryObservation](published, observationPlan, row.Key)
		if !row.Key.Available() || !readable {
			report.SetCollectionFailure(DiagnosticCollectionQueryUnreadable)
			return true
		}
		if !observation.Valid {
			report.SetCollectionFailure(DiagnosticCollectionQueryInvalid)
			return true
		}
		if len(observation.Values) != len(observation.Present) || len(observation.Values) != valueWidth || observation.Rows > 1 {
			report.SetCollectionFailure(DiagnosticCollectionValueShapeMismatch)
			return true
		}
		observations[key] = observation
	}
	if len(observations) != len(expected) {
		report.SetCollectionFailure(DiagnosticCollectionSubjectQueryAbsent)
		return true
	}
	for _, subject := range subjects {
		truths := make([]valuedomain.Truth, 0, len(subject.Points))
		invalidEvidence := false
		for _, point := range subject.Points {
			observation, observed := observations[pointKey{mount: subject.Mount, point: point}]
			if !observed {
				report.SetCollectionFailure(DiagnosticCollectionSubjectQueryAbsent)
				invalidEvidence = true
				break
			}
			if observation.Rows == 0 {
				continue
			}
			truth, ok := PublishedBranchConditionTruth(schema, subject.ValueIndex, observation)
			if !ok {
				invalidEvidence = true
				break
			}
			truths = append(truths, truth)
		}
		if invalidEvidence {
			continue
		}
		polarity := classifyGuardPolarity(truths)
		if polarity == guardPolarityInvalid {
			continue
		}
		if polarity == guardPolarityTrue && trueSeverity.Available() {
			if !subject.TrueFindingID.Available() {
				return false
			}
			report.AppendFinding(NewFindingRow(subject.TrueFindingID, subject.ID, DiagnosticCodeAlwaysTrueGuard, trueSeverity, subject.Location, EmptyTemplateData()))
		}
		if polarity == guardPolarityFalse && falseSeverity.Available() {
			if !subject.FalseFindingID.Available() {
				return false
			}
			report.AppendFinding(NewFindingRow(subject.FalseFindingID, subject.ID, DiagnosticCodeAlwaysFalseGuard, falseSeverity, subject.Location, EmptyTemplateData()))
		}
	}
	return true
}

// CollectBranch dispatches every installed branch collector from the same
// registry that admits its policy rule. An enabled spec without an
// implementation fails closed; disabled specs perform no Engine work.
func CollectBranch(
	report *DiagnosticReport,
	policy DiagnosticPolicy,
	guards []GuardSubject,
	guardRows []ObservationKey,
	valueWidth int,
	calls []CallArgumentSubject,
	summaries []QuerySummaryKey,
	schema *valuedomain.Schema,
	published *snapshot.Snapshot,
	queryPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	selects ChannelSelectInput,
) bool {
	if report == nil {
		return false
	}
	directory, directoryOK := composite.DiagnosticCollectionDirectory()
	if !directoryOK {
		return false
	}
	var trueSeverity, falseSeverity, callArgumentSeverity, selectSeverity FindingSeverity
	collectGuards, collectCallArguments, collectSelect := false, false, false
	for _, row := range directory {
		severity, enabled := policy.EnabledFor(row.Code)
		if !enabled {
			continue
		}
		switch {
		case row.Collection.Surface == declschema.SurfaceKindObservation:
			switch row.Code {
			case DiagnosticCodeAlwaysTrueGuard:
				trueSeverity, collectGuards = severity, true
			case DiagnosticCodeAlwaysFalseGuard:
				falseSeverity, collectGuards = severity, true
			case DiagnosticCodeChannelSelectExhaustiveness:
				selectSeverity, collectSelect = severity, true
			default:
				return false
			}
		case row.Collection.Surface == declschema.SurfaceKindQuery && row.Site == schemadiag.SiteCallArgument:
			callArgumentSeverity, collectCallArguments = severity, true
		case row.Collection.Surface == declschema.SurfaceKindQuery && row.Site == schemadiag.SiteAssignment:
		default:
			return false
		}
	}
	if collectGuards && (schema == nil || published == nil || !CollectGuardPolarity(report, guards, guardRows, valueWidth, schema, published, observationPlan, trueSeverity, falseSeverity)) {
		return false
	}
	if collectCallArguments && (schema == nil || published == nil || !CollectCallArguments(report, calls, summaries, valueWidth, schema, published, queryPlan, callArgumentSeverity)) {
		return false
	}
	if collectSelect && !CollectChannelSelect(report, selects, selectSeverity) {
		return false
	}
	return true
}

// CollectReport projects sealed observation rows into collector subjects and
// runs every enabled branch and static collector.
func CollectReport(
	report *DiagnosticReport,
	policy DiagnosticPolicy,
	branches, statics []Observation,
	selected []ObservationKey,
	summaries []QuerySummaryKey,
	valueWidth int,
	schema *valuedomain.Schema,
	published *snapshot.Snapshot,
	queryPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	selects ChannelSelectInput,
) bool {
	if report == nil {
		return false
	}
	guards, guardsOK := GuardSubjects(branches)
	if !guardsOK {
		return false
	}
	calls, callsOK := callArgumentSubjects(statics)
	if !callsOK {
		return false
	}
	staticSubjects, staticsOK := staticSubjects(statics)
	if !staticsOK {
		return false
	}
	if !CollectBranch(report, policy, guards, selected, valueWidth, calls, summaries, schema, published, queryPlan, observationPlan, selects) {
		return false
	}
	if !CollectStatic(report, staticSubjects, policy) {
		return false
	}
	report.SortFindingsByID()
	return report.Available()
}

// GuardSubjects projects sealed branch rows into polarity subjects.
func GuardSubjects(rows []Observation) ([]GuardSubject, bool) {
	subjects := make([]GuardSubject, 0, len(rows))
	for _, row := range rows {
		if row.Kind != structure.DiagnosticObservationBranchCondition {
			continue
		}
		if !row.Available() || len(row.Points) == 0 {
			return nil, false
		}
		location, locationOK := NewLocation(row.Location.File, row.Location.StartLine, row.Location.StartCol, row.Location.EndLine, row.Location.EndCol)
		trueID, trueOK := RowID("diagnostic-finding", row.Mount, row.Artifact, row.Local)
		falseID, falseOK := RowID("diagnostic-finding/always-false-guard", row.Mount, row.Artifact, row.Local)
		if !locationOK || !trueOK || !falseOK {
			return nil, false
		}
		subjects = append(subjects, GuardSubject{
			ID: row.ID, TrueFindingID: trueID, FalseFindingID: falseID,
			Mount: row.Mount, Location: location, ValueIndex: row.ValueIndex,
			Points: append([]identity.ContentID(nil), row.Points...),
		})
	}
	return subjects, true
}

func callArgumentSubjects(rows []Observation) ([]CallArgumentSubject, bool) {
	subjects := make([]CallArgumentSubject, 0)
	for _, row := range rows {
		if row.Kind != structure.DiagnosticObservationTypeConformance {
			continue
		}
		if !row.Available() || !row.Conformance.Available() {
			return nil, false
		}
		if row.Site != schemadiag.SiteCallArgument {
			continue
		}
		location, locationOK := NewLocation(row.Location.File, row.Location.StartLine, row.Location.StartCol, row.Location.EndLine, row.Location.EndCol)
		id, idOK := RowID("diagnostic-finding/type-call-argument", row.Mount, row.Artifact, row.Local)
		if !locationOK || !idOK {
			return nil, false
		}
		subjects = append(subjects, CallArgumentSubject{
			ID: row.ID, FindingID: id, Mount: row.Mount,
			Location: location, Site: row.Site, Actual: row.Actual,
			DeclaredMay: row.DeclaredMay, Target: row.Target,
			Evidence: append([]identity.ContentID(nil), row.Evidence...),
		})
	}
	return subjects, true
}

func staticSubjects(rows []Observation) ([]StaticSubject, bool) {
	subjects := make([]StaticSubject, 0, len(rows))
	for _, row := range rows {
		if !row.Available() {
			return nil, false
		}
		if row.Kind == structure.DiagnosticObservationTypeConformance {
			continue
		}
		location, locationOK := NewLocation(row.Location.File, row.Location.StartLine, row.Location.StartCol, row.Location.EndLine, row.Location.EndCol)
		id, idOK := RowID("diagnostic-finding", row.Mount, row.Artifact, row.Local)
		if !locationOK || !idOK {
			return nil, false
		}
		name := row.Name
		if row.Kind == structure.DiagnosticObservationTypeReferenceUnresolved {
			name = strings.Join(row.Path, ".")
		}
		subjects = append(subjects, StaticSubject{
			ID: row.ID, FindingID: id, Location: location,
			Kind: row.Kind, Name: name,
		})
	}
	return subjects, true
}
