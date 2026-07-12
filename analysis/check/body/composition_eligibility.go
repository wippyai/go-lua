package body

import (
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// CompositionEligibility is the static, behavior-neutral Stage-0 verdict for
// the value-only symbolic-call POC. An empty Reason is eligible. The classifier
// is deliberately fail-closed: unknown syntax or metadata is contextual.
type CompositionEligibility struct {
	Reason  string
	reasons []string
}

func (e CompositionEligibility) Eligible() bool { return e.Reason == "" }

// RejectionReasons returns every observed blocker in canonical priority order.
func (e CompositionEligibility) RejectionReasons() []string {
	if len(e.reasons) != 0 {
		return append([]string(nil), e.reasons...)
	}
	if e.Reason != "" {
		return []string{e.Reason}
	}
	return nil
}

// CompositionStateCapability describes whether the POC can represent one
// registered State lane. The census is derived from the State catalog, so a
// newly registered lane automatically starts contextual.
type CompositionStateCapability struct {
	Lane  state.LaneID
	Exact bool
}

func CompositionStateCapabilities() []CompositionStateCapability {
	lanes := state.DefaultLanes()
	out := make([]CompositionStateCapability, len(lanes))
	for i, lane := range lanes {
		out[i] = CompositionStateCapability{Lane: lane, Exact: lane == state.LaneValues}
	}
	return out
}

// CompositionEligibility reports whether s has the local shape proven by
// poc/symboliccall. It does not enable composition or alter solving.
func (s *Static) CompositionEligibility() CompositionEligibility {
	if s == nil {
		return CompositionEligibility{Reason: "shape:missing-static"}
	}
	s.compositionEligibilityOnce.Do(func() {
		s.compositionEligibility = s.classifyCompositionEligibility()
	})
	return s.compositionEligibility
}

func (s *Static) classifyCompositionEligibility() CompositionEligibility {
	if s == nil || s.wir == nil || s.cfg == nil || s.cfg.Graph == nil {
		return CompositionEligibility{Reason: "shape:missing-static"}
	}
	if s.function == nil {
		return CompositionEligibility{Reason: "shape:chunk"}
	}
	reasons := make(map[string]struct{})
	add := func(reason string) {
		if reason != "" {
			reasons[reason] = struct{}{}
		}
	}
	if len(s.function.TypeParams) != 0 {
		add("shape:generic-function")
	}
	if s.function.ParList != nil && s.function.ParList.HasVargs {
		add("shape:vararg")
	}
	directCallees := make(map[symbol.ID]struct{})
	for i := 0; i < s.wir.Len(); i++ {
		inst := s.wir.Instr(i)
		if inst.Op != wir.OpCall || inst.Call.Receiver.Kind != wir.OperandNone || inst.Call.Method != 0 || inst.Call.Callee.Kind != wir.OperandPath {
			continue
		}
		p := s.wir.Path(wir.PathRef(inst.Call.Callee.Ref))
		if p.Symbol != 0 && len(p.Segments) == 0 {
			directCallees[p.Symbol] = struct{}{}
		}
	}
	for _, capture := range s.bindings.DirectCaptures(s.function) {
		if _, calleeOnly := directCallees[capture.Captured]; !calleeOnly {
			add("boundary:capture")
		}
	}
	for _, global := range s.bindings.DirectGlobalReads(s.function) {
		if _, calleeOnly := directCallees[global]; !calleeOnly {
			add("boundary:global")
		}
	}

	callTemps := make(map[uint32]struct{})
	for i := 0; i < s.wir.Len(); i++ {
		inst := s.wir.Instr(i)
		switch inst.Op {
		case wir.OpNoop, wir.OpEntry, wir.OpExit:
		case wir.OpReturn:
			for _, operand := range s.wir.Operands(inst.List) {
				classifyBoundaryOperand(s.wir, operand, callTemps, directCallees, add)
			}
		case wir.OpCall:
			classifyDirectCall(s.wir, inst, callTemps, add)
			for _, result := range s.wir.Operands(inst.Results) {
				if result.Kind == wir.OperandTemp {
					callTemps[result.Ref] = struct{}{}
				}
			}
		case wir.OpBranch, wir.OpIterate:
			// Exact branch lowering and the prepared WTO evaluator are the
			// semantic authorities. Unsupported guards/loops fail closed while
			// building or freezing the relation; syntax alone is not a blocker.
		case wir.OpMakeTable, wir.OpClosure:
			add("boundary:allocation")
		case wir.OpAssign:
			if !exactSymbolicLocalDeclaration(s.wir, inst, callTemps) {
				add("boundary:mutation")
			}
		case wir.OpStaticMemberWrite, wir.OpDynamicIndexWrite:
			add("boundary:mutation")
		case wir.OpDynamicIndexRead:
			add("boundary:heap-read")
		case wir.OpSelect:
			add("actor:channel-select")
		default:
			add("shape:unsupported-op")
		}
	}
	if len(reasons) == 0 {
		return CompositionEligibility{}
	}
	ordered := compositionReasons(reasons)
	return CompositionEligibility{Reason: ordered[0], reasons: ordered}
}

// exactSymbolicLocalDeclaration matches the RootAssignment compiler's immutable
// declaration slice. Constants, unknown iterator seeds, boundary-root aliases,
// and exact direct-call result temps are names introduced once, not mutations.
// Ordinary root writes remain contextual, including loop-carried accumulators.
func exactSymbolicLocalDeclaration(body *wir.Body, inst wir.Instruction, callTemps map[uint32]struct{}) bool {
	if inst.Assign != wir.AssignLocalDeclaration || inst.Dst.Kind != wir.OperandPath {
		return false
	}
	path := body.Path(wir.PathRef(inst.Dst.Ref))
	if path.Symbol == 0 || len(path.Segments) != 0 {
		return false
	}
	kind, ok := body.SymbolKind(wir.SymbolID(path.Symbol))
	if !ok || kind != wir.SymbolLocal || body.SymbolHasWrite(wir.SymbolID(path.Symbol)) {
		return false
	}
	switch inst.A.Kind {
	case wir.OperandNone, wir.OperandConst:
		return true
	case wir.OperandTemp:
		_, exactCallResult := callTemps[inst.A.Ref]
		return exactCallResult
	case wir.OperandPath:
		source := body.Path(wir.PathRef(inst.A.Ref))
		if source.Symbol == 0 || len(source.Segments) != 0 {
			return false
		}
		sourceKind, known := body.SymbolKind(wir.SymbolID(source.Symbol))
		return known && (sourceKind == wir.SymbolParam || sourceKind == wir.SymbolLocal)
	default:
		return false
	}
}

var compositionReasonPriority = []string{
	"shape:missing-static",
	"shape:chunk",
	"shape:generic-function",
	"shape:vararg",
	"shape:loop",
	"shape:guard",
	"boundary:capture",
	"boundary:global",
	"boundary:allocation",
	"boundary:mutation",
	"boundary:heap-read",
	"actor:channel-select",
	"call:protected",
	"call:generic",
	"call:method",
	"call:dynamic",
	"boundary:unknown-symbol",
	"shape:unsupported-op",
	"shape:unsupported-value",
}

func compositionReasons(reasons map[string]struct{}) []string {
	out := make([]string, 0, len(reasons))
	for _, reason := range compositionReasonPriority {
		if _, ok := reasons[reason]; ok {
			out = append(out, reason)
			delete(reasons, reason)
		}
	}
	if len(reasons) != 0 {
		out = append(out, "shape:unknown-feature")
	}
	return out
}

func classifyDirectCall(body *wir.Body, inst wir.Instruction, callTemps map[uint32]struct{}, add func(string)) {
	if inst.Call.Receiver.Kind != wir.OperandNone || inst.Call.Method != 0 {
		add("call:method")
		return
	}
	if len(body.TypeRefs(inst.CallTypeArgs)) != 0 {
		add("call:generic")
	}
	if inst.Call.Callee.Kind != wir.OperandPath {
		add("call:dynamic")
	} else {
		p := body.Path(wir.PathRef(inst.Call.Callee.Ref))
		kind, known := body.SymbolKind(wir.SymbolID(p.Symbol))
		if p.Symbol == 0 || len(p.Segments) != 0 || !known || kind == wir.SymbolParam || body.SymbolHasWrite(wir.SymbolID(p.Symbol)) {
			add("call:dynamic")
		}
		name := body.SymbolName(wir.SymbolID(p.Symbol))
		if name == "pcall" || name == "xpcall" {
			add("call:protected")
		}
	}
	for _, operand := range body.Operands(inst.List) {
		classifyBoundaryOperand(body, operand, callTemps, nil, add)
	}
	// Open result tails remain representable as numbered result slots once the
	// direct callee's summary is known. Only an expanding argument tail needs a
	// vararg expression outside the POC algebra.
	if inst.ListSpread {
		add("shape:vararg")
	}
}

func classifyBoundaryOperand(body *wir.Body, operand wir.Operand, callTemps map[uint32]struct{}, _ map[symbol.ID]struct{}, add func(string)) {
	switch operand.Kind {
	case wir.OperandConst:
		return
	case wir.OperandVararg:
		add("shape:vararg")
	case wir.OperandTemp:
		if _, ok := callTemps[operand.Ref]; !ok {
			add("shape:unsupported-value")
		}
	case wir.OperandPath:
		p := body.Path(wir.PathRef(operand.Ref))
		if len(p.Segments) != 0 {
			add("boundary:heap-read")
			return
		}
		kind, ok := body.SymbolKind(wir.SymbolID(p.Symbol))
		if !ok {
			add("boundary:unknown-symbol")
			return
		}
		switch kind {
		case wir.SymbolGlobal:
			add("boundary:global")
		case wir.SymbolUpvalue:
			add("boundary:capture")
		case wir.SymbolParam, wir.SymbolLocal:
		default:
			add("boundary:unknown-symbol")
		}
	default:
		add("shape:unsupported-value")
	}
}
