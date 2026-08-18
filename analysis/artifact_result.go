package analysis

// This file projects immutable ProgramArtifact geometry and committed Snapshot
// rows into the public Result.

import (
	"bytes"
	"crypto/sha256"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	declschema "github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
	"github.com/wippyai/go-lua/domain/composite"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/conformance"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const artifactResultIdentityDomain = "analysis/artifact-result/mounted-row/v1"

// artifactResultProjection is an immutable, detached result. Result
// is retained separately so a future caller can attach diagnostics without
// introducing engine or domain handles into the public projection.
type artifactResultProjection struct {
	result *Result
	report *DiagnosticReport
}

// detachArtifactResult consumes owner-issued publication keys and the exact
// committed Snapshot. It is intentionally unexported: the
// production Solve lane remains the owner of the transaction and must choose
// when this detached projection becomes public.
//
// Root rows come only from the mount-qualified result geometry. The
// projection never reopens Link, Program, Source, or Flow to recover them.
func detachArtifactResult(
	geometry resultGeometry,
	mounts []mountedProgramArtifact,
	valueSchema *valuedomain.Schema,
	policy *DiagnosticPolicy,
	queries []artifactQueryPublication,
	diagnosticObservations []artifactDiagnosticObservationPublication,
	published *snapshot.Snapshot,
	queryPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
) (*artifactResultProjection, bool) {
	if !geometry.valid() || len(queries) == 0 || published == nil || !published.Published() || !queryPlan.Available() || !observationPlan.Available() {
		return nil, false
	}
	native, nativeOK := buildNativeBranchPublication(geometry, mounts, diagnosticObservations, valueSchema, published, observationPlan)
	if !nativeOK {
		return nil, false
	}
	result, ok := buildDetachedArtifactResult(geometry, queries, published, queryPlan, native)
	if !ok || result == nil {
		return nil, false
	}
	projection := &artifactResultProjection{result: result}
	if policy != nil && len(policy.Enabled) != 0 {
		report, reportOK := collectDiagnosticReport(geometry, diagnosticObservations, valueSchema, published, queries, queryPlan, observationPlan, result, *policy)
		if !reportOK {
			return nil, false
		}
		projection.report = report
	}
	return projection, true
}

// collectDiagnosticReport projects Analysis-owned findings from the one
// mount-qualified observation carrier. Branch rows use a solve observation;
// static rows have already been sealed by ProgramArtifact and are deliberately
// never attached to, or queried from, Engine.
func collectDiagnosticReport(geometry resultGeometry, selected []artifactDiagnosticObservationPublication, schema *valuedomain.Schema, published *snapshot.Snapshot, queries []artifactQueryPublication, queryPlan snapshot.QueryPlan[identity.ContentID, engine.Answer], observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer], result *Result, policy DiagnosticPolicy) (*DiagnosticReport, bool) {
	if !geometry.valid() || result == nil || !result.valid() {
		return nil, false
	}
	report := &DiagnosticReport{source: result.SourceID(), result: result.ContentID(), findings: make([]diagnosticFinding, 0), sealed: true}
	if !collectBranchDiagnosticFindings(report, geometry, selected, schema, published, queries, queryPlan, observationPlan, policy) {
		return nil, false
	}
	if !collectStaticDiagnosticFindings(report, geometry, policy) {
		return nil, false
	}
	sort.Slice(report.findings, func(i, j int) bool { return bytes.Compare(report.findings[i].id[:], report.findings[j].id[:]) < 0 })
	return report, report.Available()
}

// collectBranchDiagnosticFindings dispatches every installed branch collector
// from the same registry that admits its policy rule. An enabled spec without
// an implementation fails closed; disabled specs perform no Engine work.
func collectBranchDiagnosticFindings(report *DiagnosticReport, geometry resultGeometry, selected []artifactDiagnosticObservationPublication, schema *valuedomain.Schema, published *snapshot.Snapshot, queries []artifactQueryPublication, queryPlan snapshot.QueryPlan[identity.ContentID, engine.Answer], observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer], policy DiagnosticPolicy) bool {
	if report == nil || !geometry.valid() {
		return false
	}
	directory, directoryOK := composite.DiagnosticCollectionDirectory()
	if !directoryOK {
		return false
	}
	var trueSeverity, falseSeverity, callArgumentSeverity FindingSeverity
	collectGuards, collectCallArguments := false, false
	for _, row := range directory {
		severity, enabled := policy.enabled(row.Code)
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
			default:
				return false
			}
		case row.Collection.Surface == declschema.SurfaceKindQuery && row.Site == diagnostic.SiteCallArgument:
			callArgumentSeverity, collectCallArguments = severity, true
		case row.Collection.Surface == declschema.SurfaceKindQuery && row.Site == diagnostic.SiteAssignment:
			// Assignment sites are not issued. An enabled code with no matching
			// site is a clean empty report, not a missing producer.
		default:
			return false
		}
	}
	if collectGuards && (schema == nil || published == nil || !collectGuardPolarityFindings(report, geometry, selected, schema, published, observationPlan, trueSeverity, falseSeverity)) {
		return false
	}
	if collectCallArguments && (schema == nil || published == nil || !collectCallArgumentFindings(report, geometry, schema, published, queries, queryPlan, callArgumentSeverity)) {
		return false
	}
	return true
}

type diagnosticGuardPolarity uint8

const (
	diagnosticGuardPolarityInvalid diagnosticGuardPolarity = iota
	diagnosticGuardPolarityTrue
	diagnosticGuardPolarityFalse
)

// classifyDiagnosticGuardPolarity is the closed, query-independent decision
// at the end of the generic branch collector. No reachable truth rows, an
// unknown truth set, or disagreement between reachable rows proves neither
// polarity.
func classifyDiagnosticGuardPolarity(truths []valuedomain.Truth) diagnosticGuardPolarity {
	if len(truths) == 0 {
		return diagnosticGuardPolarityInvalid
	}
	var polarity diagnosticGuardPolarity
	for _, truth := range truths {
		candidate := diagnosticGuardPolarityInvalid
		switch truth {
		case valuedomain.TruthTrue:
			candidate = diagnosticGuardPolarityTrue
		case valuedomain.TruthFalse:
			candidate = diagnosticGuardPolarityFalse
		default:
			return diagnosticGuardPolarityInvalid
		}
		if polarity == diagnosticGuardPolarityInvalid {
			polarity = candidate
			continue
		}
		if polarity != candidate {
			return diagnosticGuardPolarityInvalid
		}
	}
	return polarity
}

// collectGuardPolarityFindings reads each selected generic BranchCondition
// observation once. A polarity is emitted only when every reachable row has a
// present value of that exact truthiness; mixed, unknown, missing, and
// unreachable evidence therefore remains silent for both rules.
func collectGuardPolarityFindings(report *DiagnosticReport, geometry resultGeometry, selected []artifactDiagnosticObservationPublication, schema *valuedomain.Schema, published *snapshot.Snapshot, observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer], trueSeverity, falseSeverity FindingSeverity) bool {
	if report == nil || !geometry.valid() || schema == nil || published == nil || !published.Published() || !observationPlan.Available() || (trueSeverity != FindingSeverityInvalid && !trueSeverity.Available()) || (falseSeverity != FindingSeverityInvalid && !falseSeverity.Available()) || (trueSeverity == FindingSeverityInvalid && falseSeverity == FindingSeverityInvalid) {
		return false
	}
	expected := make(map[artifactResultPoint]struct{})
	for _, subject := range geometry.branchObservations {
		if subject.kind != structure.DiagnosticObservationBranchCondition {
			continue
		}
		if !subject.available() || len(subject.points) == 0 {
			return false
		}
		for _, point := range subject.points {
			if !point.Available() {
				return false
			}
			expected[artifactResultPoint{mount: subject.mount, point: point}] = struct{}{}
		}
	}
	observations := make(map[artifactResultPoint]valuedomain.ValueSummaryObservation, len(expected))
	for _, selectedObservation := range selected {
		key := selectedObservation.point
		if _, expected := expected[key]; !expected {
			report.collectionFailure = DiagnosticCollectionSubjectQueryAbsent
			return true
		}
		if _, duplicate := observations[key]; duplicate {
			report.collectionFailure = DiagnosticCollectionSubjectQueryAbsent
			return true
		}
		observationID := selectedObservation.key
		observation, readable := publishedObservation[valuedomain.ValueSummaryObservation](published, observationPlan, observationID)
		if !observationID.Available() || !readable {
			report.collectionFailure = DiagnosticCollectionQueryUnreadable
			return true
		}
		if !observation.Valid {
			report.collectionFailure = DiagnosticCollectionQueryInvalid
			return true
		}
		if len(observation.Values) != len(observation.Present) || len(observation.Values) != len(geometry.values) || observation.Rows > 1 {
			report.collectionFailure = DiagnosticCollectionValueShapeMismatch
			return true
		}
		observations[key] = observation
	}
	if len(observations) != len(expected) {
		report.collectionFailure = DiagnosticCollectionSubjectQueryAbsent
		return true
	}
	for _, subject := range geometry.branchObservations {
		if subject.kind != structure.DiagnosticObservationBranchCondition {
			continue
		}
		if !subject.available() || len(subject.points) == 0 {
			return false
		}
		index := int(subject.valueIndex)
		truths := make([]valuedomain.Truth, 0, len(subject.points))
		invalidEvidence := false
		for _, point := range subject.points {
			observation, observed := observations[artifactResultPoint{mount: subject.mount, point: point}]
			if !observed {
				report.collectionFailure = DiagnosticCollectionSubjectQueryAbsent
				invalidEvidence = true
				break
			}
			if observation.Rows == 0 {
				continue
			}
			if index < 0 || index >= len(observation.Values) {
				report.collectionFailure = DiagnosticCollectionValueShapeMismatch
				invalidEvidence = true
				break
			}
			if !observation.Present[index] {
				// A generic semantic producer may lawfully have no candidate at
				// this reachable member (for example, an operand producer has not
				// yet established both comparison inputs). That is ordinary
				// undecided evidence, not a malformed observation, and must
				// never poison unrelated diagnostic observations.
				invalidEvidence = true
				break
			}
			truths = append(truths, schema.Truthiness(observation.Values[index]))
		}
		if invalidEvidence {
			continue
		}
		polarity := classifyDiagnosticGuardPolarity(truths)
		if polarity == diagnosticGuardPolarityInvalid {
			continue
		}
		location, locationOK := newDiagnosticLocation(subject.location.File, subject.location.StartLine, subject.location.StartCol, subject.location.EndLine, subject.location.EndCol)
		if !locationOK {
			return false
		}
		if polarity == diagnosticGuardPolarityTrue && trueSeverity.Available() {
			id, idOK := mountedResultID("diagnostic-finding", subject.mount, subject.artifact, subject.local)
			if !idOK {
				return false
			}
			report.findings = append(report.findings, diagnosticFinding{id: id, subject: subject.id, code: DiagnosticCodeAlwaysTrueGuard, severity: trueSeverity, location: location})
		}
		if polarity == diagnosticGuardPolarityFalse && falseSeverity.Available() {
			id, idOK := mountedResultID("diagnostic-finding/always-false-guard", subject.mount, subject.artifact, subject.local)
			if !idOK {
				return false
			}
			report.findings = append(report.findings, diagnosticFinding{id: id, subject: subject.id, code: DiagnosticCodeAlwaysFalseGuard, severity: falseSeverity, location: location})
		}
	}
	return true
}

// collectCallArgumentFindings reads each issued TypeConformance call-argument
// site against the QueryFamily value summary at its evidence points. The
// judgment is MayKindConformance: only a family outside the formal's declared
// may-set becomes a finding. Empty, All, and unprojected declarations abstain.
func collectCallArgumentFindings(report *DiagnosticReport, geometry resultGeometry, schema *valuedomain.Schema, published *snapshot.Snapshot, queries []artifactQueryPublication, queryPlan snapshot.QueryPlan[identity.ContentID, engine.Answer], severity FindingSeverity) bool {
	if report == nil || !geometry.valid() || schema == nil || published == nil || !published.Published() || !queryPlan.Available() || !severity.Available() {
		return false
	}
	summaries := make(map[artifactResultPoint]identity.ContentID, len(queries))
	for _, query := range queries {
		if query.attachment.role != artifactQueryValueSummary || !query.attachment.mount.Available() || !query.attachment.point.Available() || !query.key.Available() {
			continue
		}
		key := artifactResultPoint{mount: query.attachment.mount, point: query.attachment.point}
		if _, duplicate := summaries[key]; duplicate {
			return false
		}
		summaries[key] = query.key
	}
	for _, observation := range geometry.staticObservations {
		if observation.kind != structure.DiagnosticObservationTypeConformance {
			continue
		}
		if !observation.available() || !observation.compiledTypeConformance.available() {
			return false
		}
		payload := observation.compiledTypeConformance
		if payload.site != diagnostic.SiteCallArgument {
			continue
		}
		if payload.declaredMay == runtimekind.All {
			continue
		}
		var observed runtimekind.Set
		anyEvidence := false
		for _, point := range payload.evidence {
			publicationKey, known := summaries[artifactResultPoint{mount: observation.mount, point: point}]
			if !known {
				report.collectionFailure = DiagnosticCollectionSubjectQueryAbsent
				return true
			}
			summary, readable := publishedObservation[valuedomain.ValueSummaryObservation](published, queryPlan, publicationKey)
			if !publicationKey.Available() || !readable {
				report.collectionFailure = DiagnosticCollectionQueryUnreadable
				return true
			}
			if !summary.Valid {
				report.collectionFailure = DiagnosticCollectionQueryInvalid
				return true
			}
			if len(summary.Values) != len(summary.Present) || len(summary.Values) != len(geometry.values) || summary.Rows > 1 {
				report.collectionFailure = DiagnosticCollectionValueShapeMismatch
				return true
			}
			if summary.Rows == 0 {
				continue
			}
			index := int(payload.actual)
			if index < 0 || index >= len(summary.Values) {
				report.collectionFailure = DiagnosticCollectionValueShapeMismatch
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
		switch conformance.MayKindConformance(payload.declaredMay, observed) {
		case conformance.VerdictViolates:
			if !appendCallArgumentFinding(report, observation, payload.target, severity) {
				return false
			}
		case conformance.VerdictConforms, conformance.VerdictAbstain:
		default:
			return false
		}
	}
	return true
}

func appendCallArgumentFinding(report *DiagnosticReport, observation compiledObservation, targetName string, severity FindingSeverity) bool {
	if report == nil || observation.kind != structure.DiagnosticObservationTypeConformance || !observation.available() || !severity.Available() {
		return false
	}
	subject, subjectOK := newDiagnosticSemanticName("argument")
	target, targetOK := newDiagnosticTargetType(targetName)
	id, idOK := mountedResultID("diagnostic-finding/type-call-argument", observation.mount, observation.artifact, observation.local)
	location, locationOK := newDiagnosticLocation(observation.location.File, observation.location.StartLine, observation.location.StartCol, observation.location.EndLine, observation.location.EndCol)
	if !subjectOK || !targetOK || !idOK || !locationOK {
		return false
	}
	table, tableOK := composite.Diagnostics()
	if !tableOK {
		return false
	}
	declared, declaredOK := table.ForBranchObservation(structure.DiagnosticObservationTypeConformance.Key(), observation.compiledTypeConformance.site)
	data := diagnosticTemplateData{subject: subject, target: target}
	if !declaredOK || !declared.Site().Available() || !data.validFor(declared) {
		return false
	}
	report.findings = append(report.findings, diagnosticFinding{
		id: id, subject: observation.id, code: declared.Code(), severity: severity, location: location,
		data: data,
	})
	return true
}

// staticDiagnosticDeclaration resolves the declared row one artifact-issued
// observation population feeds. The sealed table is the sole authority: a
// population no row claims is a collector hole, not a row to skip.
//
// The canonical structure kind projects through the sealed structural table;
// the diagnostic row is then found by that member's schema identity.
func staticDiagnosticDeclaration(kind structure.DiagnosticObservationKind) (*diagnostic.Entry, bool) {
	table, tableOK := composite.Diagnostics()
	vocabulary, vocabularyOK := composite.StructureVocabulary()
	if !tableOK || !vocabularyOK {
		return nil, false
	}
	population, populationOK := structure.DiagnosticObservationEntry(vocabulary, kind)
	if !populationOK {
		return nil, false
	}
	return table.ForStaticObservation(population.Key())
}

// collectStaticDiagnosticFindings owns policy selection for static rows. It
// is the generic collection point for all current and future owner-issued
// static observations; a row without an enabled matching projector remains a
// no-op, rather than being reverse-engineered from Engine state.
func collectStaticDiagnosticFindings(report *DiagnosticReport, geometry resultGeometry, policy DiagnosticPolicy) bool {
	if report == nil || !geometry.valid() {
		return false
	}
	for _, observation := range geometry.staticObservations {
		if !observation.available() {
			return false
		}
		if observation.kind == structure.DiagnosticObservationTypeConformance {
			continue
		}
		entry, known := staticDiagnosticDeclaration(observation.kind)
		if !known {
			// A row that the artifact carrier admits but this Analysis collector
			// does not recognize is not safely ignorable: it could otherwise turn
			// an enabled policy into a false-clean report.
			return false
		}
		severity, enabled := policy.enabled(entry.Code())
		if !enabled {
			continue
		}
		switch entry.Code() {
		case DiagnosticCodeUnresolvedTypeReference:
			if !appendStaticUnresolvedTypeFinding(report, observation, severity) {
				return false
			}
		case DiagnosticCodeUnresolvedValueReference:
			if !appendStaticUnresolvedValueFinding(report, observation, severity) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// appendStaticUnresolvedValueFinding consumes Link's exact absence-filtered
// implicit-global geometry. It needs no Engine observation: Program issued the
// read/cell evidence and Link proved the name had no configured global.
func appendStaticUnresolvedValueFinding(report *DiagnosticReport, observation compiledObservation, severity FindingSeverity) bool {
	if report == nil || observation.kind != structure.DiagnosticObservationValueReferenceUnresolved || !observation.available() || !severity.Available() {
		return false
	}
	name, nameOK := newDiagnosticSemanticName(observation.name)
	id, idOK := mountedResultID("diagnostic-finding", observation.mount, observation.artifact, observation.local)
	location, locationOK := newDiagnosticLocation(observation.location.File, observation.location.StartLine, observation.location.StartCol, observation.location.EndLine, observation.location.EndCol)
	if !nameOK || !idOK || !locationOK {
		return false
	}
	report.findings = append(report.findings, diagnosticFinding{
		id: id, subject: observation.id, code: DiagnosticCodeUnresolvedValueReference, severity: severity, location: location,
		data: diagnosticTemplateData{subject: name},
	})
	return true
}

// appendStaticUnresolvedTypeFinding is intentionally a static projection: the
// unresolved disposition, exact authored span, and typed reference path were
// issued by ProgramArtifact before it was mounted. It has no Engine dependency
// and cannot add an Engine observation or affect the solve.
func appendStaticUnresolvedTypeFinding(report *DiagnosticReport, observation compiledObservation, severity FindingSeverity) bool {
	if report == nil || observation.kind != structure.DiagnosticObservationTypeReferenceUnresolved || !observation.available() || !severity.Available() {
		return false
	}
	name, nameOK := newDiagnosticSemanticName(strings.Join(observation.path, "."))
	id, idOK := mountedResultID("diagnostic-finding", observation.mount, observation.artifact, observation.local)
	location, locationOK := newDiagnosticLocation(observation.location.File, observation.location.StartLine, observation.location.StartCol, observation.location.EndLine, observation.location.EndCol)
	if !nameOK || !idOK || !locationOK {
		return false
	}
	report.findings = append(report.findings, diagnosticFinding{
		id: id, subject: observation.id, code: DiagnosticCodeUnresolvedTypeReference, severity: severity, location: location,
		data: diagnosticTemplateData{subject: name},
	})
	return true
}

func publishedObservation[R any](published *snapshot.Snapshot, plan snapshot.QueryPlan[identity.ContentID, engine.Answer], id identity.ContentID) (R, bool) {
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

func buildDetachedArtifactResult(
	geometry resultGeometry,
	queries []artifactQueryPublication,
	published *snapshot.Snapshot,
	plan snapshot.QueryPlan[identity.ContentID, engine.Answer],
	native *nativePublicationReceipt,
) (*Result, bool) {
	if !geometry.valid() || published == nil || !published.Published() || !plan.Available() || native == nil || !native.valid() {
		return nil, false
	}
	values := append([]identity.ContentID(nil), geometry.values...)
	bodies := make([]resultBody, len(geometry.bodies))
	for index, body := range geometry.bodies {
		bodies[index] = resultBody{id: body.id, roots: append([]resultRoot(nil), body.roots...), valuePresence: make([]uint64, resultValueWordCount(len(values)))}
	}
	for _, query := range queries {
		key := artifactResultPoint{mount: query.attachment.mount, point: query.attachment.point}
		indexes := geometry.pointBodies[key]
		answer, status := snapshot.Query(published, plan, query.key)
		if status == snapshot.ReadProvenAbsent {
			continue
		}
		if status != snapshot.ReadHit || !answer.Available() {
			return nil, false
		}
		switch query.attachment.role {
		case artifactQueryValueSummary:
			observation, readable := engine.AnswerValue[valuedomain.ValueSummaryObservation](answer)
			if !readable || !observation.Valid {
				return nil, false
			}
			count := len(observation.Values)
			if len(observation.Present) != count || count != len(geometry.values) || observation.Rows > 1 {
				return nil, false
			}
			if observation.Rows == 0 {
				continue
			}
			if len(indexes) == 0 {
				for _, present := range observation.Present {
					if present {
						return nil, false
					}
				}
				continue
			}
			for _, bodyIndex := range indexes {
				if bodyIndex < 0 || bodyIndex >= len(bodies) || len(observation.Present) != len(values) {
					return nil, false
				}
				for valueIndex, present := range observation.Present {
					if present && !setResultValuePresent(bodies[bodyIndex].valuePresence, valueIndex) {
						return nil, false
					}
				}
			}
		case artifactQueryEffectExact:
			observation, readable := engine.AnswerValue[effectfactor.EffectObservation](answer)
			if !readable || !observation.Valid {
				return nil, false
			}
			if observation.Rows == 0 {
				continue
			}
			if len(indexes) == 0 {
				if observation.Present {
					return nil, false
				}
				continue
			}
			if observation.Rows != 1 {
				return nil, false
			}
			for _, bodyIndex := range indexes {
				bodies[bodyIndex].effectPresent = bodies[bodyIndex].effectPresent || observation.Present
				bodies[bodyIndex].effectTop = bodies[bodyIndex].effectTop || observation.Top
				if !observation.Top {
					bodies[bodyIndex].effects = appendUniqueIDs(bodies[bodyIndex].effects, observation.Atoms)
				}
			}
		default:
			return nil, false
		}
	}
	for index := range bodies {
		if bodies[index].effectTop {
			bodies[index].effects = nil
		} else {
			sort.Slice(bodies[index].effects, func(left, right int) bool {
				return bytes.Compare(bodies[index].effects[left][:], bodies[index].effects[right][:]) < 0
			})
		}
	}
	content, ok := analysisResultIDWithPublication(geometry.source, values, bodies, native)
	if !ok {
		return nil, false
	}
	result := &Result{source: geometry.source, content: content, values: values, bodies: bodies, native: native}
	if !result.validPayload() {
		return nil, false
	}
	result.sealed = true
	return result, true
}

type artifactResultPoint struct {
	mount identity.ContentID
	point identity.ContentID
}

type artifactResultBody struct {
	mount identity.ContentID
	body  identity.ContentID
}

func appendUniqueInt(values []int, value int) []int {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueIDs(values, additions []identity.ContentID) []identity.ContentID {
	for _, addition := range additions {
		if !addition.Available() {
			continue
		}
		seen := false
		for _, value := range values {
			if value == addition {
				seen = true
				break
			}
		}
		if !seen {
			values = append(values, addition)
		}
	}
	return values
}

func mountedResultID(role string, mount, artifact, local identity.ContentID) (identity.ContentID, bool) {
	if role == "" || !mount.Available() || !artifact.Available() || !local.Available() {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(artifactResultIdentityDomain))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(role))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(mount[:])
	_, _ = hash.Write(artifact[:])
	_, _ = hash.Write(local[:])
	return identity.ContentID(hash.Sum(nil)), true
}

// resultGeometry is the mount-qualified body/value/observation projection
// built from ProgramArtifact mounts and Link value substitution. It is
// computed when Result is detached; compiledState does not retain it.
type resultGeometry struct {
	source             identity.ContentID
	bodies             []resultGeometryBody
	values             []identity.ContentID
	branchObservations []compiledObservation
	staticObservations []compiledObservation
	pointBodies        map[artifactResultPoint][]int
}

type resultGeometryBody struct {
	key   artifactResultBody
	id    identity.ContentID
	roots []resultRoot
}

func projectArtifactResult(
	sourceID identity.ContentID,
	mounts []mountedProgramArtifact,
	coordinates []compiledValueCoordinate,
	observations []compiledObservation,
) (resultGeometry, bool) {
	if !sourceID.Available() || len(mounts) == 0 || len(coordinates) == 0 {
		return resultGeometry{}, false
	}
	geometry := resultGeometry{
		source:             sourceID,
		bodies:             make([]resultGeometryBody, 0),
		values:             make([]identity.ContentID, len(coordinates)),
		branchObservations: make([]compiledObservation, 0, len(observations)),
		staticObservations: make([]compiledObservation, 0, len(observations)),
		pointBodies:        make(map[artifactResultPoint][]int),
	}
	for _, observation := range observations {
		if !observation.available() {
			return resultGeometry{}, false
		}
		copy := observation
		copy.points = append([]identity.ContentID(nil), observation.points...)
		copy.producers = append([]compiledObservationProducer(nil), observation.producers...)
		copy.path = append([]string(nil), observation.path...)
		switch observation.kind {
		case structure.DiagnosticObservationBranchCondition:
			geometry.branchObservations = append(geometry.branchObservations, copy)
		case structure.DiagnosticObservationTypeReferenceUnresolved, structure.DiagnosticObservationValueReferenceUnresolved, structure.DiagnosticObservationTypeConformance:
			geometry.staticObservations = append(geometry.staticObservations, copy)
		default:
			return resultGeometry{}, false
		}
	}
	artifactIDs := make(map[identity.ContentID]identity.ContentID, len(mounts))
	bodyIndexes := make(map[artifactResultBody]int)
	for _, mount := range mounts {
		if mount.artifact == nil || !mount.artifact.Available() || !mount.moduleKey.Available() || !mount.artifact.ID().Available() {
			return resultGeometry{}, false
		}
		if _, duplicate := artifactIDs[mount.moduleKey]; duplicate {
			return resultGeometry{}, false
		}
		artifactIDs[mount.moduleKey] = mount.artifact.ID()
		localBodies := make(map[identity.ContentID]int)
		for bodyIndex := 0; bodyIndex < mount.artifact.BodyCount(); bodyIndex++ {
			body, bodyOK := mount.artifact.BodyAt(bodyIndex)
			if !bodyOK || !body.Available() || !body.ID().Available() {
				return resultGeometry{}, false
			}
			key := artifactResultBody{mount: mount.moduleKey, body: body.ID()}
			id, idOK := mountedResultID("body", mount.moduleKey, mount.artifact.ID(), body.ID())
			if !idOK {
				return resultGeometry{}, false
			}
			if _, duplicate := localBodies[body.ID()]; duplicate {
				return resultGeometry{}, false
			}
			if _, duplicate := bodyIndexes[key]; duplicate {
				return resultGeometry{}, false
			}
			localBodies[body.ID()] = len(geometry.bodies)
			bodyIndexes[key] = len(geometry.bodies)
			roots := make([]resultRoot, body.RootCount())
			seenRoots := make(map[identity.ContentID]struct{}, len(roots))
			for rootIndex := range roots {
				root, rootOK := body.RootAt(rootIndex)
				if !rootOK || !root.Available() || !root.ID().Available() || root.Family() == keyspace.FamilyInvalid {
					return resultGeometry{}, false
				}
				rootID, rootIDOK := mountedResultID("root", mount.moduleKey, mount.artifact.ID(), root.ID())
				if !rootIDOK {
					return resultGeometry{}, false
				}
				if _, duplicate := seenRoots[rootID]; duplicate {
					return resultGeometry{}, false
				}
				seenRoots[rootID] = struct{}{}
				roots[rootIndex] = resultRoot{id: rootID, family: root.Family()}
			}
			geometry.bodies = append(geometry.bodies, resultGeometryBody{key: key, id: id, roots: roots})
			if body.Callable() {
				continue
			}
			entryBody := localBodies[body.ID()]
			for entryIndex := 0; entryIndex < body.EntryPointCount(); entryIndex++ {
				entry, entryOK := body.EntryPointAt(entryIndex)
				if !entryOK || !entry.Available() {
					continue
				}
				pointKey := artifactResultPoint{mount: mount.moduleKey, point: entry}
				geometry.pointBodies[pointKey] = appendUniqueInt(geometry.pointBodies[pointKey], entryBody)
			}
		}
		for occurrenceIndex := 0; occurrenceIndex < mount.artifact.OccurrenceCount(); occurrenceIndex++ {
			occurrence, occurrenceOK := mount.artifact.OccurrenceAt(occurrenceIndex)
			if !occurrenceOK || !occurrence.Available() {
				return resultGeometry{}, false
			}
			bodyID, bodyOK := occurrence.BodyID()
			if !bodyOK {
				continue
			}
			mapped, bodyKnown := localBodies[bodyID]
			if !bodyKnown {
				return resultGeometry{}, false
			}
			for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
				point, pointOK := occurrence.PointAt(pointIndex)
				if !pointOK || !point.Available() {
					return resultGeometry{}, false
				}
				pointKey := artifactResultPoint{mount: mount.moduleKey, point: point}
				geometry.pointBodies[pointKey] = appendUniqueInt(geometry.pointBodies[pointKey], mapped)
			}
		}
	}
	for index, coordinate := range coordinates {
		if !coordinate.id.Available() || !coordinate.mount.Available() {
			return resultGeometry{}, false
		}
		artifactID, artifactOK := artifactIDs[coordinate.mount]
		id, idOK := mountedResultID("value", coordinate.mount, artifactID, coordinate.id)
		if !artifactOK || !idOK {
			return resultGeometry{}, false
		}
		geometry.values[index] = id
	}
	for _, observation := range geometry.branchObservations {
		if !observation.id.Available() || !observation.mount.Available() || !observation.artifact.Available() ||
			!observation.local.Available() || len(observation.producers) == 0 || uint64(observation.valueIndex) >= uint64(len(coordinates)) ||
			observation.kind != structure.DiagnosticObservationBranchCondition {
			return resultGeometry{}, false
		}
		artifactID, artifactOK := artifactIDs[observation.mount]
		if !artifactOK || artifactID != observation.artifact {
			return resultGeometry{}, false
		}
		coordinate := coordinates[observation.valueIndex]
		if coordinate.mount != observation.mount {
			return resultGeometry{}, false
		}
		seenPoints := make(map[identity.ContentID]struct{}, len(observation.points))
		for _, point := range observation.points {
			if !point.Available() {
				return resultGeometry{}, false
			}
			if _, duplicate := seenPoints[point]; duplicate {
				return resultGeometry{}, false
			}
			seenPoints[point] = struct{}{}
		}
		seenAnchors := make(map[identity.ContentID]struct{}, len(observation.producers))
		seenExecution := make(map[identity.ContentID]struct{}, len(observation.producers))
		for _, producer := range observation.producers {
			if !producer.key.Available() || !producer.occurrence.Available() || !producer.point.Available() || !producer.anchor.Available() {
				return resultGeometry{}, false
			}
			if _, known := seenPoints[producer.anchor]; !known {
				return resultGeometry{}, false
			}
			if _, duplicate := seenAnchors[producer.anchor]; duplicate {
				return resultGeometry{}, false
			}
			if _, duplicate := seenExecution[producer.point]; duplicate {
				return resultGeometry{}, false
			}
			seenAnchors[producer.anchor] = struct{}{}
			seenExecution[producer.point] = struct{}{}
		}
		if len(seenAnchors) != len(seenPoints) {
			return resultGeometry{}, false
		}
	}
	for _, observation := range geometry.staticObservations {
		if !observation.available() {
			return resultGeometry{}, false
		}
		switch observation.kind {
		case structure.DiagnosticObservationTypeReferenceUnresolved:
			if !observation.reference.Available() || len(observation.path) == 0 {
				return resultGeometry{}, false
			}
		case structure.DiagnosticObservationValueReferenceUnresolved:
			if !observation.read.Available() || !observation.cell.Available() || observation.name == "" {
				return resultGeometry{}, false
			}
		case structure.DiagnosticObservationTypeConformance:
			if !observation.compiledTypeConformance.available() {
				return resultGeometry{}, false
			}
		default:
			return resultGeometry{}, false
		}
		artifactID, artifactOK := artifactIDs[observation.mount]
		if !artifactOK || artifactID != observation.artifact {
			return resultGeometry{}, false
		}
	}
	return geometry, geometry.valid()
}

func (geometry resultGeometry) valid() bool {
	return geometry.source.Available() && len(geometry.bodies) != 0 && len(geometry.values) != 0 &&
		geometry.pointBodies != nil
}
