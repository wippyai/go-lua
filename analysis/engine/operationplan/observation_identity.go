package operationplan

import (
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

type observationPoint struct{ after, call wir.DebugPointID }

// WithObservationIdentity binds lowering-owned debug points to stable lexical
// ownership. The plan retains no source spans, dense expression IDs, or wire
// artifact digest; service projection owns the canonical DebugMap fence.
func (p *Plan) WithObservationIdentity(body lexicalidentity.StableLexicalBodyID, lowered *wir.Body) *Plan {
	if p == nil {
		return nil
	}
	out := *p
	out.observationBody = lexicalidentity.StableLexicalBodyID{}
	out.observationPoints = nil
	if body == (lexicalidentity.StableLexicalBodyID{}) || lowered == nil {
		return &out
	}
	points := make([]observationPoint, p.PointCount())
	for raw := range points {
		after, afterOK := lowered.DebugPointID(cfg.Point(raw), wir.DebugPhaseAfter)
		if !afterOK {
			return &out
		}
		points[raw].after = after
		if lowered.HasInstruction(cfg.Point(raw), wir.OpCall) {
			points[raw].call, _ = lowered.DebugPointID(cfg.Point(raw), wir.DebugPhaseCall)
		}
	}
	out.observationBody, out.observationPoints = body, points
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
