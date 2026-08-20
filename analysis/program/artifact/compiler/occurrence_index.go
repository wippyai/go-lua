package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func (compiler *compiler) pointIDs(site causal.Site) []identity.ContentID {
	if compiler == nil || !site.Available() || !compiler.input.OwnsSite(site) || compiler.pointIDsBySite == nil {
		return nil
	}
	points, known := compiler.pointIDsBySite[site.ContextID()]
	if !known {
		return nil
	}
	return points
}

// indexPointAttachmentsFailure indexes the immutable Site-to-LocalWTO
// relation once from canonical Flow schedule data. The lookup map is
// compile-only geometry; each relation is emitted directly into the generic
// occurrence catalog so Artifact retains no second attachment plane.
func (compiler *compiler) indexPointAttachmentsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceAttachment)
	}
	if compiler.pointIDsBySite == nil {
		compiler.pointIDsBySite = make(map[identity.ContentID][]identity.ContentID)
	}
	for site := range compiler.pointIDsBySite {
		delete(compiler.pointIDsBySite, site)
	}
	wto := compiler.input.Flow().LocalWTO()
	seenPoints := make(map[identity.ContentID]struct{})
	seenAttachments := make(map[struct {
		site  identity.ContentID
		point identity.ContentID
	}]struct{})
	for eventIndex := 0; eventIndex < wto.EventCount(); eventIndex++ {
		event, eventOK := wto.EventAt(eventIndex)
		if !eventOK || !event.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, -1, CompileReasonOccurrenceAttachment)
		}
		if event.Kind() != causal.WTOEventPoint {
			continue
		}
		point, pointOK := event.Point()
		if !pointOK || !point.Available() || !point.PathID().Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, -1, CompileReasonOccurrenceAttachment)
		}
		if _, duplicate := seenPoints[point.PathID()]; duplicate {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, -1, CompileReasonOccurrenceAttachment)
		}
		seenPoints[point.PathID()] = struct{}{}
		for siteIndex := 0; siteIndex < point.SiteCount(); siteIndex++ {
			site, siteOK := point.SiteAt(siteIndex)
			if !siteOK || !site.Available() || !compiler.input.OwnsSite(site) || !site.ContextID().Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, siteIndex, CompileReasonOccurrenceAttachment)
			}
			key := struct {
				site  identity.ContentID
				point identity.ContentID
			}{site: site.ContextID(), point: point.PathID()}
			if _, duplicate := seenAttachments[key]; duplicate {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, siteIndex, CompileReasonOccurrenceAttachment)
			}
			if uint64(len(compiler.occurrences)) > uint64(^uint32(0)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, siteIndex, CompileReasonOccurrenceAttachment)
			}
			seenAttachments[key] = struct{}{}
			id := digest("analysis/program-artifact/point-attachment", artifactFormat(), bytesField(key.site), bytesField(key.point))
			if !id.Available() || !compiler.appendOccurrence(programschema.OccurrencePointAttachment, id, identity.ContentID{}, []identity.ContentID{key.point}, []identity.ContentID{key.site}, 0) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, siteIndex, CompileReasonOccurrenceAttachment)
			}
			compiler.pointIDsBySite[key.site] = append(compiler.pointIDsBySite[key.site], key.point)
		}
	}
	// Freeze each site index at its exact length before any later compiler
	// stage can append a temporary concatenation to the returned slice.
	for site, points := range compiler.pointIDsBySite {
		frozen := make([]identity.ContentID, len(points))
		copy(frozen, points)
		compiler.pointIDsBySite[site] = frozen
	}
	return CompileFailure{}
}

func (compiler *compiler) appendOccurrence(kind programschema.OccurrenceKind, id, body identity.ContentID, points, inputs []identity.ContentID, code uint64) bool {
	return compiler.appendOccurrencePayload(kind, id, body, points, inputs, code, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
}

func (compiler *compiler) appendOccurrencePayload(kind programschema.OccurrenceKind, id, body identity.ContentID, points, inputs []identity.ContentID, code uint64, literalFamily keyspace.Family, literal keyspace.LiteralValue, literalOK bool) bool {
	if compiler == nil || !fitsUint32(len(compiler.occurrences)) || !fitsUint32(len(compiler.occurrencePoints)) || !fitsUint32(len(compiler.occurrenceInputs)) {
		return false
	}
	// A Rule occurrence is attached to a semantic phase, not once per route
	// through which that phase was reached.  Source spans can legitimately
	// expose the same phase as both entry and finish (notably at a branch
	// join), so canonicalize that relation before it becomes an immutable
	// artifact row. Inputs retain their parent-issued order; only the point
	// membership relation is a set.
	points = canonicalPoints(points)
	if !fitsUint32(len(points)) || !fitsUint32(len(inputs)) ||
		uint64(len(compiler.occurrencePoints))+uint64(len(points)) > uint64(^uint32(0)) ||
		uint64(len(compiler.occurrenceInputs))+uint64(len(inputs)) > uint64(^uint32(0)) {
		return false
	}
	for _, pointID := range points {
		if !pointID.Available() {
			return false
		}
	}
	for _, inputID := range inputs {
		if !inputID.Available() {
			return false
		}
	}
	pointOffset, inputOffset := uint32(len(compiler.occurrencePoints)), uint32(len(compiler.occurrenceInputs))
	row, rowOK := programschema.NewOccurrence(kind, id, body, code, pointOffset, uint32(len(points)), inputOffset, uint32(len(inputs)), literalFamily, literal, literalOK)
	if !rowOK || !occurrenceSemanticAvailable(row) {
		return false
	}
	for _, pointID := range points {
		point, pointOK := programschema.NewOccurrencePoint(pointID)
		if !pointOK {
			return false
		}
		compiler.occurrencePoints = append(compiler.occurrencePoints, point)
	}
	for _, inputID := range inputs {
		input, inputOK := programschema.NewOccurrenceInput(inputID)
		if !inputOK {
			return false
		}
		compiler.occurrenceInputs = append(compiler.occurrenceInputs, input)
	}
	compiler.occurrences = append(compiler.occurrences, row)
	return true
}

func (compiler *compiler) appendRuleOccurrence(key, writes schema.Key, occurrence uint32, point, input identity.ContentID, stage programschema.RuleStage, inputKind programschema.RuleInputKind, route identity.ContentID) bool {
	if compiler == nil || !fitsUint32(len(compiler.ruleOccurrences)) {
		return false
	}
	row, rowOK := programschema.NewRuleOccurrence(key, writes, occurrence, point, input, stage, inputKind, route)
	if !rowOK {
		return false
	}
	compiler.ruleOccurrences = append(compiler.ruleOccurrences, row)
	return true
}

func (compiler *compiler) replaceRuleOccurrenceInput(index int, input identity.ContentID) bool {
	if compiler == nil || index < 0 || index >= len(compiler.ruleOccurrences) || !input.Available() {
		return false
	}
	row := compiler.ruleOccurrences[index]
	occurrence, occurrenceOK := row.Occurrence()
	if !occurrenceOK {
		return false
	}
	replaced, replacedOK := programschema.NewRuleOccurrence(row.Key(), row.Writes(), occurrence, row.PointID(), input, row.Stage(), row.InputKind(), func() identity.ContentID {
		route, _ := row.PredecessorRouteID()
		return route
	}())
	if !replacedOK {
		return false
	}
	compiler.ruleOccurrences[index] = replaced
	return true
}

func (compiler *compiler) recordOccurrenceSpan(kind programschema.OccurrenceKind, id identity.ContentID, entry, finish []identity.ContentID) bool {
	if compiler == nil || compiler.occurrenceSpans == nil || !kind.Valid() || !id.Available() || len(finish) == 0 {
		return false
	}
	key := occurrenceLookup{kind: kind, id: id}
	if _, duplicate := compiler.occurrenceSpans[key]; duplicate {
		return false
	}
	entry = canonicalPoints(entry)
	finish = canonicalPoints(finish)
	for _, point := range append(append([]identity.ContentID(nil), entry...), finish...) {
		if !point.Available() {
			return false
		}
	}
	compiler.occurrenceSpans[key] = occurrenceSpanGeometry{entry: append([]identity.ContentID(nil), entry...), finish: append([]identity.ContentID(nil), finish...)}
	return true
}

func (compiler *compiler) recordOccurrencePredecessor(kind programschema.OccurrenceKind, id, route identity.ContentID, finish []identity.ContentID) bool {
	if !compiler.recordOccurrenceSpan(kind, id, nil, finish) || !route.Available() {
		return false
	}
	key := occurrenceLookup{kind: kind, id: id}
	geometry := compiler.occurrenceSpans[key]
	geometry.route = route
	compiler.occurrenceSpans[key] = geometry
	return true
}

func canonicalPoints(points []identity.ContentID) []identity.ContentID {
	if len(points) < 2 {
		return points
	}
	seen := make(map[identity.ContentID]struct{}, len(points))
	canonical := make([]identity.ContentID, 0, len(points))
	for _, point := range points {
		if _, duplicate := seen[point]; duplicate {
			continue
		}
		seen[point] = struct{}{}
		canonical = append(canonical, point)
	}
	return canonical
}
