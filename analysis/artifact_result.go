package analysis

// This file is the receipt-native result projection.  It deliberately does
// not attempt to recover the old body/root analysis from Link or Flow: the
// only inputs are immutable ProgramArtifact rows and detached engine query
// receipts/results.

import (
	"bytes"
	"crypto/sha256"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/domain/composite"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

const artifactResultIdentityDomain = "analysis/artifact-result/mounted-row/v1"

// artifactResultQueryReceipt is the small solve-local bridge needed to read a
// query row.  ReceiptQuery is opaque; callers obtain it only from the exact
// ReceiptGraph.Query(id) lookup before entering this projection.
type artifactResultQueryReceipt struct {
	attachment artifactQueryAttachment
	query      engine.ReceiptQuery
}

// artifactResultProjection is an immutable, detached result receipt.  Result
// is retained separately so a future caller can attach diagnostics without
// introducing engine or domain handles into the public projection.
type artifactResultProjection struct {
	result *Result
	report *DiagnosticReport
}

// artifactResultProjectionReceipts is the closed projection handoff after a
// solve completes. Each populated receipt must already be owner-issued and
// source-fenced; the Result layer only authenticates and detaches it. Further
// typed surfaces (for example placement) extend this handoff rather than
// recovering facts from Engine state here.
type artifactResultProjectionReceipts struct {
	native *nativePublicationReceipt
}

func (receipts artifactResultProjectionReceipts) detachNative() (*nativePublicationReceipt, bool) {
	if receipts.native == nil || !receipts.native.valid() {
		return nil, false
	}
	return receipts.native, true
}

// detachArtifactResult consumes the new artifact query plan and the exact
// completed receipt-native solver state.  It is intentionally unexported: the
// production Solve lane remains the owner of the transaction and must choose
// when this detached projection becomes public.
//
// Root rows come only from the immutable, mount-qualified artifact result
// receipt. The projection never reopens Link, Program, Source, or Flow to
// recover them.
func detachArtifactResult(
	receipt *artifactResultReceipt,
	valueSchema *valuedomain.Schema,
	policy *DiagnosticPolicy,
	plan *artifactQueryPlan,
	diagnosticObservations []artifactDiagnosticObservationReceipt,
	graph *engine.ReceiptGraph,
	solver *engine.Solver,
	state *engine.State,
	publications artifactResultProjectionReceipts,
) (*artifactResultProjection, bool) {
	if !receipt.valid() || plan == nil || len(plan.rows) == 0 || graph == nil || solver == nil || state == nil {
		return nil, false
	}
	queries := make([]artifactResultQueryReceipt, len(plan.rows))
	for index, attachment := range plan.rows {
		if !attachment.id.Available() || !attachment.mount.Available() || !attachment.point.Available() {
			return nil, false
		}
		query, ok := graph.Query(attachment.id)
		if !ok {
			return nil, false
		}
		queries[index] = artifactResultQueryReceipt{attachment: attachment, query: query}
	}
	native, nativeOK := buildNativeBranchPublication(receipt, diagnosticObservations, valueSchema, solver, state)
	if !nativeOK {
		return nil, false
	}
	publications.native = native
	result, ok := buildDetachedArtifactResult(receipt, queries, solver, state, publications)
	if !ok || result == nil {
		return nil, false
	}
	projection := &artifactResultProjection{result: result}
	if policy != nil && len(policy.Enabled) != 0 {
		report, reportOK := collectDiagnosticReport(receipt, diagnosticObservations, valueSchema, solver, state, result, *policy)
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
func collectDiagnosticReport(receipt *artifactResultReceipt, selected []artifactDiagnosticObservationReceipt, schema *valuedomain.Schema, solver *engine.Solver, state *engine.State, result *Result, policy DiagnosticPolicy) (*DiagnosticReport, bool) {
	if receipt == nil || result == nil || !result.valid() {
		return nil, false
	}
	report := &DiagnosticReport{source: result.SourceID(), result: result.ContentID(), findings: make([]diagnosticFinding, 0), sealed: true}
	if !collectBranchDiagnosticFindings(report, receipt, selected, schema, solver, state, policy) {
		return nil, false
	}
	if !collectStaticDiagnosticFindings(report, receipt, policy) {
		return nil, false
	}
	sort.Slice(report.findings, func(i, j int) bool { return bytes.Compare(report.findings[i].id[:], report.findings[j].id[:]) < 0 })
	return report, report.Available()
}

// collectBranchDiagnosticFindings dispatches every installed branch collector
// from the same registry that admits its policy rule. An enabled spec without
// an implementation fails closed; disabled specs perform no Engine work.
func collectBranchDiagnosticFindings(report *DiagnosticReport, receipt *artifactResultReceipt, selected []artifactDiagnosticObservationReceipt, schema *valuedomain.Schema, solver *engine.Solver, state *engine.State, policy DiagnosticPolicy) bool {
	if report == nil || receipt == nil {
		return false
	}
	table, tableOK := composite.Diagnostics()
	if !tableOK {
		return false
	}
	var trueSeverity, falseSeverity FindingSeverity
	collectGuards := false
	for position := 0; position < table.Count(); position++ {
		entry, entryOK := table.At(position)
		if !entryOK {
			return false
		}
		if entry.Lane() != diagnostic.LaneBranch {
			continue
		}
		severity, enabled := policy.enabled(entry.Code())
		if !enabled {
			continue
		}
		switch entry.Code() {
		case DiagnosticCodeAlwaysTrueGuard:
			trueSeverity, collectGuards = severity, true
		case DiagnosticCodeAlwaysFalseGuard:
			falseSeverity, collectGuards = severity, true
		default:
			return false
		}
	}
	if collectGuards && (schema == nil || solver == nil || state == nil || !collectGuardPolarityFindings(report, receipt, selected, schema, solver, state, trueSeverity, falseSeverity)) {
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
func collectGuardPolarityFindings(report *DiagnosticReport, receipt *artifactResultReceipt, selected []artifactDiagnosticObservationReceipt, schema *valuedomain.Schema, solver *engine.Solver, state *engine.State, trueSeverity, falseSeverity FindingSeverity) bool {
	if report == nil || receipt == nil || schema == nil || solver == nil || state == nil || (trueSeverity != FindingSeverityInvalid && !trueSeverity.Available()) || (falseSeverity != FindingSeverityInvalid && !falseSeverity.Available()) || (trueSeverity == FindingSeverityInvalid && falseSeverity == FindingSeverityInvalid) {
		return false
	}
	observations := make(map[artifactResultPoint]valuedomain.ValueSummaryObservation, len(receipt.pointObservations))
	for _, selectedObservation := range selected {
		key := selectedObservation.point
		if len(receipt.pointObservations[key]) == 0 {
			report.collectionFailure = DiagnosticCollectionSubjectQueryAbsent
			return true
		}
		if _, duplicate := observations[key]; duplicate {
			report.collectionFailure = DiagnosticCollectionSubjectQueryAbsent
			return true
		}
		observation, readable := engine.ReceiptObservationResult(selectedObservation.observation, solver, state)
		if !readable {
			report.collectionFailure = DiagnosticCollectionQueryUnreadable
			return true
		}
		if !observation.Valid {
			report.collectionFailure = DiagnosticCollectionQueryInvalid
			return true
		}
		if len(observation.Values) != len(observation.Present) || len(observation.Values) != len(receipt.values) || observation.Rows > 1 {
			report.collectionFailure = DiagnosticCollectionValueShapeMismatch
			return true
		}
		observations[key] = observation
	}
	for _, subject := range receipt.branchObservations {
		if subject.kind != programartifact.DiagnosticObservationBranchCondition || !subject.available() || len(subject.points) == 0 {
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
				// undecided evidence, not a malformed observation receipt and must
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

// staticDiagnosticDeclaration resolves the declared row one artifact-issued
// observation population feeds. The sealed table is the sole authority: a
// population no row claims is a collector hole, not a row to skip.
//
// The artifact numbers the observation populations at load; the declaration
// numbers the same members, and the pin law holds the two numberings equal, so
// the compiled kind resolves the declared member at its ordinal and the row is
// found by that member's identity.
func staticDiagnosticDeclaration(kind programartifact.DiagnosticObservationKind) (*diagnostic.Entry, bool) {
	table, tableOK := composite.Diagnostics()
	vocabulary, vocabularyOK := composite.StructureVocabulary()
	if !tableOK || !vocabularyOK {
		return nil, false
	}
	population, populationOK := vocabulary.At(structure.CategoryDiagnosticObservation, uint16(kind))
	if !populationOK {
		return nil, false
	}
	return table.ForStaticObservation(population.Key())
}

// collectStaticDiagnosticFindings owns policy selection for static rows. It
// is the generic collection point for all current and future owner-issued
// static observations; a row without an enabled matching projector remains a
// no-op, rather than being reverse-engineered from Engine state.
func collectStaticDiagnosticFindings(report *DiagnosticReport, receipt *artifactResultReceipt, policy DiagnosticPolicy) bool {
	if report == nil || receipt == nil {
		return false
	}
	for _, observation := range receipt.staticObservations {
		if !observation.available() {
			return false
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
// implicit-global receipt. It needs no Engine observation: Program issued the
// read/cell evidence and Link proved the name had no configured global.
func appendStaticUnresolvedValueFinding(report *DiagnosticReport, observation compiledObservation, severity FindingSeverity) bool {
	if report == nil || observation.kind != programartifact.DiagnosticObservationValueReferenceUnresolved || !observation.available() || !severity.Available() {
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
	if report == nil || observation.kind != programartifact.DiagnosticObservationTypeReferenceUnresolved || !observation.available() || !severity.Available() {
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

func buildDetachedArtifactResult(
	receipt *artifactResultReceipt,
	queries []artifactResultQueryReceipt,
	solver *engine.Solver,
	state *engine.State,
	publications artifactResultProjectionReceipts,
) (*Result, bool) {
	if !receipt.valid() || solver == nil || state == nil {
		return nil, false
	}
	values := append([]identity.ContentID(nil), receipt.values...)
	bodies := make([]resultBody, len(receipt.bodies))
	for index, body := range receipt.bodies {
		bodies[index] = resultBody{id: body.id, roots: append([]resultRoot(nil), body.roots...), valuePresence: make([]uint64, resultValueWordCount(len(values)))}
	}
	for _, query := range queries {
		key := artifactResultPoint{mount: query.attachment.mount, point: query.attachment.point}
		indexes := receipt.pointBodies[key]
		switch query.attachment.role {
		case artifactQueryValueSummary:
			observation, readable := engine.ReceiptQueryResult[valuedomain.ValueSummaryObservation](query.query, solver, state)
			if !readable || !observation.Valid {
				return nil, false
			}
			count := len(observation.Values)
			if len(observation.Present) != count || count != len(receipt.values) || observation.Rows > 1 {
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
			observation, readable := engine.ReceiptQueryResult[effectfactor.EffectObservation](query.query, solver, state)
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
	native, nativeOK := publications.detachNative()
	if !nativeOK {
		return nil, false
	}
	content, ok := analysisResultIDWithPublication(receipt.source, values, bodies, native)
	if !ok {
		return nil, false
	}
	result := &Result{source: receipt.source, content: content, values: values, bodies: bodies, native: native}
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
