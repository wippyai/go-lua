package diagnostic

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	declschema "github.com/wippyai/go-lua/analysis/schema"
	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

type pointKey struct {
	mount, point, context identity.ContentID
}

// ObservationKey is one Snapshot row address for a mounted evidence point.
type ObservationKey struct {
	Mount, Point, Key identity.ContentID
	// Context is the exact execution Context that owns Key. It is part of the
	// canonical observation address and is never inferred by a consumer.
	Context identity.ContentID
}

// GuardSubject is one branch-condition site the polarity collector reads.
type GuardSubject struct {
	ID, TrueFindingID, FalseFindingID, Mount identity.ContentID
	Context                                  executioncontext.Context
	Location                                 DiagnosticLocation
	ValueID                                  identity.ContentID
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

// PublishedBranchConditionTruth reads the owner-issued ValueID from one
// published Value summary. The dense summary image is private to Value; the
// diagnostic collector never carries a ValueSchema or a coordinate ordinal.
func PublishedBranchConditionTruth(valueID identity.ContentID, observation valuedomain.ValueSummaryObservation) (valuedomain.Truth, bool) {
	truth, present, valid := observation.TruthinessAtID(valueID)
	return truth, valid && present
}

// CollectGuardPolarity reads each selected branch observation once. A polarity
// is emitted only when every reachable row has a present value of that exact
// truthiness.
func CollectGuardPolarity(
	report *DiagnosticReport,
	subjects []GuardSubject,
	selected []ObservationKey,
	published *snapshot.Snapshot,
	observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	trueSeverity, falseSeverity FindingSeverity,
) bool {
	if report == nil || published == nil || !published.Published() || !observationPlan.Available() ||
		(trueSeverity != FindingSeverityInvalid && !trueSeverity.Available()) ||
		(falseSeverity != FindingSeverityInvalid && !falseSeverity.Available()) ||
		(trueSeverity == FindingSeverityInvalid && falseSeverity == FindingSeverityInvalid) {
		return false
	}
	expected := make(map[pointKey]struct{})
	for _, subject := range subjects {
		if !subject.ID.Available() || !subject.Mount.Available() || !subject.Context.Available() || !subject.Location.Available() || !subject.ValueID.Available() || len(subject.Points) == 0 {
			return false
		}
		for _, point := range subject.Points {
			if !point.Available() {
				return false
			}
			expected[pointKey{mount: subject.Mount, point: point, context: subject.Context.ID()}] = struct{}{}
		}
	}
	observations := make(map[pointKey]valuedomain.ValueSummaryObservation, len(expected))
	for _, row := range selected {
		if !row.Context.Available() {
			report.SetCollectionFailure(DiagnosticCollectionSubjectQueryAbsent)
			return true
		}
		key := pointKey{mount: row.Mount, point: row.Point, context: row.Context}
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
		if len(observation.Values) != len(observation.Present) || observation.Rows > 1 {
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
			observation, observed := observations[pointKey{mount: subject.Mount, point: point, context: subject.Context.ID()}]
			if !observed {
				report.SetCollectionFailure(DiagnosticCollectionSubjectQueryAbsent)
				invalidEvidence = true
				break
			}
			if observation.Rows == 0 {
				continue
			}
			truth, ok := PublishedBranchConditionTruth(subject.ValueID, observation)
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
	conformances []ConformanceSubject,
	published *snapshot.Snapshot,
	observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	selects ChannelSelectInput,
) bool {
	if report == nil {
		return false
	}
	directory := report.collections
	if !directory.Available() {
		return false
	}
	var trueSeverity, falseSeverity, selectSeverity FindingSeverity
	collectGuards, collectSelect := false, false
	conformanceSeverities := make(map[schemadiag.Site]FindingSeverity, 4)
	for position := 0; position < directory.Count(); position++ {
		row, rowOK := directory.At(position)
		if !rowOK {
			return false
		}
		severity, enabled := policy.EnabledFor(report.declarations, row.Code)
		if !enabled {
			continue
		}
		if row.Collection.Surface != declschema.SurfaceKindObservation {
			return false
		}
		switch {
		case row.Population == structure.DiagnosticObservationTypeConformance.Key():
			if row.SiteCount() == 0 {
				return false
			}
			for sitePosition := 0; sitePosition < row.SiteCount(); sitePosition++ {
				site, siteOK := row.SiteAt(sitePosition)
				if !siteOK || !site.Available() {
					return false
				}
				conformanceSeverities[site] = severity
			}
		case row.Code == DiagnosticCodeAlwaysTrueGuard:
			trueSeverity, collectGuards = severity, true
		case row.Code == DiagnosticCodeAlwaysFalseGuard:
			falseSeverity, collectGuards = severity, true
		case row.Code == DiagnosticCodeChannelSelectExhaustiveness:
			selectSeverity, collectSelect = severity, true
		default:
			return false
		}
	}
	if collectGuards && (published == nil || !CollectGuardPolarity(report, guards, guardRows, published, observationPlan, trueSeverity, falseSeverity)) {
		return false
	}
	if len(conformanceSeverities) != 0 && (published == nil || !CollectConformance(report, conformances, published, observationPlan, conformanceSeverities)) {
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
	branches, conformances, statics []Observation,
	branchRows []ObservationKey,
	published *snapshot.Snapshot,
	observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	selects ChannelSelectInput,
	contexts executioncontext.Directory,
) bool {
	if report == nil || !contexts.Available() {
		return false
	}
	guards, guardsOK := GuardSubjects(branches, contexts)
	if !guardsOK {
		return false
	}
	subjects, subjectsOK := conformanceSubjects(conformances, contexts)
	if !subjectsOK {
		return false
	}
	staticSubjects, staticsOK := staticSubjects(statics)
	if !staticsOK {
		return false
	}
	if !CollectBranch(report, policy, guards, branchRows, subjects, published, observationPlan, selects) {
		return false
	}
	if !CollectStatic(report, staticSubjects, policy) {
		return false
	}
	report.SortFindingsByID()
	return report.Available()
}

// GuardSubjects projects sealed branch rows into polarity subjects.
func GuardSubjects(rows []Observation, contexts executioncontext.Directory) ([]GuardSubject, bool) {
	if !contexts.Available() {
		return nil, false
	}
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
		eligible, eligibleOK := observationContexts(contexts, row.Mount)
		if !eligibleOK {
			return nil, false
		}
		for _, context := range eligible {
			subjects = append(subjects, GuardSubject{
				ID: row.ID, TrueFindingID: trueID, FalseFindingID: falseID,
				Mount: row.Mount, Context: context, Location: location, ValueID: row.Branch.ValueID,
				Points: append([]identity.ContentID(nil), row.Points...),
			})
		}
	}
	return subjects, true
}

func conformanceSubjects(rows []Observation, contexts executioncontext.Directory) ([]ConformanceSubject, bool) {
	if !contexts.Available() {
		return nil, false
	}
	subjects := make([]ConformanceSubject, 0, len(rows))
	for _, row := range rows {
		if row.Kind != structure.DiagnosticObservationTypeConformance {
			return nil, false
		}
		if !row.Available() || !row.Conformance.Available() {
			return nil, false
		}
		location, locationOK := NewLocation(row.Location.File, row.Location.StartLine, row.Location.StartCol, row.Location.EndLine, row.Location.EndCol)
		id, idOK := RowID(conformanceFindingRole(row.Site), row.Mount, row.Artifact, row.Local)
		if !locationOK || !idOK {
			return nil, false
		}
		eligible, eligibleOK := observationContexts(contexts, row.Mount)
		if !eligibleOK {
			return nil, false
		}
		for _, context := range eligible {
			subjects = append(subjects, ConformanceSubject{
				ID: row.ID, FindingID: id, Mount: row.Mount, Context: context,
				Location: location, Site: row.Site, Position: row.Position, ValueID: row.Conformance.ValueID,
				DeclaredMay: row.DeclaredMay, Target: row.Target, Member: row.Conformance.Member,
				Subject: row.Conformance.Subject, Callee: row.Conformance.Callee,
				Points: conformanceProducerPoints(row.Conformance.Producers),
			})
		}
	}
	return subjects, true
}

// conformanceProducerPoints names the occurrences whose value the subject is
// measured at.
func conformanceProducerPoints(producers []Producer) []identity.ContentID {
	points := make([]identity.ContentID, 0, len(producers))
	for _, producer := range producers {
		points = append(points, producer.Point)
	}
	return points
}

// conformanceFindingRole keeps one finding identity per site, so two sites
// reported at one observation never collide.
func conformanceFindingRole(site schemadiag.Site) string {
	switch site {
	case schemadiag.SiteAssignment:
		return "diagnostic-finding/type-assignment"
	case schemadiag.SiteMember:
		return "diagnostic-finding/type-member"
	case schemadiag.SiteMemberAbsent:
		return "diagnostic-finding/type-member-absent"
	default:
		return "diagnostic-finding/type-call-argument"
	}
}

func staticSubjects(rows []Observation) ([]StaticSubject, bool) {
	subjects := make([]StaticSubject, 0, len(rows))
	for _, row := range rows {
		if !row.Available() {
			return nil, false
		}
		if row.Kind == structure.DiagnosticObservationTypeConformance {
			return nil, false
		}
		location, locationOK := NewLocation(row.Location.File, row.Location.StartLine, row.Location.StartCol, row.Location.EndLine, row.Location.EndCol)
		id, idOK := RowID("diagnostic-finding", row.Mount, row.Artifact, row.Local)
		if !locationOK || !idOK {
			return nil, false
		}
		name := row.UnresolvedValue.Name
		if row.Kind == structure.DiagnosticObservationTypeReferenceUnresolved {
			name = row.UnresolvedType.Name
		}
		subjects = append(subjects, StaticSubject{
			ID: row.ID, FindingID: id, Location: location,
			Kind: row.Kind, Name: name,
		})
	}
	return subjects, true
}
