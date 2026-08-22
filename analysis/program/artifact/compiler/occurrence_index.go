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
	return ok && compiler.appendOccurrencePayloadCanonical(kind, id, body, points, inputs, code, literalFamily, literal, literalOK)
}

func (compiler *compiler) appendOccurrencePayload(kind programschema.OccurrenceKind, id, body identity.ContentID, points, inputs []identity.ContentID, code uint64, literalFamily keyspace.Family, literal keyspace.LiteralValue, literalOK bool) bool {
	points = canonicalPoints(points)
	return compiler.appendOccurrencePayloadCanonical(kind, id, body, points, inputs, code, literalFamily, literal, literalOK)
}

func (compiler *compiler) appendOccurrencePayloadCanonical(kind programschema.OccurrenceKind, id, body identity.ContentID, points, inputs []identity.ContentID, code uint64, literalFamily keyspace.Family, literal keyspace.LiteralValue, literalOK bool) bool {
	if compiler == nil || !fitsUint32(len(compiler.publication.Occurrences)) || !fitsUint32(len(compiler.publication.OccurrencePoints)) || !fitsUint32(len(compiler.publication.OccurrenceInputs)) {
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

func (compiler *compiler) recordOccurrencePaths(kind programschema.OccurrenceKind, id identity.ContentID, entry, finish causal.SitePointPaths) bool {
	if compiler == nil || compiler.occurrenceSpans == nil || !kind.Valid() || !id.Available() || finish.Count() == 0 {
		return false
	}
	key := occurrenceLookup{kind: kind, id: id}
	if _, duplicate := compiler.occurrenceSpans[key]; duplicate {
		return false
	}
	entryPoints, entryOK := []identity.ContentID(nil), true
	if entry.Available() {
		entryPoints, entryOK = copyPointPaths(entry)
	}
	finishPoints, finishOK := copyPointPaths(finish)
	if !entryOK || !finishOK {
		return false
	}
	// Causal seals each Site→Point range as an injective canonical sequence.
	// These exact slices become the occurrence geometry; copying them again
	// would create a second temporary representation without adding a law.
	compiler.occurrenceSpans[key] = occurrenceSpanGeometry{entry: entryPoints, finish: finishPoints}
	return true
}

func (compiler *compiler) recordOccurrencePredecessorPaths(kind programschema.OccurrenceKind, id, route identity.ContentID, finish causal.SitePointPaths) bool {
	finishPoints, finishOK := copyPointPaths(finish)
	return finishOK && compiler.recordOccurrencePredecessor(kind, id, route, finishPoints)
}

func (compiler *compiler) appendRuleOccurrence(key, writes schema.Key, occurrence uint32, point, input identity.ContentID, stage programschema.RuleStage, inputKind programschema.RuleInputKind, route identity.ContentID) bool {
	if compiler == nil || !fitsUint32(len(compiler.publication.RuleOccurrences)) {
		return false
	}
	row, rowOK := programschema.NewRuleOccurrence(key, writes, occurrence, point, input, stage, inputKind, route)
	if !rowOK {
		return false
	}
	compiler.publication.RuleOccurrences = append(compiler.publication.RuleOccurrences, row)
	return true
}

func (compiler *compiler) replaceRuleOccurrenceInput(index int, input identity.ContentID) bool {
	if compiler == nil || index < 0 || index >= len(compiler.publication.RuleOccurrences) || !input.Available() {
		return false
	}
	row := compiler.publication.RuleOccurrences[index]
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
	compiler.publication.RuleOccurrences[index] = replaced
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
