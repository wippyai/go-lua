package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	artifactdigest "github.com/wippyai/go-lua/analysis/program/artifact/digest"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// indexPointAttachmentsFailure publishes the immutable Site-to-LocalWTO
// relation already owned by Flow. The relation is emitted directly into the
// generic occurrence catalog; no compiler-side inverse or copied point-ID
// map is built.
func (compiler *compiler) indexPointAttachmentsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceAttachment)
	}
	wto := compiler.input.Flow().LocalWTO()
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
		for siteIndex := 0; siteIndex < point.SiteCount(); siteIndex++ {
			site, siteOK := point.SiteAt(siteIndex)
			if !siteOK || !site.Available() || !compiler.input.OwnsSite(site) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, siteIndex, CompileReasonOccurrenceAttachment)
			}
			if uint64(len(compiler.publication.Occurrences)) > uint64(^uint32(0)) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, siteIndex, CompileReasonOccurrenceAttachment)
			}
			id := artifactdigest.Digest("analysis/program-artifact/point-attachment", artifactFormat(), artifactdigest.ContentID(site.ContextID()), artifactdigest.ContentID(point.PathID()))
			if !id.Available() || !compiler.appendOccurrence(programschema.OccurrencePointAttachment, id, identity.ContentID{}, []identity.ContentID{point.PathID()}, []identity.ContentID{site.ContextID()}, 0) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, eventIndex, siteIndex, CompileReasonOccurrenceAttachment)
			}
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) appendOccurrence(kind programschema.OccurrenceKind, id, body identity.ContentID, points, inputs []identity.ContentID, code uint64) bool {
	return compiler.appendOccurrencePayload(kind, id, body, points, inputs, code, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
}

// pointPathScratch projects one or two sealed owner ranges into compiler
// scratch. The scratch is reused across rows; no Site→Point slice or map is
// retained, and the owner remains the only authority for the relation.
func (compiler *compiler) pointPathScratch(first, second causal.SitePointPaths) ([]identity.ContentID, bool) {
	if compiler == nil {
		return nil, false
	}
	if compiler.pointScratchSeen == nil {
		compiler.pointScratchSeen = make(map[identity.ContentID]struct{})
	}
	clear(compiler.pointScratchSeen)
	compiler.pointScratch = compiler.pointScratch[:0]
	for _, paths := range [...]causal.SitePointPaths{first, second} {
		if !paths.Available() {
			continue
		}
		for index := 0; index < paths.Count(); index++ {
			point, ok := paths.At(index)
			if !ok {
				return nil, false
			}
			if _, duplicate := compiler.pointScratchSeen[point]; duplicate {
				continue
			}
			compiler.pointScratchSeen[point] = struct{}{}
			compiler.pointScratch = append(compiler.pointScratch, point)
		}
	}
	return compiler.pointScratch, true
}

// appendOccurrencePaths consumes sealed owner ranges directly into the
// canonical occurrence publication. The only reusable temporary is the
// compiler's bounded scratch plane; callers never receive owner storage.
func (compiler *compiler) appendOccurrencePaths(kind programschema.OccurrenceKind, id, body identity.ContentID, first, second causal.SitePointPaths, inputs []identity.ContentID, code uint64) bool {
	return compiler.appendOccurrencePathsPayload(kind, id, body, first, second, inputs, code, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
}

func (compiler *compiler) appendOccurrencePathsPayload(kind programschema.OccurrenceKind, id, body identity.ContentID, first, second causal.SitePointPaths, inputs []identity.ContentID, code uint64, literalFamily keyspace.Family, literal keyspace.LiteralValue, literalOK bool) bool {
	points, ok := compiler.pointPathScratch(first, second)
	entry, entryOK := copyOptionalPointPaths(first)
	finish, finishOK := copyOptionalPointPaths(second)
	return ok && entryOK && finishOK && compiler.appendOccurrencePayloadCanonical(kind, id, body, points, inputs, code, literalFamily, literal, literalOK, entry, finish)
}

func (compiler *compiler) appendOccurrencePayload(kind programschema.OccurrenceKind, id, body identity.ContentID, points, inputs []identity.ContentID, code uint64, literalFamily keyspace.Family, literal keyspace.LiteralValue, literalOK bool) bool {
	points = canonicalPoints(points)
	return compiler.appendOccurrencePayloadCanonical(kind, id, body, points, inputs, code, literalFamily, literal, literalOK, nil, points)
}

func (compiler *compiler) appendOccurrencePayloadCanonical(kind programschema.OccurrenceKind, id, body identity.ContentID, points, inputs []identity.ContentID, code uint64, literalFamily keyspace.Family, literal keyspace.LiteralValue, literalOK bool, entry, finish []identity.ContentID) bool {
	if compiler == nil || compiler.issuanceRows == nil || !fitsUint32(len(compiler.publication.Occurrences)) || !fitsUint32(len(compiler.publication.OccurrencePoints)) || !fitsUint32(len(compiler.publication.OccurrenceInputs)) {
		return false
	}
	if !fitsUint32(len(points)) || !fitsUint32(len(inputs)) ||
		uint64(len(compiler.publication.OccurrencePoints))+uint64(len(points)) > uint64(^uint32(0)) ||
		uint64(len(compiler.publication.OccurrenceInputs))+uint64(len(inputs)) > uint64(^uint32(0)) {
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
	if !compiler.issuanceRows.AddGeometry(kind, id, entry, finish) {
		return false
	}
	pointOffset, inputOffset := uint32(len(compiler.publication.OccurrencePoints)), uint32(len(compiler.publication.OccurrenceInputs))
	row, rowOK := programschema.NewOccurrence(kind, id, body, code, pointOffset, uint32(len(points)), inputOffset, uint32(len(inputs)), literalFamily, literal, literalOK)
	if !rowOK || !programschema.OccurrenceSemanticAvailable(row) {
		return false
	}
	for _, pointID := range points {
		point, pointOK := programschema.NewOccurrencePoint(pointID)
		if !pointOK {
			return false
		}
		compiler.publication.OccurrencePoints = append(compiler.publication.OccurrencePoints, point)
	}
	for _, inputID := range inputs {
		input, inputOK := programschema.NewOccurrenceInput(inputID)
		if !inputOK {
			return false
		}
		compiler.publication.OccurrenceInputs = append(compiler.publication.OccurrenceInputs, input)
	}
	compiler.publication.Occurrences = append(compiler.publication.Occurrences, row)
	return true
}

func copyOptionalPointPaths(paths causal.SitePointPaths) ([]identity.ContentID, bool) {
	if !paths.Available() {
		return nil, true
	}
	return copyPointPaths(paths)
}

func copyPointPaths(paths causal.SitePointPaths) ([]identity.ContentID, bool) {
	if !paths.Available() {
		return nil, false
	}
	points := make([]identity.ContentID, paths.Count())
	for index := range points {
		point, ok := paths.At(index)
		if !ok {
			return nil, false
		}
		points[index] = point
	}
	return points, true
}

func (compiler *compiler) recordOccurrencePredecessorPaths(kind programschema.OccurrenceKind, id, route identity.ContentID, finish causal.SitePointPaths) bool {
	finishPoints, finishOK := copyPointPaths(finish)
	return finishOK && compiler.recordOccurrencePredecessor(kind, id, route, finishPoints)
}

// appendRuleOccurrenceVector publishes the ordered input-point roles selected
// by the sealed issuance schedule. The compiler copies the vector into the
// immutable Program row; no later phase may infer or replace one of its roles.
func (compiler *compiler) appendRuleOccurrenceVector(key, writes schema.Key, occurrence uint32, point identity.ContentID, inputs []identity.ContentID, stage, inputSpec schema.Key, route programschema.RuleOccurrenceRoute, native bool, source programschema.RuleOccurrenceSource) bool {
	if compiler == nil || !fitsUint32(len(compiler.publication.RuleOccurrences)) {
		return false
	}
	row, rowOK := programschema.NewRuleOccurrenceWithInputs(key, writes, occurrence, point, inputs, stage, inputSpec, route, native, source)
	if !rowOK {
		return false
	}
	compiler.publication.RuleOccurrences = append(compiler.publication.RuleOccurrences, row)
	return true
}

func (compiler *compiler) recordOccurrencePredecessor(kind programschema.OccurrenceKind, id, route identity.ContentID, finish []identity.ContentID) bool {
	if compiler == nil || compiler.issuanceRows == nil || !kind.Valid() || !id.Available() || !route.Available() || len(finish) == 0 {
		return false
	}
	routeIndex, found := compiler.environmentByRoute[route]
	edgeOrdinal, unique := routeIndex.uniqueAt(len(compiler.environment))
	if !found || !unique {
		return false
	}
	edge := compiler.environment[edgeOrdinal]
	expectedID := environmentRouteOccurrenceID(compiler.input.ContentID(), route, edge.arm)
	if !edge.Available() || edge.route != route || edge.id != expectedID || !edge.to.Available() {
		return false
	}
	for _, point := range finish {
		if point == edge.to {
			return compiler.issuanceRows.AddPredecessor(id, route, edge.to)
		}
	}
	return false
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
