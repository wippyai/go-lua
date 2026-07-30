package body

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// DbgLocal is the source-level local identity visible at a debug point.
// LocalID is an ordinal in this body's canonical debug-local projection, so it
// distinguishes shadowed locals without leaking a binder-global symbol number.
// Its external identity is always the enclosing body/artifact digest plus this
// ordinal; Slot remains codegen-owned as specified by the frozen WIR contract.
type DbgLocal struct {
	LocalID uint32
	Name    string
	Kind    wir.SymbolKind
}

// DebugAnchor is the point's source anchor within its enclosing body. The
// artifact/body digest scopes it; callers resolve it only against a
// digest-matched editor snapshot.
type DebugAnchor struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// DebugMapEntry is one phase-qualified sequence-point mapping in a solved
// body. Entries are emitted in canonical body-point and phase order.
type DebugMapEntry struct {
	ID         wir.DebugPointID
	SourceSpan SourceSpan
	Anchor     DebugAnchor
	Visible    []DbgLocal
	MaySuspend bool
}

// DebugMap projects this solved body's observable WIR points into a
// deterministic debug map. It deliberately does not expose raw cfg.Point:
// DebugPointID is body-local and must be paired with the body/artifact digest
// at an external boundary.
func (r *Result) DebugMap() []DebugMapEntry {
	if r == nil || r.wir == nil || r.Graph() == nil {
		return nil
	}
	var out []DebugMapEntry
	localIDs := r.debugLocalOrdinals()
	for _, point := range r.wir.DebugPoints() {
		baseSpan, ok := r.debugPointSpan(point.Point, wir.DebugPhaseBefore)
		if !ok {
			continue
		}
		maySuspend := r.PointMaySuspend(point.Point)
		for _, phase := range r.debugPhasesAt(point.Point, maySuspend) {
			id, ok := r.wir.DebugPointID(point.Point, phase)
			if !ok {
				continue
			}
			span := baseSpan
			if phaseSpan, hasPhaseSpan := r.debugPointSpan(point.Point, phase); hasPhaseSpan {
				span = phaseSpan
			}
			out = append(out, DebugMapEntry{
				ID:         id,
				SourceSpan: sourceSpanFromWIR(span),
				Anchor:     debugAnchorFromWIR(span),
				Visible:    r.debugVisibleLocals(point.Point, phase, localIDs),
				MaySuspend: maySuspend,
			})
		}
	}
	return out
}

func (r *Result) debugPhasesAt(point cfg.Point, maySuspend bool) []wir.DebugPhase {
	phases := []wir.DebugPhase{wir.DebugPhaseBefore, wir.DebugPhaseAfter}
	for _, inst := range r.wir.PointInstructions(point) {
		switch inst.Op {
		case wir.OpCall:
			phases = append(phases, wir.DebugPhaseCall)
		case wir.OpReturn:
			phases = append(phases, wir.DebugPhaseReturn)
		}
	}
	if maySuspend {
		phases = append(phases, wir.DebugPhaseSuspend)
	}
	return phases
}

func (r *Result) debugLocalOrdinals() map[wir.SymbolID]uint32 {
	if r == nil || r.wir == nil {
		return nil
	}
	out := make(map[wir.SymbolID]uint32)
	for _, point := range r.wir.DebugPoints() {
		for _, phase := range []wir.DebugPhase{wir.DebugPhaseBefore, wir.DebugPhaseAfter} {
			for _, id := range r.wir.DebugLocalVisibility(point.Point, phase) {
				if _, exists := out[id]; exists || !r.isDbgLocal(id) {
					continue
				}
				out[id] = uint32(len(out) + 1)
			}
		}
	}
	return out
}

func (r *Result) debugVisibleLocals(point cfg.Point, phase wir.DebugPhase, localIDs map[wir.SymbolID]uint32) []DbgLocal {
	if r == nil || r.wir == nil {
		return nil
	}
	ids := r.wir.DebugLocalVisibility(point, phase)
	if len(ids) == 0 {
		return nil
	}
	out := make([]DbgLocal, 0, len(ids))
	for _, id := range ids {
		if !r.isDbgLocal(id) {
			continue
		}
		info, _ := r.wir.SymbolInfo(id)
		localID := localIDs[id]
		if localID == 0 {
			continue
		}
		out = append(out, DbgLocal{
			LocalID: localID,
			Name:    r.wir.SymbolName(id),
			Kind:    info.Kind,
		})
	}
	return out
}

func (r *Result) isDbgLocal(id wir.SymbolID) bool {
	if r == nil || r.wir == nil || id == 0 {
		return false
	}
	info, ok := r.wir.SymbolInfo(id)
	return ok && (info.Kind == wir.SymbolParam || info.Kind == wir.SymbolLocal)
}

func (r *Result) debugPointSpan(point cfg.Point, phase wir.DebugPhase) (wir.Span, bool) {
	if r == nil || r.wir == nil {
		return wir.Span{}, false
	}
	for _, inst := range r.wir.PointInstructions(point) {
		if phase == wir.DebugPhaseCall && inst.Op != wir.OpCall {
			continue
		}
		if phase == wir.DebugPhaseReturn && inst.Op != wir.OpReturn {
			continue
		}
		if span, ok := debugInstructionSpan(r.wir, inst, phase); ok {
			return span, true
		}
	}
	return wir.Span{}, false
}

func debugInstructionSpan(body *wir.Body, inst wir.Instruction, phase wir.DebugPhase) (wir.Span, bool) {
	if phase == wir.DebugPhaseCall && inst.CallSpan.Valid() {
		return inst.CallSpan, true
	}
	if phase == wir.DebugPhaseReturn {
		for _, meta := range body.ReturnValueMeta(inst.ReturnValues) {
			if meta.Span.Valid() {
				return meta.Span, true
			}
		}
	}
	for _, span := range []wir.Span{inst.ExprSpan, inst.TargetSpan, inst.CallSpan, inst.CalleeSpan, inst.ContainerSpan} {
		if span.Valid() {
			return span, true
		}
	}
	return wir.Span{}, false
}

func sourceSpanFromWIR(span wir.Span) SourceSpan {
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}

func debugAnchorFromWIR(span wir.Span) DebugAnchor {
	return DebugAnchor{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}
