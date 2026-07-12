package body

import (
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
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
	boundaryCaptures := make(map[symbol.ID]struct{})
	for _, capture := range symbolicBoundaryCaptureSymbols(s.wir, s.bindings.DirectCaptures(s.function)) {
		boundaryCaptures[capture] = struct{}{}
	}
	for capture := range compositionDescendantCaptureInputs(s.bindings, s.function) {
		if _, boundary := boundaryCaptures[capture]; boundary {
			add("boundary:capture")
		}
	}
	capturedLocals := compositionCapturedLocals(s.bindings, s.function)
	numericForLocals := make(map[symbol.ID]struct{})
	for i := 0; i < s.wir.Len(); i++ {
		inst := s.wir.Instr(i)
		if inst.Op == wir.OpIterate && inst.Iter == wir.IterNumeric {
			for _, result := range s.wir.Operands(inst.Results) {
				if result.Kind != wir.OperandPath {
					continue
				}
				p := s.wir.Path(wir.PathRef(result.Ref))
				if p.Symbol != 0 && len(p.Segments) == 0 {
					numericForLocals[p.Symbol] = struct{}{}
				}
			}
		}
		if inst.Op != wir.OpCall || inst.Call.Receiver.Kind != wir.OperandNone || inst.Call.Method != 0 || inst.Call.Callee.Kind != wir.OperandPath {
			continue
		}
		p := s.wir.Path(wir.PathRef(inst.Call.Callee.Ref))
		if p.Symbol != 0 && len(p.Segments) == 0 {
			directCallees[p.Symbol] = struct{}{}
		}
	}
	// Capture-bearing value relations are admitted later only when the exact
	// context certificate proves immutable scalar bindings. The same helper
	// supplies the operation-plan RootCapture order; direct-callee-only symbols
	// are absent from both authorities.
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
				classifyBoundaryOperand(s.wir, operand, callTemps, boundaryCaptures, add)
			}
		case wir.OpCall:
			classifyDirectCall(s.wir, inst, callTemps, boundaryCaptures, add)
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
			if !exactSymbolicLocalDeclaration(s.wir, inst, callTemps, numericForLocals, capturedLocals) {
				add("boundary:mutation")
			}
		case wir.OpBinOp:
			if !exactSymbolicAccumulatorWrite(s.wir, inst, numericForLocals, capturedLocals) {
				add("shape:unsupported-op")
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
func exactSymbolicLocalDeclaration(body *wir.Body, inst wir.Instruction, callTemps map[uint32]struct{}, numericForLocals, captured map[symbol.ID]struct{}) bool {
	if inst.Dst.Kind != wir.OperandPath {
		return false
	}
	path := body.Path(wir.PathRef(inst.Dst.Ref))
	if inst.Assign == wir.AssignNone && inst.A.Kind == wir.OperandNone {
		_, numeric := numericForLocals[path.Symbol]
		return numeric && path.Symbol != 0 && len(path.Segments) == 0
	}
	if inst.Assign != wir.AssignLocalDeclaration {
		return false
	}
	if path.Symbol == 0 || len(path.Segments) != 0 {
		return false
	}
	kind, ok := body.SymbolKind(wir.SymbolID(path.Symbol))
	if !ok || kind != wir.SymbolLocal {
		return false
	}
	if _, escapes := captured[path.Symbol]; escapes {
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

// exactSymbolicAccumulatorWrite recognizes the deliberately bounded mutable
// scalar slice: one local accumulator updated by adding the current numeric-for
// binder. The compiler independently proves that the operands resolve to
// context-independent constant product values and that this is the target's
// only ordinary write point. This syntax gate therefore grants no symbolic
// recurrence, alias, capture, or heap authority.
func exactSymbolicAccumulatorWrite(body *wir.Body, inst wir.Instruction, numericForLocals, captured map[symbol.ID]struct{}) bool {
	if body == nil || inst.Op != wir.OpBinOp || inst.Assign != wir.AssignOrdinaryRootWrite || inst.Operator != wir.BinAdd ||
		inst.Dst.Kind != wir.OperandPath || inst.A.Kind != wir.OperandPath || inst.B.Kind != wir.OperandPath {
		return false
	}
	target := body.Path(wir.PathRef(inst.Dst.Ref))
	left := body.Path(wir.PathRef(inst.A.Ref))
	right := body.Path(wir.PathRef(inst.B.Ref))
	if target.Symbol == 0 || len(target.Segments) != 0 || !target.Equal(left) || right.Symbol == 0 || len(right.Segments) != 0 {
		return false
	}
	kind, ok := body.SymbolKind(wir.SymbolID(target.Symbol))
	if !ok || kind != wir.SymbolLocal {
		return false
	}
	if _, escapes := captured[target.Symbol]; escapes {
		return false
	}
	_, numeric := numericForLocals[right.Symbol]
	return numeric
}

func compositionCapturedLocals(bindings *bind.Result, owner *ast.FunctionExpr) map[symbol.ID]struct{} {
	out := make(map[symbol.ID]struct{})
	if bindings == nil || owner == nil {
		return out
	}
	var visit func(*ast.FunctionExpr)
	visit = func(parent *ast.FunctionExpr) {
		for _, child := range bindings.NestedFunctions(parent) {
			for _, capture := range bindings.DirectCaptures(child) {
				if capture.Captured != 0 && capture.DeclaringFunction == owner {
					out[capture.Captured] = struct{}{}
				}
			}
			visit(child)
		}
	}
	visit(owner)
	return out
}

// compositionDescendantCaptureInputs reports boundary inputs that escape into
// a nested closure. The first capture slice is intentionally non-escaping:
// even an immutable scalar is declined when ownership crosses again into a
// descendant function.
func compositionDescendantCaptureInputs(bindings *bind.Result, owner *ast.FunctionExpr) map[symbol.ID]struct{} {
	out := make(map[symbol.ID]struct{})
	if bindings == nil || owner == nil {
		return out
	}
	var visit func(*ast.FunctionExpr)
	visit = func(parent *ast.FunctionExpr) {
		for _, child := range bindings.NestedFunctions(parent) {
			for _, capture := range bindings.DirectCaptures(child) {
				if capture.Captured != 0 && capture.DeclaringFunction != owner {
					out[capture.Captured] = struct{}{}
				}
			}
			visit(child)
		}
	}
	visit(owner)
	return out
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

func classifyDirectCall(body *wir.Body, inst wir.Instruction, callTemps map[uint32]struct{}, boundaryCaptures map[symbol.ID]struct{}, add func(string)) {
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
		classifyBoundaryOperand(body, operand, callTemps, boundaryCaptures, add)
	}
	// Open result tails remain representable as numbered result slots once the
	// direct callee's summary is known. Only an expanding argument tail needs a
	// vararg expression outside the POC algebra.
	if inst.ListSpread {
		add("shape:vararg")
	}
}

func classifyBoundaryOperand(body *wir.Body, operand wir.Operand, callTemps map[uint32]struct{}, boundaryCaptures map[symbol.ID]struct{}, add func(string)) {
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
			if _, represented := boundaryCaptures[p.Symbol]; !represented {
				add("boundary:capture")
			}
		case wir.SymbolParam, wir.SymbolLocal:
		default:
			add("boundary:unknown-symbol")
		}
	default:
		add("shape:unsupported-value")
	}
}
