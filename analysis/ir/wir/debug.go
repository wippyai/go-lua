package wir

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// DebugPhase identifies an observable execution phase at one body-local
// sequence point. It is part of DebugPointID, not a separately allocated
// global counter.
type DebugPhase uint8

const (
	DebugPhaseBefore DebugPhase = iota + 1
	DebugPhaseAfter
	DebugPhaseCall
	DebugPhaseReturn
	DebugPhaseSuspend
)

// String returns the schema vocabulary used by serialized debug maps.
func (p DebugPhase) String() string {
	switch p {
	case DebugPhaseBefore:
		return "before"
	case DebugPhaseAfter:
		return "after"
	case DebugPhaseCall:
		return "call"
	case DebugPhaseReturn:
		return "return"
	case DebugPhaseSuspend:
		return "suspend"
	default:
		return "unknown"
	}
}

// Valid reports whether p is in the frozen debug-map phase vocabulary.
func (p DebugPhase) Valid() bool {
	return p >= DebugPhaseBefore && p <= DebugPhaseSuspend
}

// DebugPointID identifies one observable phase at a sequence point in one
// Body. Ordinal is canonical only within that body; consumers must compose an
// ID with the body digest (normally via StaticArtifactID) before using it as an
// external identity.
type DebugPointID struct {
	Ordinal uint32
	Phase   DebugPhase
}

// Valid reports whether id names a body-local observable phase.
func (id DebugPointID) Valid() bool {
	return id.Ordinal != 0 && id.Phase.Valid()
}

// String is the canonical, human-readable wire form of a DebugPointID.
func (id DebugPointID) String() string {
	if !id.Valid() {
		return ""
	}
	return fmt.Sprintf("p%d:%s", id.Ordinal, id.Phase)
}

// DebugPoint is the lowering-time correspondence between a CFG point and its
// canonical, body-local sequence-point ordinal.
type DebugPoint struct {
	Point   cfg.Point
	Ordinal uint32
}

// AssignDebugPointOrdinals records the canonical RPO traversal as this body's
// debug-point sequence. Lowering calls this once after it has emitted all
// instructions. RPO is deterministic for fixed source, CFG builder, and
// toolchain, but the resulting ordinals deliberately make no cross-body or
// cross-digest identity promise.
func (b *Body) AssignDebugPointOrdinals(graph cfg.Graph) {
	if b == nil {
		return
	}
	b.debugPointOrdinals = nil
	b.debugPointOrder = nil
	if graph == nil {
		return
	}
	points := graph.RPO()
	if len(points) == 0 {
		return
	}
	b.debugPointOrdinals = make(map[cfg.Point]uint32, len(points))
	b.debugPointOrder = append([]cfg.Point(nil), points...)
	for index, point := range points {
		b.debugPointOrdinals[point] = uint32(index + 1)
	}
}

// DebugPoints returns the body's canonical debug-point traversal. The returned
// slice is detached from Body storage and is ordered by ordinal.
func (b *Body) DebugPoints() []DebugPoint {
	if b == nil || len(b.debugPointOrder) == 0 {
		return nil
	}
	out := make([]DebugPoint, len(b.debugPointOrder))
	for index, point := range b.debugPointOrder {
		out[index] = DebugPoint{Point: point, Ordinal: uint32(index + 1)}
	}
	return out
}

// DebugPointID returns the phase-qualified body-local ID for point.
func (b *Body) DebugPointID(point cfg.Point, phase DebugPhase) (DebugPointID, bool) {
	if b == nil || !phase.Valid() {
		return DebugPointID{}, false
	}
	ordinal := b.debugPointOrdinals[point]
	if ordinal == 0 {
		return DebugPointID{}, false
	}
	return DebugPointID{Ordinal: ordinal, Phase: phase}, true
}
