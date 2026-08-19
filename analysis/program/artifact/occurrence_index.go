package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
)

func (compiler *compiler) pointIDs(site flow.Site) []identity.ContentID {
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
	wto := compiler.input.Flow().Local().WTO()
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
		if event.Kind() != flow.WTOEventPoint {
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
			id := digest("analysis/program-artifact/point-attachment", artifactFormat, bytesField(key.site), bytesField(key.point))
			if !id.Available() || !compiler.appendOccurrence(OccurrencePointAttachment, id, identity.ContentID{}, []identity.ContentID{key.point}, []identity.ContentID{key.site}, 0) {
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

func (compiler *compiler) appendOccurrence(kind OccurrenceKind, id, body identity.ContentID, points, inputs []identity.ContentID, code uint64) bool {
	// A Rule occurrence is attached to a semantic phase, not once per route
	// through which that phase was reached.  Source spans can legitimately
	// expose the same phase as both entry and finish (notably at a branch
	// join), so canonicalize that relation before it becomes an immutable
	// artifact row. Inputs retain their parent-issued order; only the point
	// membership relation is a set.
	points = canonicalPoints(points)
	row := OccurrenceRow{kind: kind, id: id, body: body, points: points, inputs: inputs, code: code}
	if !row.Available() {
		return false
	}
	compiler.occurrences = append(compiler.occurrences, row)
	return true
}

func (compiler *compiler) recordOccurrenceSpan(kind OccurrenceKind, id identity.ContentID, entry, finish []identity.ContentID) bool {
	if compiler == nil || compiler.occurrenceSpans == nil || !kind.valid() || !id.Available() || len(finish) == 0 {
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

func (compiler *compiler) recordOccurrencePredecessor(kind OccurrenceKind, id, route identity.ContentID, finish []identity.ContentID) bool {
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
