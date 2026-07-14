package operationplan

import (
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

type observationPoint struct{ after, call wir.DebugPointID }

// WithObservationIdentity binds lowering-owned debug points to stable lexical
// ownership. Graph is the independent reachable-topology authority: its exact
// RPO must match lowering's canonical debug traversal. Dense unreachable plan
// slots intentionally retain zero anchors. The plan retains no source spans,
// dense expression IDs, or wire artifact digest; service projection owns the
// canonical DebugMap fence.
func (p *Plan) WithObservationIdentity(body lexicalidentity.StableLexicalBodyID, lowered *wir.Body, graph cfg.Graph) *Plan {
	if p == nil {
		return nil
	}
	out := *p
	out.observationBody = lexicalidentity.StableLexicalBodyID{}
	out.observationPoints = nil
	out.observationRequirements = ObservationRequirements{}
	out.callSurface = CallSurface{}
	if body == (lexicalidentity.StableLexicalBodyID{}) || lowered == nil || graph == nil || graph.Size() != p.PointCount() {
		return &out
	}
	reachable := cfg.RPOReadOnly(graph)
	debugPoints := lowered.DebugPoints()
	if len(reachable) == 0 || len(debugPoints) != len(reachable) {
		return &out
	}
	points := make([]observationPoint, p.PointCount())
	requirements := newObservationRequirementBuilder(p.PointCount())
	for index, point := range reachable {
		if uint64(point) >= uint64(len(points)) || points[point].after.Valid() {
			return &out
		}
		debugPoint := debugPoints[index]
		if debugPoint.Point != point || debugPoint.Ordinal != uint32(index+1) {
			return &out
		}
		after, afterOK := lowered.DebugPointID(point, wir.DebugPhaseAfter)
		if !afterOK || after != (wir.DebugPointID{Ordinal: debugPoint.Ordinal, Phase: wir.DebugPhaseAfter}) {
			return &out
		}
		points[point].after = after
		if lowered.HasInstruction(point, wir.OpCall) {
			call, callOK := lowered.DebugPointID(point, wir.DebugPhaseCall)
			if !callOK || call != (wir.DebugPointID{Ordinal: debugPoint.Ordinal, Phase: wir.DebugPhaseCall}) {
				return &out
			}
			points[point].call = call
		}
		// Requirement compilation shares this existing canonical RPO/debug
		// traversal. It must never introduce a second CFG scan.
		requirements.addPoint(&out, graph, body, point, points[point])
	}
	out.observationBody, out.observationPoints = body, points
	out.observationRequirements = requirements.freezeCanonical(&out, points)
	if !out.observationRequirements.sealed {
		out.observationBody = lexicalidentity.StableLexicalBodyID{}
		out.observationPoints = nil
	}
	return &out
}

func (p *Plan) ObservationBody() lexicalidentity.StableLexicalBodyID {
	if p == nil {
		return lexicalidentity.StableLexicalBodyID{}
	}
	return p.observationBody
}

func (p *Plan) observationOccurrence(point cfg.Point, kind observation.Kind, slot uint32) (observation.Occurrence, bool) {
	if p == nil || p.observationBody == (lexicalidentity.StableLexicalBodyID{}) || int(point) < 0 || int(point) >= len(p.observationPoints) {
		return observation.Occurrence{}, false
	}
	ids := p.observationPoints[point]
	id := ids.after
	if kind == observation.CallInvocation || kind == observation.CallArgument || kind == observation.CallResult {
		id = ids.call
	}
	out := observation.Occurrence{Point: id, Kind: kind, Slot: slot}
	return out, out.Valid()
}

func (p *Plan) AssignmentObservationAnchor(point cfg.Point) (observation.Occurrence, bool) {
	return p.observationOccurrence(point, observation.Assignment, 0)
}
func (p *Plan) CallResultObservationAnchor(point cfg.Point, slot uint32) (observation.Occurrence, bool) {
	return p.observationOccurrence(point, observation.CallResult, slot)
}
func (p *Plan) CallArgumentObservationAnchor(point cfg.Point, slot uint32) (observation.Occurrence, bool) {
	return p.observationOccurrence(point, observation.CallArgument, slot)
}
func (p *Plan) CallInvocationObservationAnchor(point cfg.Point) (observation.Occurrence, bool) {
	return p.observationOccurrence(point, observation.CallInvocation, 0)
}
