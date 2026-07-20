package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// projectLinkedCallDiagnostics closes one stabilized child diagnostic value at
// its immediate call site. Explicit-parameter descriptors remain target-
// relative because fact/diagnostic application substitutes actual arguments.
// Capture/global path obligations instead cross the exact linked frame now,
// yielding the caller-visible concrete path that owns the diagnostic.
func projectLinkedCallDiagnostics(caller, target *relationProgramBody, frame *linkedRelationFrame, callerInput state.State, child callpayload.DiagnosticOutput) (callpayload.DiagnosticOutput, error) {
	return projectLinkedCallDiagnosticsWithPath(caller, target, frame, child, func(path pathdom.Path) (pathdom.Path, bool) {
		return projectLinkedBoundaryDiagnosticPathAtCall(caller, target, frame, callerInput, path)
	})
}

// projectLinkedCallDiagnosticsWithPath is the sole carrier-neutral diagnostic
// lifting law. The caller supplies only the exact boundary-path resolver for
// its carrier; parameter validation/index shifting, exposure preservation and
// normalization cannot drift between concrete State and formal tuple use.
func projectLinkedCallDiagnosticsWithPath(
	caller, target *relationProgramBody,
	frame *linkedRelationFrame,
	child callpayload.DiagnosticOutput,
	projectPath func(pathdom.Path) (pathdom.Path, bool),
) (callpayload.DiagnosticOutput, error) {
	if caller == nil || target == nil || caller.relation.arena == nil || target.relation.arena == nil || frame == nil ||
		!frame.valid() || frame.owner != caller.variable || frame.target != target.variable ||
		!child.Valid(target.relation.arena.reg) || projectPath == nil {
		return callpayload.DiagnosticOutput{}, fmt.Errorf("transformer: linked call diagnostic projection is unowned")
	}
	for _, obligation := range child.ParamObligations {
		if obligation.ParamIndex < 0 || uint32(obligation.ParamIndex) >= target.relation.shape.Params {
			return callpayload.DiagnosticOutput{}, fmt.Errorf("transformer: linked call parameter obligation is outside target shape")
		}
	}
	for _, exposure := range child.ParamExposures {
		index := exposure.Source.PlaceholderIndex()
		if !exposure.Source.IsPlaceholder() || index < 0 || uint32(index) >= target.relation.shape.Params {
			return callpayload.DiagnosticOutput{}, fmt.Errorf("transformer: linked call parameter exposure is outside target shape")
		}
	}
	out := callpayload.DiagnosticOutput{
		SuspensionKnown: child.SuspensionKnown,
		MaySuspend:      child.MaySuspend,
		ParamExposures:  append([]callpayload.CallParamExposure(nil), child.ParamExposures...),
	}
	for _, obligation := range child.ParamObligations {
		index, explicit := linkedCallExplicitParamIndex(caller, frame, obligation.ParamIndex)
		if !explicit {
			continue
		}
		obligation.ParamIndex = index
		out.ParamObligations = append(out.ParamObligations, obligation)
	}
	for _, obligation := range child.PathObligations {
		root, _, exact := bodyBoundaryRootForDiagnosticPath(target, obligation.Path)
		if !exact {
			return callpayload.DiagnosticOutput{}, fmt.Errorf("transformer: child path obligation is outside target boundary")
		}
		if root.Kind == RootParam {
			obligation.Path = obligation.Path.Clone()
			out.PathObligations = append(out.PathObligations, obligation)
			continue
		}
		projected, exact := projectPath(obligation.Path)
		if !exact {
			return callpayload.DiagnosticOutput{}, fmt.Errorf("transformer: child path obligation has no exact linked caller path")
		}
		obligation.Path = projected
		out.PathObligations = append(out.PathObligations, obligation)
	}
	return out.Normalize(caller.relation.arena.reg), nil
}

// linkedCallExplicitParamIndex converts one target lexical parameter ordinal
// into the caller's explicit-argument ordinal. A colon call binds target
// parameter zero from the receiver operand; CallParamObligation is explicitly
// defined over argument syntax and therefore must neither publish that receiver
// slot nor leave the remaining parameters shifted by one.
func linkedCallExplicitParamIndex(caller *relationProgramBody, frame *linkedRelationFrame, targetParam int) (int, bool) {
	if caller == nil || caller.plan == nil || frame == nil || targetParam < 0 {
		return 0, false
	}
	site, ok := caller.plan.Facts().CallSiteView(frame.point)
	if !ok {
		return 0, false
	}
	_, receiverSource := site.ReceiverSource()
	_, receiverPath := site.ReceiverPath()
	if !receiverSource && !receiverPath {
		return targetParam, true
	}
	if targetParam == 0 {
		return 0, false
	}
	return targetParam - 1, true
}

func projectLinkedBoundaryDiagnosticPathAtCall(caller, target *relationProgramBody, frame *linkedRelationFrame, callerState state.State, targetPath pathdom.Path) (pathdom.Path, bool) {
	root, suffix, exact := bodyBoundaryRootForDiagnosticPath(target, targetPath)
	if !exact {
		return pathdom.Path{}, false
	}
	if root.Kind == RootParam {
		return targetPath.Clone(), true
	}
	offset := frame.shape.offset(root.Kind) + int(root.Index)
	if offset < 0 || offset >= len(frame.rootCircuit) || frame.rootCircuit[offset].root != root {
		return pathdom.Path{}, false
	}
	wire := frame.rootCircuit[offset]
	cursor, err := caller.roots.cursor(caller.relation.arena.reg, callerState)
	if err != nil {
		return pathdom.Path{}, false
	}
	if wire.path != 0 {
		base, exact := caller.relation.arena.evalPathWithContext(wire.path, cursor, SpecializationContext{MiddlePath: caller.middleInputPath})
		if exact && !base.IsEmpty() {
			return base.AppendSegments(suffix), true
		}
	}
	if wire.value == 0 || int(wire.value) >= len(caller.relation.arena.values) {
		return pathdom.Path{}, false
	}
	node := caller.relation.arena.values[wire.value]
	if node.op == valueEnvironment {
		symbol := rootSymbol(node.slot)
		if symbol != 0 {
			return pathdom.NewPath(symbol, "").AppendSegments(suffix), true
		}
	}
	if node.op == valueRoot && caller.relation.shape.validateInput(node.root) {
		base, exact := bodyRelativeBoundaryDiagnosticRootPath(caller, node.root, nil)
		if exact {
			return base.AppendSegments(suffix), true
		}
	}
	return pathdom.Path{}, false
}

// bodyRelativeBoundaryDiagnosticPath turns one arena-owned boundary path into
// the canonical path vocabulary of that same lexical body. Parameter roots
// use placeholders; captures and globals use their stable lexical symbols.
// It does not intern into the sealed arena, so Apply may append descendant
// suffixes without opening a second term owner.
func bodyRelativeBoundaryDiagnosticPath(body *relationProgramBody, term PathTerm) (pathdom.Path, bool) {
	if body == nil || body.relation.arena == nil || term == 0 || int(term) >= len(body.relation.arena.paths) {
		return pathdom.Path{}, false
	}
	node := body.relation.arena.paths[term]
	// Structural lowering deliberately addresses the current lexical value of
	// a parameter/capture/global through EnvironmentPath after N4. That storage
	// spelling is still boundary-relative when its symbol is one of this body's
	// sealed carriers; recover the carrier namespace instead of treating every
	// environment path as an owner-local temporary.
	if node.environment != 0 {
		for _, carrier := range body.roots.roots {
			if rootSymbol(carrier.slot) != node.environment {
				continue
			}
			return bodyRelativeBoundaryDiagnosticRootPath(body, carrier.root, node.segments)
		}
		return pathdom.Path{}, false
	}
	if node.root.Kind == RootMiddle {
		input, exact := body.relation.arena.middle.inputRoot(node.root)
		if !exact {
			return pathdom.Path{}, false
		}
		return bodyRelativeBoundaryDiagnosticRootPath(body, input, node.segments)
	}
	if !body.relation.shape.validateInput(node.root) {
		return pathdom.Path{}, false
	}
	return bodyRelativeBoundaryDiagnosticRootPath(body, node.root, node.segments)
}

func bodyRelativeBoundaryDiagnosticRootPath(body *relationProgramBody, root Root, suffix []segment.Segment) (pathdom.Path, bool) {
	if body == nil || !body.relation.shape.validateInput(root) {
		return pathdom.Path{}, false
	}
	if root.Kind == RootParam {
		return pathdom.NewPlaceholder(int(root.Index)).AppendSegments(suffix), true
	}
	for _, carrier := range body.roots.roots {
		if carrier.root != root {
			continue
		}
		symbol := rootSymbol(carrier.slot)
		if symbol == 0 {
			return pathdom.Path{}, false
		}
		return pathdom.NewPath(symbol, "").AppendSegments(suffix), true
	}
	return pathdom.Path{}, false
}

// linkedCallerBoundaryDiagnosticPath rebases a target-relative diagnostic
// path through the exact linked argument/capture/global circuit. The result is
// relative to the caller's own lexical boundary and can therefore be lifted
// again by the same Apply transfer. Locals and rvalues deliberately have no
// outward path identity.
func linkedCallerBoundaryDiagnosticPath(caller, target *relationProgramBody, frame *linkedRelationFrame, targetPath pathdom.Path) (pathdom.Path, bool) {
	if caller == nil || target == nil || frame == nil || !frame.valid() || frame.owner != caller.variable || frame.target != target.variable ||
		targetPath.IsEmpty() || targetPath.Version != 0 {
		return pathdom.Path{}, false
	}
	root, suffix, exact := bodyBoundaryRootForDiagnosticPath(target, targetPath)
	if !exact {
		return pathdom.Path{}, false
	}
	offset := frame.shape.offset(root.Kind) + int(root.Index)
	if offset < 0 || offset >= len(frame.rootCircuit) || frame.rootCircuit[offset].root != root {
		return pathdom.Path{}, false
	}
	wire := frame.rootCircuit[offset]
	var base pathdom.Path
	baseExact := false
	if wire.path != 0 {
		base, baseExact = bodyRelativeBoundaryDiagnosticPath(caller, wire.path)
	}
	if !baseExact && wire.value != 0 && int(wire.value) < len(caller.relation.arena.values) {
		node := caller.relation.arena.values[wire.value]
		if node.op == valueRoot {
			root := node.root
			if root.Kind == RootMiddle {
				root, baseExact = caller.relation.arena.middle.inputRoot(root)
			}
			if caller.relation.shape.validateInput(root) {
				base, baseExact = bodyRelativeBoundaryDiagnosticRootPath(caller, root, nil)
			}
		} else if node.op == valueEnvironment && node.slot != 0 {
			for _, carrier := range caller.roots.roots {
				if carrier.slot == node.slot {
					base, baseExact = bodyRelativeBoundaryDiagnosticRootPath(caller, carrier.root, nil)
					break
				}
			}
		}
	}
	if !baseExact || base.IsEmpty() {
		return pathdom.Path{}, false
	}
	return base.AppendSegments(suffix), true
}

func bodyBoundaryRootForDiagnosticPath(body *relationProgramBody, path pathdom.Path) (Root, []segment.Segment, bool) {
	if body == nil || path.IsEmpty() || path.Version != 0 {
		return Root{}, nil, false
	}
	if path.IsPlaceholder() {
		index := path.PlaceholderIndex()
		if index < 0 || uint32(index) >= body.relation.shape.Params {
			return Root{}, nil, false
		}
		return Root{Kind: RootParam, Index: uint32(index)}, append([]segment.Segment(nil), path.Segments...), true
	}
	if path.Symbol == 0 {
		return Root{}, nil, false
	}
	for _, carrier := range body.roots.roots {
		if rootSymbol(carrier.slot) == path.Symbol {
			return carrier.root, append([]segment.Segment(nil), path.Segments...), true
		}
	}
	return Root{}, nil, false
}
